package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// LegacyConfirmedUnsell delegates ONLY off-platform cases to the existing void
// compatibility path, outside return ownership so its sale/key preparation can
// commit separately. Its guarded preparation AND execution must recheck the
// supplied expectedSaleID against the fresh persisted sale before dispatch.
// Task3 must journal handle-recovery/void; nil fails closed, never routes external.
type LegacyConfirmedUnsell func(ctx context.Context, purchaseID, expectedSaleID string) error

type ConfirmedReturnService struct {
	scope      PurchaseMutationScope
	repo       ConfirmedReturnRepository
	remote     DHReturner
	legacy     LegacyConfirmedUnsell
	generateID func() string
	logger     observability.Logger
}
type ConfirmedReturnOption func(*ConfirmedReturnService)

func WithConfirmedReturnLogger(logger observability.Logger) ConfirmedReturnOption {
	return func(s *ConfirmedReturnService) { s.logger = logger }
}
func NewConfirmedReturnService(scope PurchaseMutationScope, repo ConfirmedReturnRepository, remote DHReturner, legacy LegacyConfirmedUnsell, generateID func() string, opts ...ConfirmedReturnOption) *ConfirmedReturnService {
	s := &ConfirmedReturnService{scope: scope, repo: repo, remote: remote, legacy: legacy, generateID: generateID}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
func (s *ConfirmedReturnService) available() error {
	if s == nil || !DHDependenciesPresent(s.repo) {
		return NewReturnConflict("coordination_unavailable", "confirmed returns require durable state storage")
	}
	return nil
}
func (s *ConfirmedReturnService) GetReturnState(ctx context.Context, id string) (*ConfirmedReturnState, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	return s.repo.GetReturnState(ctx, id)
}

func (s *ConfirmedReturnService) ConfirmReturn(ctx context.Context, id string, req ConfirmReturnRequest) (*ConfirmedReturnState, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if !DHDependenciesPresent(s.scope) {
		return nil, NewReturnConflict("coordination_unavailable", "confirmed returns require durable mutation ownership")
	}
	if !req.ReturnConfirmed {
		return nil, NewReturnConflict("return_confirmation_required", "physical possession and refund resolution must be explicitly confirmed")
	}
	if req.ExpectedSaleID != nil && *req.ExpectedSaleID == "" {
		return nil, NewReturnConflict("sale_precondition_failed", "expectedSaleId must be a nonempty ID or null")
	}
	if err := s.repo.AssertDHPreparationAllowed(ctx); err != nil {
		return nil, err
	}
	var state *ConfirmedReturnState
	var attempt *DHMutationAttempt
	legacySaleID := ""
	err := s.scope.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		var e error
		state, e = s.repo.ResolveReturn(owned, id, req)
		if e != nil {
			return e
		}
		if state.Operation != nil && state.Operation.State == "completed" {
			state.Outcome = "completed_replay"
			return nil
		}
		if a := state.PrecedingAttempt; a != nil && (state.Operation == nil || a.Kind != "return" || a.ReturnOperationID != state.Operation.ID) {
			return NewReturnConflict("preceding_dh_mutation_uncertain", "preceding DH attempt "+a.ID+" ("+a.Kind+"/"+a.Phase+") is unresolved")
		}
		source, e := SelectReturnSource(state.Purchase, state.Sale)
		if e != nil {
			return e
		}
		if source == "external" && state.Operation == nil {
			target := req.ExpectedTarget
			if target == nil || target.DHInventoryID <= 0 || strings.TrimSpace(target.CertNumber) == "" || strings.TrimSpace(target.Grader) == "" {
				return NewReturnConflict("client_update_required", "reload the app before confirming an external return")
			}
			if target.DHInventoryID != state.Purchase.DHInventoryID || target.CertNumber != state.Purchase.CertNumber || target.Grader != state.Purchase.Grader {
				return NewReturnConflict("identity_conflict", "observed return target differs from current purchase linkage; reload the app")
			}
		}
		if source != "external" {
			if e := s.repo.AssertMutationAllowed(owned, id); e != nil {
				return e
			}
			if source == "none" {
				state.Outcome = "no_return_required"
				return nil
			}
			if source == "legacy_void" {
				if s.legacy == nil {
					return NewReturnConflict("legacy_unsell_unavailable", "off-platform unsell path is not configured")
				}
				legacySaleID = state.Sale.ID
				return nil
			}
			if e := s.repo.DeleteLocalConfirmedSale(owned, id, state.Sale.ID); e != nil {
				return e
			}
			state, e = s.repo.GetReturnState(owned, id)
			if e != nil {
				return e
			}
			state.Outcome = source
			return nil
		}
		// Runtime deliberately has no returner when DH enterprise is disabled.
		// Local paths and completed replay above need only durable coordination.
		if !DHDependenciesPresent(s.remote) {
			return NewReturnConflict("coordination_unavailable", "external returns require DH capability")
		}
		if state.Operation == nil && state.Sale == nil {
			status, e := s.remote.GetReturnInventoryStatus(owned, state.Purchase.DHInventoryID, state.Purchase.CertNumber)
			if e != nil {
				return e
			}
			if status == "in_stock" || status == "listed" {
				state.Outcome = "no_return_required"
				return nil
			}
			if status != "sold" {
				return NewReturnConflict("return_preflight_conflict", "unexpected current inventory status")
			}
		}
		operationID, key := "", ""
		if state.Operation == nil {
			if s.generateID == nil {
				return NewReturnConflict("coordination_unavailable", "new external returns require server ID generation")
			}
			operationID, key = s.generateID(), "slabledger-return-"+s.generateID()
		}
		state, e = s.repo.PrepareReturn(owned, id, req, operationID, key)
		if e != nil {
			return e
		}
		ep := state.Operation
		attempt, e = s.repo.PrepareDHMutation(owned, id, DHMutationRequest{Kind: "return", Phase: "return_to_stock", Key: ep.Key, PayloadIdentity: fmt.Sprintf("return:%s:%d:true", ep.ID, ep.DHInventoryID), ReturnOperationID: ep.ID})
		return e
	})
	if err != nil {
		// A failed preparation COMMIT must not expose an operation that only existed
		// in the rolled-back transaction. The projection is always durable truth.
		refresh, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		fresh, readErr := s.repo.GetReturnState(refresh, id)
		if readErr != nil {
			return nil, errors.Join(err, readErr)
		}
		return fresh, err
	}
	if legacySaleID != "" {
		if e := s.legacy(ctx, id, legacySaleID); e != nil {
			return state, e
		}
		state, e := s.repo.GetReturnState(ctx, id)
		if state != nil {
			state.Outcome = "legacy_void"
		}
		return state, e
	}
	if attempt == nil {
		return state, nil
	}
	ep := state.Operation
	var receipt *DHReturnResult
	phase := "dispatch"
	var remoteErr error
	err = s.scope.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		current, e := s.repo.ResolveReturn(owned, id, req)
		if e != nil {
			return e
		}
		if current.Operation != nil && current.Operation.State == "completed" {
			state = current
			state.Outcome = "completed_replay"
			return nil
		}
		if current.Operation == nil || current.Operation.ID != ep.ID {
			return NewReturnConflict("identity_conflict", "return operation changed before execution")
		}
		ep = current.Operation // preparation may predate another caller's durable receipt
		if !DHDependenciesPresent(s.remote) {
			return NewReturnConflict("coordination_unavailable", "external returns require DH capability")
		}
		if e := s.repo.OwnDHMutation(owned, id, attempt); e != nil {
			return e
		}
		receipt, remoteErr = s.remote.ReturnInventoryToStock(owned, ep.DHInventoryID, ep.Key)
		if remoteErr != nil {
			return remoteErr
		}
		phase = "response"
		if e := validateReturnReceipt(ep, receipt); e != nil {
			return e
		}
		phase = "completion"
		if e := s.repo.CompleteReturn(owned, id, ep, receipt); e != nil {
			return e
		}
		phase = "settlement"
		return s.repo.SettleDHMutation(owned, id, attempt, DHMutationSettlement{Outcome: "succeeded", Receipt: fmt.Sprintf("ext-%d", receipt.ExternalSaleID)})
	})
	if err != nil {
		failure := returnFailure(err, phase)
		// Context cancellation may have ended local ownership, not remote execution.
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		definitive := remoteErr != nil && IsDefinitiveDHReturnRejection(remoteErr) && !attempt.Replaying
		if definitive {
			settleErr := s.scope.WithPurchaseMutation(persist, id, func(owned context.Context) error {
				if e := s.repo.OwnDHMutation(owned, id, attempt); e != nil {
					return e
				}
				return s.repo.SettleDHMutation(owned, id, attempt, DHMutationSettlement{Outcome: "rejected", Receipt: failure.Code})
			})
			if settleErr != nil {
				err = errors.Join(err, settleErr)
			}
		}
		conflicted := errors.Is(err, ErrReturnConflict) || definitive
		observed := receipt
		// Preserve the first validated original attribution across local rollback;
		// a later conflicting replay is evidence of conflict, not new authority.
		if validObservedReturn(ep) {
			observed = nil
		}
		if e := s.repo.RecordReturnFailure(persist, id, ep.ID, failure, observed, conflicted); e != nil {
			err = errors.Join(err, e)
		}
		fresh, e := s.repo.GetReturnState(persist, id)
		if e == nil {
			state = fresh
		} else {
			err = errors.Join(err, e)
			state = nil
		}
		s.log(ctx, id, ep, receipt, failure.Phase, "failed", failure.Code)
		return state, err
	}
	if state.Outcome == "completed_replay" {
		// Preserve the historical operation resolved under execution ownership;
		// a latest-state reload could replace it with a different newer episode.
		return state, nil
	}
	state, err = s.repo.GetReturnState(ctx, id)
	if state != nil && state.Outcome == "" {
		state.Outcome = "completed"
	}
	s.log(ctx, id, ep, receipt, "completed", "succeeded", "")
	return state, err
}
func validateReturnReceipt(e *ConfirmedReturnEpisode, r *DHReturnResult) error {
	if r == nil || r.DHInventoryID != e.DHInventoryID || r.ExternalSaleID <= 0 {
		return NewReturnConflict("identity_conflict", "return response does not identify the captured target and external sale")
	}
	if r.ItemStatus != "in_stock" {
		return NewReturnConflict("return_status_conflict", "pending return replay reports current status "+r.ItemStatus)
	}
	if e.CapturedOrderID != "" && e.CapturedOrderID != fmt.Sprintf("ext-%d", r.ExternalSaleID) {
		return NewReturnConflict("identity_conflict", "returned external sale disagrees with captured local order")
	}
	if validObservedReturn(e) && r.ExternalSaleID != e.ObservedReceipt.ExternalSaleID {
		return NewReturnConflict("identity_conflict", "same-key replay changed the observed original external sale")
	}
	return nil
}
func validObservedReturn(e *ConfirmedReturnEpisode) bool {
	r := e.ObservedReceipt
	return r != nil && r.DHInventoryID == e.DHInventoryID && r.ExternalSaleID > 0 && r.ItemStatus == "in_stock" && (e.CapturedOrderID == "" || e.CapturedOrderID == fmt.Sprintf("ext-%d", r.ExternalSaleID))
}
func returnFailure(err error, phase string) ReturnFailure {
	f := ReturnFailure{Code: "return_failed", Message: err.Error(), Phase: phase}
	var upstream *DHReturnError
	if errors.As(err, &upstream) {
		if upstream.Code != "" {
			f.Code = upstream.Code
		}
		if upstream.Message != "" {
			f.Message = upstream.Message
		}
	}
	var conflict *ReturnConflict
	if errors.As(err, &conflict) {
		f.Code = conflict.Code
		f.Message = conflict.Message
	}
	f.Code = string([]rune(f.Code)[:min(len([]rune(f.Code)), 128)])
	f.Message = string([]rune(f.Message)[:min(len([]rune(f.Message)), 1024)])
	return f
}
func (s *ConfirmedReturnService) log(ctx context.Context, id string, e *ConfirmedReturnEpisode, r *DHReturnResult, phase, outcome, code string) {
	if !DHDependenciesPresent(s.logger) {
		return
	}
	external := ""
	restored := false
	if r != nil {
		external = fmt.Sprint(r.ExternalSaleID)
		restored = r.Restored
	}
	s.logger.Info(ctx, "confirmed DH return", observability.String("operationId", e.ID), observability.String("purchaseId", id), observability.String("inventoryId", fmt.Sprint(e.DHInventoryID)), observability.String("originalOrderId", e.CapturedOrderID), observability.String("returnedExternalSaleId", external), observability.String("phase", phase), observability.String("outcome", outcome), observability.String("upstreamCode", code), observability.Bool("restored", restored))
}
