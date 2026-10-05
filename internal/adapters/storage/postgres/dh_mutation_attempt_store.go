package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

const attemptColumns = `id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,idempotency_key,payload_identity,COALESCE(return_operation_id,''),started_at,settled_at,outcome`

func scanAttempt(row scanner) (*inventory.DHMutationAttempt, error) {
	var a inventory.DHMutationAttempt
	err := row.Scan(&a.ID, &a.CapturedPurchaseID, &a.DHInventoryID, &a.CertNumber, &a.Grader, &a.Kind, &a.Phase, &a.Key, &a.PayloadIdentity, &a.ReturnOperationID, &a.StartedAt, &a.SettledAt, &a.Outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}
func (s *ConfirmedReturnStore) openAttempt(ctx context.Context, p *inventory.Purchase) (*inventory.DHMutationAttempt, error) {
	return scanAttempt(executor(ctx, s.db).QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM dh_mutation_attempts WHERE outcome='open' AND ((grader=$1 AND cert_number=$2 AND cert_number<>'') OR dh_inventory_id=NULLIF($3,0) OR captured_purchase_id=$4) ORDER BY started_at LIMIT 1`, p.Grader, p.CertNumber, p.DHInventoryID, p.ID))
}
func (s *ConfirmedReturnStore) AssertDHPreparationAllowed(ctx context.Context) error {
	if ctx.Value(mutationScopeKey{}) != nil {
		return inventory.NewReturnConflict("nested_dh_preparation", "DH preparation must commit before opening execution ownership")
	}
	return nil
}
func (s *ConfirmedReturnStore) PrepareDHMutation(ctx context.Context, id string, req inventory.DHMutationRequest) (*inventory.DHMutationAttempt, error) {
	scope, err := ownedScope(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if scope.attemptID != "" {
		return nil, inventory.NewReturnConflict("nested_dh_preparation", "cannot prepare during execution")
	}
	if req.Kind == "" || req.Phase == "" || req.PayloadIdentity == "" {
		return nil, inventory.NewReturnConflict("invalid_mutation_identity", "immutable kind, phase and payload identity required")
	}
	state, err := s.returnState(ctx, id)
	if err != nil {
		return nil, err
	}
	p := state.Purchase
	open := state.PrecedingAttempt
	if open != nil {
		if (req.Kind != "sale" && req.Kind != "return") || req.Key == "" || open.Key != req.Key || open.Kind != req.Kind || open.Phase != req.Phase || open.PayloadIdentity != req.PayloadIdentity || open.ReturnOperationID != req.ReturnOperationID || open.CapturedPurchaseID != id || open.DHInventoryID != p.DHInventoryID || open.CertNumber != p.CertNumber || open.Grader != p.Grader {
			return nil, inventory.NewReturnConflict("preceding_dh_mutation_uncertain", "preceding DH attempt "+open.ID+" ("+open.Kind+"/"+open.Phase+") requires verified completion evidence")
		}
		open.Replaying = true
		return open, nil
	}
	if req.ReturnOperationID != "" {
		e := state.Operation
		if req.Kind != "return" || e == nil || e.ID != req.ReturnOperationID || e.Key != req.Key || e.State == "completed" || !episodeTargetMatches(e, p) {
			return nil, inventory.NewReturnConflict("identity_conflict", "attempt does not name current return episode")
		}
		scope.returnID = e.ID
	}
	if err := s.assertAllowed(ctx, id, req.Kind != "sale" && req.Kind != "return"); err != nil {
		return nil, err
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	attemptID := "dh-attempt-" + hex.EncodeToString(bytes)
	var operation any
	if req.ReturnOperationID != "" {
		operation = req.ReturnOperationID
	}
	a, err := scanAttempt(executor(ctx, s.db).QueryRowContext(ctx, `INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,idempotency_key,payload_identity,return_operation_id) VALUES($1,$2,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+attemptColumns, attemptID, id, p.DHInventoryID, p.CertNumber, p.Grader, req.Kind, req.Phase, req.Key, req.PayloadIdentity, operation))
	if err != nil {
		return nil, fmt.Errorf("prepare DH attempt: %w", err)
	}
	if err := s.watermark(ctx, p, "mutation_started_at"); err != nil {
		return nil, err
	}
	return a, nil
}
func (s *ConfirmedReturnStore) OwnDHMutation(ctx context.Context, id string, a *inventory.DHMutationAttempt) error {
	scope, err := ownedScope(ctx, s.db, id)
	if err != nil {
		return err
	}
	state, err := s.returnState(ctx, id)
	if err != nil {
		return err
	}
	open := state.PrecedingAttempt
	p := state.Purchase
	if a == nil || open == nil || open.ID != a.ID || open.Kind != a.Kind || open.Key != a.Key || open.PayloadIdentity != a.PayloadIdentity || open.ReturnOperationID != a.ReturnOperationID || p.DHInventoryID != a.DHInventoryID || p.CertNumber != a.CertNumber || p.Grader != a.Grader || id != a.CapturedPurchaseID {
		return inventory.NewReturnConflict("identity_conflict", "prepared attempt or target changed before execution")
	}
	scope.attemptID = a.ID
	scope.returnID = a.ReturnOperationID
	return s.assertAllowed(ctx, id, a.Kind != "sale" && a.Kind != "return")
}
func (s *ConfirmedReturnStore) SettleDHMutation(ctx context.Context, id string, a *inventory.DHMutationAttempt, result inventory.DHMutationSettlement) error {
	scope, err := ownedScope(ctx, s.db, id)
	if err != nil {
		return err
	}
	if a == nil || scope.attemptID != a.ID || result.Receipt == "" || (result.Outcome != "succeeded" && result.Outcome != "rejected") {
		return inventory.NewReturnConflict("terminal_receipt_required", "settlement requires owned attempt and verified terminal response")
	}
	x := executor(ctx, s.db)
	res, err := x.ExecContext(ctx, `UPDATE dh_mutation_attempts SET settled_at=clock_timestamp(),outcome=$1,receipt=$2 WHERE id=$3 AND outcome='open'`, result.Outcome, result.Receipt, a.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return inventory.NewReturnConflict("attempt_settlement_conflict", "attempt is no longer open")
	}
	p, err := (&PurchaseStore{base: base{db: s.db}}).GetPurchase(ctx, id)
	if err != nil {
		return err
	}
	return s.watermark(ctx, p, "mutation_settled_at")
}
func (s *ConfirmedReturnStore) watermark(ctx context.Context, p *inventory.Purchase, column string) error {
	// Column names are internal constants, never caller input.
	_, err := executor(ctx, s.db).ExecContext(ctx, `INSERT INTO dh_target_watermarks(grader,cert_number,dh_inventory_id,`+column+`) VALUES($1,$2,$3,clock_timestamp()) ON CONFLICT(grader,cert_number) DO UPDATE SET dh_inventory_id=EXCLUDED.dh_inventory_id,`+column+`=EXCLUDED.`+column, p.Grader, p.CertNumber, p.DHInventoryID)
	return err
}

