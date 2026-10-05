package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type ConfirmedReturnStore struct{ *PurchaseMutationCoordinator }

func NewConfirmedReturnStore(db *sql.DB) *ConfirmedReturnStore {
	return &ConfirmedReturnStore{NewPurchaseMutationScope(db)}
}

var _ inventory.ConfirmedReturnRepository = (*ConfirmedReturnStore)(nil)

const episodeColumns = `id,idempotency_key,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,captured_sale_id,captured_order_id,returned_order_id,state,created_at,completed_at,listing_authorized_at,error_code,error_message,error_phase,observed_receipt`

func scanReturn(row scanner) (*inventory.ConfirmedReturnEpisode, error) {
	var e inventory.ConfirmedReturnEpisode
	var code, msg, phase string
	var receipt []byte
	err := row.Scan(&e.ID, &e.Key, &e.PurchaseID, &e.CapturedPurchaseID, &e.DHInventoryID, &e.CertNumber, &e.Grader, &e.ExpectedSaleID, &e.CapturedOrderID, &e.ReturnedOrderID, &e.State, &e.CreatedAt, &e.CompletedAt, &e.ListingAuthorizedAt, &code, &msg, &phase, &receipt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if code != "" {
		e.LastError = &inventory.ReturnFailure{Code: code, Message: msg, Phase: phase}
	}
	if receipt != nil {
		if err := json.Unmarshal(receipt, &e.ObservedReceipt); err != nil {
			return nil, err
		}
	}
	return &e, nil
}
func (s *ConfirmedReturnStore) latestReturn(ctx context.Context, p *inventory.Purchase) (*inventory.ConfirmedReturnEpisode, error) {
	return scanReturn(executor(ctx, s.db).QueryRowContext(ctx, `SELECT `+episodeColumns+` FROM confirmed_dh_returns WHERE (grader=$1 AND cert_number=$2) OR dh_inventory_id=NULLIF($3,0) ORDER BY created_at DESC,id DESC LIMIT 1`, p.Grader, p.CertNumber, p.DHInventoryID))
}
func (s *ConfirmedReturnStore) GetReturnState(ctx context.Context, id string) (*inventory.ConfirmedReturnState, error) {
	var state *inventory.ConfirmedReturnState
	err := s.WithPurchaseMutation(ctx, id, func(owned context.Context) error { var e error; state, e = s.returnState(owned, id); return e })
	return state, err
}
func (s *ConfirmedReturnStore) returnState(ctx context.Context, id string) (*inventory.ConfirmedReturnState, error) {
	if _, err := ownedScope(ctx, s.db, id); err != nil {
		return nil, err
	}
	p, err := (&PurchaseStore{base: base{db: s.db}}).GetPurchase(ctx, id)
	if err != nil {
		return nil, err
	}
	sale, err := (&SaleStore{base: base{db: s.db}}).GetSaleByPurchaseID(ctx, id)
	if err != nil && !errors.Is(err, inventory.ErrSaleNotFound) {
		return nil, err
	}
	ep, err := s.latestReturn(ctx, p)
	if err != nil {
		return nil, err
	}
	a, err := s.openAttempt(ctx, p)
	if err != nil {
		return nil, err
	}
	state := &inventory.ConfirmedReturnState{Purchase: p, Sale: sale, Operation: ep, PrecedingAttempt: a}
	if sale != nil {
		v := sale.ID
		state.ExpectedSaleID = &v
	}
	state.AwaitingListing = ep != nil && ep.State == "completed" && ep.ListingAuthorizedAt == nil
	return state, nil
}
func sameSale(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
func episodeTargetMatches(e *inventory.ConfirmedReturnEpisode, p *inventory.Purchase) bool {
	return e.DHInventoryID == p.DHInventoryID && e.CertNumber == p.CertNumber && e.Grader == p.Grader
}

// ResolveReturn performs lifecycle/CAS resolution without creating an episode.
// The service uses it before dispatch and initial read-only remote preflight.
func (s *ConfirmedReturnStore) ResolveReturn(ctx context.Context, id string, req inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error) {
	state, err := s.returnState(ctx, id)
	if err != nil {
		return nil, err
	}
	p := state.Purchase
	if req.OperationID != "" {
		state.Operation, err = scanReturn(executor(ctx, s.db).QueryRowContext(ctx, `SELECT `+episodeColumns+` FROM confirmed_dh_returns WHERE id=$1`, req.OperationID))
		if err != nil {
			return nil, err
		}
		if state.Operation == nil {
			return nil, inventory.NewReturnConflict("operation_not_found", "unknown return operation")
		}
	} else {
		// operationId is optional: an identical completed body still identifies its
		// historical episode after a different persisted sale starts a newer one.
		// Keep current sale/attempt/latest hold from returnState, not the old episode.
		completed, e := scanReturn(executor(ctx, s.db).QueryRowContext(ctx, `SELECT `+episodeColumns+` FROM confirmed_dh_returns WHERE state='completed' AND grader=$1 AND cert_number=$2 AND dh_inventory_id=$3 AND captured_sale_id IS NOT DISTINCT FROM $4::text ORDER BY created_at DESC,id DESC LIMIT 1`, p.Grader, p.CertNumber, p.DHInventoryID, req.ExpectedSaleID))
		if e != nil {
			return nil, e
		}
		if completed != nil {
			state.Operation = completed
			return state, nil
		}
	}
	e := state.Operation
	if e != nil {
		if !episodeTargetMatches(e, p) {
			return state, inventory.NewReturnConflict("identity_conflict", "retained return target differs from current purchase linkage")
		}
		if req.OperationID != "" && !sameSale(e.ExpectedSaleID, req.ExpectedSaleID) {
			return state, inventory.NewReturnConflict("sale_precondition_failed", "operation sale precondition differs")
		}
		if e.State != "completed" {
			if !sameSale(e.ExpectedSaleID, req.ExpectedSaleID) || !sameSale(e.ExpectedSaleID, state.ExpectedSaleID) {
				return state, inventory.NewReturnConflict("sale_precondition_failed", "captured sale changed")
			}
			return state, nil
		}
		// Null always replays the historical recovery; a genuinely new persisted
		// sale and matching CAS is required to establish the next episode.
		if sameSale(e.ExpectedSaleID, req.ExpectedSaleID) || req.ExpectedSaleID == nil || req.OperationID != "" {
			return state, nil
		}
	}
	if !sameSale(req.ExpectedSaleID, state.ExpectedSaleID) {
		return state, inventory.NewReturnConflict("sale_precondition_failed", "current sale differs from confirmation")
	}
	state.Operation = nil
	return state, nil
}
func (s *ConfirmedReturnStore) PrepareReturn(ctx context.Context, id string, req inventory.ConfirmReturnRequest, operationID, key string) (*inventory.ConfirmedReturnState, error) {
	state, err := s.ResolveReturn(ctx, id, req)
	if err != nil {
		return state, err
	}
	if state.Operation != nil {
		return state, nil
	}
	if err := s.assertAllowed(ctx, id, false); err != nil {
		return state, err
	}
	p := state.Purchase
	if p.DHInventoryID <= 0 || p.CertNumber == "" || p.Grader == "" {
		return state, inventory.NewReturnConflict("identity_conflict", "external return requires retained inventory/cert/grader")
	}
	order := ""
	if state.Sale != nil {
		order = state.Sale.OrderID
	}
	ep, err := scanReturn(executor(ctx, s.db).QueryRowContext(ctx, `INSERT INTO confirmed_dh_returns(id,idempotency_key,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,captured_sale_id,captured_order_id) VALUES($1,$2,$3,$3,$4,$5,$6,$7,$8) RETURNING `+episodeColumns, operationID, key, id, p.DHInventoryID, p.CertNumber, p.Grader, req.ExpectedSaleID, order))
	if err != nil {
		return state, fmt.Errorf("prepare confirmed return: %w", err)
	}
	state.Operation = ep
	return state, nil
}

func (s *ConfirmedReturnStore) CompleteReturn(ctx context.Context, id string, e *inventory.ConfirmedReturnEpisode, r *inventory.DHReturnResult) error {
	scope, err := ownedScope(ctx, s.db, id)
	if err != nil {
		return err
	}
	if scope.returnID != e.ID || scope.attemptID == "" {
		return inventory.NewReturnConflict("mutation_scope_required", "return completion requires journal ownership")
	}
	state, err := s.returnState(ctx, id)
	if err != nil {
		return err
	}
	if state.Operation == nil || state.Operation.ID != e.ID || state.Operation.State == "completed" {
		return inventory.NewReturnConflict("identity_conflict", "current durable episode differs before completion")
	}
	e = state.Operation
	if !episodeTargetMatches(e, state.Purchase) || !sameSale(e.ExpectedSaleID, state.ExpectedSaleID) {
		return inventory.NewReturnConflict("identity_conflict", "captured target or sale changed before completion")
	}
	if r == nil || r.DHInventoryID != e.DHInventoryID || r.ItemStatus != "in_stock" || r.ExternalSaleID <= 0 {
		return inventory.NewReturnConflict("return_receipt_conflict", "return receipt has unsafe identity or current status")
	}
	canonical := fmt.Sprintf("ext-%d", r.ExternalSaleID)
	if e.CapturedOrderID != "" && e.CapturedOrderID != canonical {
		return inventory.NewReturnConflict("identity_conflict", "returned external sale differs from captured order")
	}
	if previous := e.ObservedReceipt; previous != nil && previous.DHInventoryID == e.DHInventoryID && previous.ExternalSaleID > 0 && previous.ItemStatus == "in_stock" && previous.ExternalSaleID != r.ExternalSaleID {
		return inventory.NewReturnConflict("identity_conflict", "same-key replay changed observed external attribution")
	}
	if state.Sale != nil && (state.Sale.OrderID != e.CapturedOrderID || state.Sale.DHSaleID != "" || state.Sale.DHIdempotencyKey != "") {
		return inventory.NewReturnConflict("identity_conflict", "captured sale attribution changed")
	}
	x := executor(ctx, s.db)
	if e.ExpectedSaleID != nil {
		result, err := x.ExecContext(ctx, `DELETE FROM campaign_sales WHERE id=$1 AND purchase_id=$2`, *e.ExpectedSaleID, id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return inventory.NewReturnConflict("sale_precondition_failed", "captured sale disappeared")
		}
	}
	if _, err := x.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='in_stock',dh_push_status='matched',dh_push_attempts=0,dh_listing_price_cents=0,dh_channels_json='[]',dh_hold_reason='',dh_unlisted_detected_at=NULL,dh_last_synced_at=to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),updated_at=clock_timestamp() WHERE id=$1`, id); err != nil {
		return err
	}
	receipt, err := json.Marshal(r)
	if err != nil {
		return err
	}
	result, err := x.ExecContext(ctx, `UPDATE confirmed_dh_returns SET state='completed',returned_order_id=$1,completed_at=clock_timestamp(),observed_receipt=$2,error_code='',error_message='',error_phase='' WHERE id=$3 AND state<>'completed'`, canonical, string(receipt), e.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return inventory.NewReturnConflict("return_receipt_conflict", "episode already completed or missing")
	}
	return nil
}
func (s *ConfirmedReturnStore) RecordReturnFailure(ctx context.Context, id, operationID string, f inventory.ReturnFailure, r *inventory.DHReturnResult, conflicted bool) error {
	return s.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		// Failure evidence is written after execution rollback. Another prepared
		// caller may have persisted the original receipt in the meantime, so never
		// decide which attribution to retain from its old preparation snapshot.
		episode, err := scanReturn(executor(owned, s.db).QueryRowContext(owned, `SELECT `+episodeColumns+` FROM confirmed_dh_returns WHERE id=$1 AND captured_purchase_id=$2`, operationID, id))
		if err != nil {
			return err
		}
		if episode == nil {
			return inventory.NewReturnConflict("operation_not_found", "return failure references unknown captured operation")
		}
		if episode.State == "completed" {
			return nil
		}
		previous := episode.ObservedReceipt
		if previous != nil && previous.DHInventoryID == episode.DHInventoryID && previous.ExternalSaleID > 0 && previous.ItemStatus == "in_stock" && (episode.CapturedOrderID == "" || episode.CapturedOrderID == fmt.Sprintf("ext-%d", previous.ExternalSaleID)) {
			r = nil // preserve the FIRST valid original attribution, not a later conflict
		}
		state := "pending"
		if conflicted {
			state = "conflicted"
		}
		var receipt any
		if r != nil {
			b, e := json.Marshal(r)
			if e != nil {
				return e
			}
			receipt = string(b)
		}
		result, err := executor(owned, s.db).ExecContext(owned, `UPDATE confirmed_dh_returns SET state=$1,error_code=$2,error_message=$3,error_phase=$4,observed_receipt=COALESCE($5::jsonb,observed_receipt) WHERE id=$6 AND state<>'completed' AND captured_purchase_id=$7`, state, f.Code, f.Message, f.Phase, receipt, operationID, id)
		if err != nil {
			return err
		}
		_, err = result.RowsAffected()
		return err
	})
}

// DeleteLocalConfirmedSale never calls a provider or resets DH tracking.
func (s *ConfirmedReturnStore) DeleteLocalConfirmedSale(ctx context.Context, id, saleID string) error {
	state, err := s.returnState(ctx, id)
	if err != nil {
		return err
	}
	if state.Purchase.DHInventoryID != 0 || state.Sale == nil || state.Sale.ID != saleID {
		return inventory.NewReturnConflict("sale_precondition_failed", "local unsell target or sale changed")
	}
	return (&SaleStore{base: base{db: s.db}}).DeleteSale(ctx, saleID)
}

// Authentication, positive price, received status and pause validation belong
// to the explicit List caller. This method only grants the exact latest target.
func (s *ConfirmedReturnStore) AuthorizeReturnedListing(ctx context.Context, id, operationID string) error {
	state, err := s.returnState(ctx, id)
	if err != nil {
		return err
	}
	if err := s.assertAllowed(ctx, id, false); err != nil {
		return err
	}
	e := state.Operation
	if e == nil || e.ID != operationID || e.State != "completed" || !episodeTargetMatches(e, state.Purchase) || state.Sale != nil {
		return inventory.NewReturnConflict("listing_authorization_conflict", "latest completed same-target return with no current sale required")
	}
	_, err = executor(ctx, s.db).ExecContext(ctx, `UPDATE confirmed_dh_returns SET listing_authorized_at=COALESCE(listing_authorized_at,clock_timestamp()) WHERE id=$1`, e.ID)
	return err
}
