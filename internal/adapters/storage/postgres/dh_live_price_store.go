package postgres

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// ListPendingDHLivePrices discovers attached open keyed price attempts before
// ordinary drift. Local sold/nonlisted state and cooldown do not hide receipts
// still needing replay; exact identity and cooldown are checked under ownership.
func (s *PurchaseStore) ListPendingDHLivePrices(ctx context.Context) ([]string, error) {
	rows, err := executor(ctx, s.db).QueryContext(ctx, `SELECT DISTINCT p.id FROM dh_mutation_attempts a JOIN campaign_purchases p ON p.id=a.purchase_id WHERE a.kind='price' AND a.phase='live_price' AND a.idempotency_key<>'' AND a.outcome='open' ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

var _ inventory.DHMutationRetryRepository = (*ConfirmedReturnStore)(nil)

// DeferDHMutation records uncertainty, not a receipt. Exact execution ownership
// is required even when this is a fresh transaction after execution rolled back.
func (s *ConfirmedReturnStore) DeferDHMutation(ctx context.Context, id string, a *inventory.DHMutationAttempt, notBefore time.Time) error {
	scope, err := ownedScope(ctx, s.db, id)
	if err != nil {
		return err
	}
	if a == nil || scope.attemptID != a.ID || notBefore.IsZero() {
		return inventory.NewReturnConflict("identity_conflict", "cooldown requires an exact owned open attempt and nonzero deadline")
	}
	if err := s.OwnDHMutation(ctx, id, a); err != nil {
		return err
	}
	result, err := executor(ctx, s.db).ExecContext(ctx, `UPDATE dh_mutation_attempts SET retry_not_before=GREATEST(retry_not_before,$1) WHERE id=$2 AND outcome='open' AND captured_purchase_id=$3 AND dh_inventory_id=$4 AND cert_number=$5 AND grader=$6 AND kind=$7 AND phase=$8 AND idempotency_key=$9 AND payload_identity=$10 AND COALESCE(return_operation_id,'')=$11`, notBefore, a.ID, id, a.DHInventoryID, a.CertNumber, a.Grader, a.Kind, a.Phase, a.Key, a.PayloadIdentity, a.ReturnOperationID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return inventory.NewReturnConflict("identity_conflict", "cooldown attempt is no longer open or its identity changed")
	}
	return nil
}