// AssertMutationAllowed must run under ownership BEFORE any remote side effect.
// It blocks pending/conflicted returns, unknown prior DH work and retained holds.
func (s *ConfirmedReturnStore) AssertMutationAllowed(ctx context.Context, id string) error {
	return s.assertAllowed(ctx, id, true)
}
func (s *ConfirmedReturnStore) assertAllowed(ctx context.Context, id string, awaiting bool) error {
	scope, err := ownedScope(ctx, s.db, id)
	if err != nil {
		return err
	}
	state, err := s.returnState(ctx, id)
	if err != nil {
		return err
	}
	e := state.Operation
	p := state.Purchase
	if a := state.PrecedingAttempt; a != nil && a.ID != scope.attemptID {
		return inventory.NewReturnConflict("preceding_dh_mutation_uncertain", "preceding DH attempt "+a.ID+" ("+a.Kind+"/"+a.Phase+") is unresolved")
	}
	if e != nil {
		// An unlinked/new target cannot escape a retained return hold or receipt.
		if !episodeTargetMatches(e, p) {
			return inventory.NewReturnConflict("identity_conflict", "retained return target must be resolved before mutation")
		}
		if e.State != "completed" && e.ID != scope.returnID {
			return inventory.NewReturnConflict("confirmed_return_unresolved", "return operation "+e.ID+" is unresolved")
		}
		if awaiting && state.AwaitingListing {
			return inventory.NewReturnConflict("awaiting_explicit_listing", "returned target awaits explicit authenticated listing")
		}
	}
	return nil
}

// ObservationTime is DATABASE time captured BEFORE a fetch starts.
func (s *ConfirmedReturnStore) ObservationTime(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := executor(ctx, s.db).QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&t)
	return t, err
}

// ApplyDHObservation checks both dispatch and settlement watermarks under fresh
// ownership. The callback uses that context for its conditional local writes.
// False means discarded; neither GET nor an observation settles uncertainty.
func (s *ConfirmedReturnStore) ApplyDHObservation(ctx context.Context, id string, target int, fetchedAt time.Time, apply func(context.Context) error) (bool, error) {
	applied := false
	err := s.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		state, err := s.returnState(owned, id)
		if err != nil {
			return err
		}
		if state.Purchase.DHInventoryID != target || fetchedAt.IsZero() || state.PrecedingAttempt != nil || state.AwaitingListing || (state.Operation != nil && (state.Operation.State != "completed" || !episodeTargetMatches(state.Operation, state.Purchase))) {
			return nil
		}
		var stale bool
		err = executor(owned, s.db).QueryRowContext(owned, `SELECT EXISTS(SELECT 1 FROM dh_target_watermarks WHERE grader=$1 AND cert_number=$2 AND (mutation_started_at>=$3 OR mutation_settled_at>=$3))`, state.Purchase.Grader, state.Purchase.CertNumber, fetchedAt).Scan(&stale)
		if err != nil {
			return err
		}
		if stale {
			return nil
		}
		scope, _ := ownedScope(owned, s.db, id)
		previousObservation := scope.observation
		scope.observation = true
		// A reentrant callback shares this scope with later ordinary mutations.
		// Restore its prior mode even on error, and preserve an outer observation.
		defer func() { scope.observation = previousObservation }()
		if err := apply(owned); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

// ApplyDHSoldObservation ties delayed sold writes to a present, nonreturned sale.
func (s *ConfirmedReturnStore) ApplyDHSoldObservation(ctx context.Context, id, saleID string, target int, fetchedAt time.Time, apply func(context.Context) error) (bool, error) {
	valid := false
	applied, err := s.ApplyDHObservation(ctx, id, target, fetchedAt, func(owned context.Context) error {
		state, err := s.returnState(owned, id)
		if err != nil {
			return err
		}
		if state.Sale == nil || state.Sale.ID != saleID {
			return nil
		}
		if err := s.assertOrderNotReturned(owned, state.Purchase, state.Sale.OrderID); err != nil {
			return nil
		}
		valid = true
		return apply(owned)
	})
	return applied && valid, err
}
