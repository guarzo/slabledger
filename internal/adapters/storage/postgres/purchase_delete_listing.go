package postgres

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// A retryable PATCH is a failed listing, not proof that DH made no changes.
// This exception is only for acknowledged local Delete, never generic mutation.
func (s *ConfirmedReturnStore) failedListingDeleteAttempts(ctx context.Context, state *inventory.ConfirmedReturnState) ([]string, error) {
	p := state.Purchase
	if p.DHInventoryID <= 0 || state.Sale != nil || state.Operation != nil || state.AwaitingListing || p.DHSaleConflict != "" || p.DHSaleConflictAt != nil || p.DHStatus == inventory.DHStatusSold {
		return nil, nil
	}
	rows, err := executor(ctx, s.db).QueryContext(ctx, `SELECT id,
 captured_purchase_id=$1 AND dh_inventory_id=$2 AND cert_number=$3 AND grader=$4
 AND kind='list' AND phase='intake_patch_sync' AND listing_retryable
 AND idempotency_key='' AND return_operation_id IS NULL AND receipt='' AND payload_identity<>''
 FROM dh_mutation_attempts WHERE outcome='open' AND
 (captured_purchase_id=$1 OR dh_inventory_id=$2 OR (cert_number=$3 AND grader=$4 AND cert_number<>''))`, p.ID, p.DHInventoryID, p.CertNumber, p.Grader)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	all := true
	for rows.Next() {
		var id string
		var eligible bool
		if err := rows.Scan(&id, &eligible); err != nil {
			return nil, err
		}
		ids = append(ids, id)
		all = all && eligible
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !all {
		return nil, nil
	}
	return ids, nil
}

func (ps *PurchaseStore) withFailedListingDelete(ctx context.Context, id string, remove func(context.Context) error) error {
	s := NewConfirmedReturnStore(ps.db)
	return s.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		state, err := s.returnState(owned, id)
		if err != nil {
			return err
		}
		ids, err := s.failedListingDeleteAttempts(owned, state)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			if !inventory.FailedListingDeleteConfirmed(ctx) {
				return inventory.NewReturnConflict("failed_listing_delete_confirmation_required", "A DH listing update failed. Confirm Delete with the warning; DH may still have the item listed.")
			}
			result, err := executor(owned, ps.db).ExecContext(owned, `UPDATE dh_mutation_attempts SET outcome='abandoned',settled_at=clock_timestamp(),listing_retryable=false,receipt='abandoned on delete; original provider outcome unknown' WHERE id=ANY($1) AND outcome='open' AND listing_retryable`, ids)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != int64(len(ids)) {
				return inventory.NewReturnConflict("attempt_settlement_conflict", "Listing attempts changed before Delete")
			}
		}
		// Re-run the ordinary guard and delete in the SAME transaction. Any failure
		// rolls back abandonment too; other matching attempts are never bypassed.
		return remove(owned)
	})
}
