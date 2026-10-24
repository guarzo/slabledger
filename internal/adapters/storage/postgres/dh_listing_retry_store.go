package postgres

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

var _ inventory.DHListingRetryRepository = (*ConfirmedReturnStore)(nil)

// EnableDHListingRetry records a known replay boundary, not a terminal receipt.
// Purchase ownership and the exact immutable attempt are required. Current
// target/return guards run before replay, not here: a pre-dispatch guard failure
// must not permanently consume the permission it just borrowed.
func (s *ConfirmedReturnStore) EnableDHListingRetry(ctx context.Context, id string, a *inventory.DHMutationAttempt) error {
	if _, err := ownedScope(ctx, s.db, id); err != nil {
		return err
	}
	if a == nil || a.CapturedPurchaseID != id || a.Kind != "list" || a.Phase != "intake_patch_sync" || a.DHInventoryID <= 0 || a.Key != "" || a.ReturnOperationID != "" {
		return inventory.NewReturnConflict("invalid_listing_retry", "only an exact existing-target listing PATCH can retry")
	}
	result, err := executor(ctx, s.db).ExecContext(ctx, `UPDATE dh_mutation_attempts SET listing_retryable=true WHERE id=$1 AND outcome='open' AND captured_purchase_id=$2 AND dh_inventory_id=$3 AND cert_number=$4 AND grader=$5 AND kind=$6 AND phase=$7 AND idempotency_key=$8 AND payload_identity=$9 AND return_operation_id IS NULL`, a.ID, id, a.DHInventoryID, a.CertNumber, a.Grader, a.Kind, a.Phase, a.Key, a.PayloadIdentity)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return inventory.NewReturnConflict("identity_conflict", "listing attempt is no longer open or its identity changed")
	}
	return nil
}
