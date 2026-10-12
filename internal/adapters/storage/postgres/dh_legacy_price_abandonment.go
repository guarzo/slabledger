package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// LegacyDHPriceAbandonmentRequest names one previously reviewed unkeyed PATCH.
// FrozenPayloadIdentity is the exact original string, not today's price payload.
// Authorization and quiescence are operator evidence, not provider receipts.
// This storage-only operation deliberately has no domain port or API endpoint.
type LegacyDHPriceAbandonmentRequest struct {
	ExpectedAttemptID         string
	DHInventoryID             int
	CertNumber                string
	Grader                    string
	FrozenPayloadIdentity     string
	ApprovedListingPriceCents int
	OperatorAuthorization     string
	QuiescenceEvidence        string
}

// AbandonLegacyDHPrice closes administrative uncertainty without asserting that
// DH succeeded or rejected the original request. It requires an existing owned
// transaction; it neither dispatches remote work nor changes local prices.
func (s *ConfirmedReturnStore) AbandonLegacyDHPrice(ctx context.Context, id string, req LegacyDHPriceAbandonmentRequest) error {
	if _, err := ownedScope(ctx, s.db, id); err != nil {
		return err
	}
	if req.ExpectedAttemptID == "" || req.DHInventoryID <= 0 || req.CertNumber == "" || req.Grader == "" || req.FrozenPayloadIdentity == "" || req.ApprovedListingPriceCents <= 0 || strings.TrimSpace(req.OperatorAuthorization) == "" || strings.TrimSpace(req.QuiescenceEvidence) == "" {
		return inventory.NewReturnConflict("invalid_legacy_abandonment", "exact attempt identity, positive approved price, authorization and quiescence evidence required")
	}
	state, err := s.returnState(ctx, id)
	if err != nil {
		return err
	}
	p := state.Purchase
	if state.Sale != nil || state.Operation != nil || state.AwaitingListing || p.DHSaleConflict != "" || p.DHSaleConflictAt != nil || p.ReceivedAt == nil || p.DHStatus != inventory.DHStatusInStock || inventory.ResolveListingPriceCents(p) != req.ApprovedListingPriceCents {
		return inventory.NewReturnConflict("legacy_abandonment_precondition", "received in-stock unsold target without return or sale conflict and unchanged approved price required")
	}
	a := state.PrecedingAttempt
	if p.DHInventoryID != req.DHInventoryID || p.CertNumber != req.CertNumber || p.Grader != req.Grader || a == nil || a.ID != req.ExpectedAttemptID || a.CapturedPurchaseID != id || a.DHInventoryID != req.DHInventoryID || a.CertNumber != req.CertNumber || a.Grader != req.Grader || a.Key != "" || a.Kind != "price" || a.Phase != "patch" || a.ReturnOperationID != "" || a.Outcome != "open" || a.PayloadIdentity != req.FrozenPayloadIdentity {
		return inventory.NewReturnConflict("legacy_abandonment_identity", "only the exact open unkeyed legacy price PATCH may be abandoned")
	}
	// Reuse the full execution guard so this administrative path cannot bypass
	// the coordinator's retained-target fences or authorize unrelated writers.
	if err := s.OwnDHMutation(ctx, id, a); err != nil {
		return err
	}
	receipt, err := json.Marshal(struct {
		Type                      string `json:"type"`
		OriginalProviderOutcome   string `json:"original_provider_outcome"`
		OperatorAuthorization     string `json:"operator_authorization"`
		QuiescenceEvidence        string `json:"quiescence_evidence"`
		ApprovedListingPriceCents int    `json:"approved_listing_price_cents"`
	}{"administrative_abandonment", "unknown", req.OperatorAuthorization, req.QuiescenceEvidence, req.ApprovedListingPriceCents})
	if err != nil {
		return err
	}
	// Preserve any existing provider evidence, even on a still-open attempt.
	// Recheck frozen fields in the CAS; only outcome, settlement time and the
	// originally empty receipt are written. Nothing is rekeyed or replayed.
	result, err := executor(ctx, s.db).ExecContext(ctx, `UPDATE dh_mutation_attempts SET outcome='abandoned',settled_at=clock_timestamp(),receipt=$1
	 WHERE id=$2 AND captured_purchase_id=$3 AND dh_inventory_id=$4 AND cert_number=$5 AND grader=$6
	 AND kind='price' AND phase='patch' AND idempotency_key='' AND return_operation_id IS NULL
	 AND outcome='open' AND payload_identity=$7 AND receipt=''`, string(receipt), req.ExpectedAttemptID, id, req.DHInventoryID, req.CertNumber, req.Grader, req.FrozenPayloadIdentity)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return inventory.NewReturnConflict("legacy_abandonment_conflict", "attempt changed, is no longer open or retains receipt evidence")
	}
	return s.watermark(ctx, p, "mutation_settled_at")
}
