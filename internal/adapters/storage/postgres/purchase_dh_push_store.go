package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func (ps *PurchaseStore) UpdatePurchaseDHFields(ctx context.Context, id string, update inventory.DHFieldsUpdate) error {
	return ps.execAndExpectRow(ctx, "update DH fields",
		`UPDATE campaign_purchases
		 SET dh_card_id = $1, dh_inventory_id = $2, dh_cert_status = $3,
		     dh_listing_price_cents = $4, dh_channels_json = $5, dh_status = $6,
		     dh_last_synced_at = $7, updated_at = $8
		 WHERE id = $9`,
		update.CardID, update.InventoryID, update.CertStatus, update.ListingPriceCents, update.ChannelsJSON, update.DHStatus, update.LastSyncedAt, time.Now(), id,
	)
}

// UpdatePurchaseDHFieldsAndPushStatus atomically updates DH tracking fields
// and the push status in a single UPDATE to prevent inconsistent state where
// fields are saved (inventory ID set) but the status remains "pending".
func (ps *PurchaseStore) UpdatePurchaseDHFieldsAndPushStatus(ctx context.Context, id string, update inventory.DHFieldsUpdate, pushStatus string) error {
	return ps.execAndExpectRow(ctx, "update DH fields + push status",
		`UPDATE campaign_purchases
		 SET dh_card_id = $1, dh_inventory_id = $2, dh_cert_status = $3,
		     dh_listing_price_cents = $4, dh_channels_json = $5, dh_status = $6,
		     dh_last_synced_at = $7, dh_push_status = $8, updated_at = $9
		 WHERE id = $10`,
		update.CardID, update.InventoryID, update.CertStatus, update.ListingPriceCents,
		update.ChannelsJSON, update.DHStatus, update.LastSyncedAt,
		pushStatus, time.Now(), id,
	)
}

// UnmatchPurchaseDH atomically clears all DH tracking fields and sets the push
// status in a single UPDATE. dh_push_attempts is reset to 0 for "pending" and
// "matched" transitions so the re-enrolled purchase starts with a clean retry
// budget; "unmatched" preserves the counter as a diagnostic signal.
func (ps *PurchaseStore) UnmatchPurchaseDH(ctx context.Context, purchaseID string, pushStatus string) error {
	return ps.execAndExpectRow(ctx, "unmatch purchase DH",
		`UPDATE campaign_purchases
		 SET dh_card_id           = 0,
		     dh_inventory_id      = 0,
		     dh_cert_status       = '',
		     dh_listing_price_cents = 0,
		     dh_channels_json     = '',
		     dh_status            = '',
		     dh_last_synced_at    = '',
		     dh_push_status       = $1,
		     dh_push_attempts     = CASE WHEN $2 IN ('pending', 'matched', 'unmatched_created', 'override_corrected', 'already_listed') THEN 0 ELSE dh_push_attempts END,
		     updated_at           = $3
		 WHERE id = $4`,
		pushStatus, pushStatus, time.Now(), purchaseID,
	)
}

// UpdatePurchaseDHPushStatus updates the dh_push_status field on a purchase.
// When the new status is 'pending' or 'matched' the attempt counter is reset
// to 0 in the same UPDATE so a successful resolve or a fresh re-enrollment
// starts a clean retry budget. Transitions to 'unmatched' preserve the counter
// as diagnostic signal for how many cycles were wasted before we gave up.
func (ps *PurchaseStore) UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error {
	return ps.execAndExpectRow(ctx, "update DH push status",
		`UPDATE campaign_purchases
		 SET dh_push_status = $1,
		     dh_push_attempts = CASE WHEN $2 IN ('pending', 'matched', 'unmatched_created', 'override_corrected', 'already_listed') THEN 0 ELSE dh_push_attempts END,
		     updated_at = $3
		 WHERE id = $4`,
		status, status, time.Now(), id,
	)
}

// IncrementDHPushAttempts atomically increments the skip-attempt counter and
// returns the new value. The scheduler uses the returned count to decide when
// a cert has been retried enough times to warrant moving it out of 'pending'
// and into 'unmatched', where it becomes user-actionable.
func (ps *PurchaseStore) IncrementDHPushAttempts(ctx context.Context, id string) (int, error) {
	var newCount int
	err := ps.db.QueryRowContext(ctx,
		`UPDATE campaign_purchases
		 SET dh_push_attempts = dh_push_attempts + 1,
		     updated_at = $1
		 WHERE id = $2
		 RETURNING dh_push_attempts`,
		time.Now(), id,
	).Scan(&newCount)
	if err != nil {
		return 0, fmt.Errorf("increment dh push attempts: %w", err)
	}
	return newCount, nil
}

// UpdatePurchaseDHStatus updates only the dh_status column on a purchase.
// Targeted update — does not touch other DH fields.
func (ps *PurchaseStore) UpdatePurchaseDHStatus(ctx context.Context, id string, status string) error {
	return ps.execAndExpectRow(ctx, "update DH status",
		`UPDATE campaign_purchases SET dh_status = $1, updated_at = $2 WHERE id = $3`,
		status, time.Now(), id,
	)
}

// ListStaleDHStatusSoldPurchases returns IDs of purchases that have a linked
// sale but whose dh_status is not 'sold'.
func (ps *PurchaseStore) ListStaleDHStatusSoldPurchases(ctx context.Context) ([]string, error) {
	rows, err := ps.db.QueryContext(ctx,
		`SELECT DISTINCT cp.id FROM campaign_purchases cp
		 JOIN campaign_sales cs ON cs.purchase_id = cp.id
		 WHERE cp.dh_status != '' AND cp.dh_status IS DISTINCT FROM 'sold'`)
	if err != nil {
		return nil, fmt.Errorf("list stale dh status sold purchases: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stale dh status purchase id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stale dh status sold purchases: rows iteration: %w", err)
	}
	return ids, nil
}

// UpdatePurchaseDHCardID updates only the dh_card_id column on a purchase.
// Targeted update — does not touch other DH fields.
func (ps *PurchaseStore) UpdatePurchaseDHCardID(ctx context.Context, id string, cardID int) error {
	return ps.execAndExpectRow(ctx, "update DH card id",
		`UPDATE campaign_purchases SET dh_card_id = $1, updated_at = $2 WHERE id = $3`,
		cardID, time.Now(), id,
	)
}

// UpdatePurchaseDHCandidates stores disambiguation candidates JSON on a purchase.
func (ps *PurchaseStore) UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error {
	return ps.execAndExpectRow(ctx, "update DH candidates",
		`UPDATE campaign_purchases SET dh_candidates = $1, updated_at = $2 WHERE id = $3`,
		candidatesJSON, time.Now(), id,
	)
}

// UpdatePurchaseDHHoldReason stores the hold reason on a purchase.
func (ps *PurchaseStore) UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error {
	return ps.execAndExpectRow(ctx, "update DH hold reason",
		`UPDATE campaign_purchases SET dh_hold_reason = $1, updated_at = $2 WHERE id = $3`,
		reason, time.Now(), id,
	)
}

// SetHeldWithReason atomically sets the push status to held and records
// the hold reason in a single UPDATE, preventing any reader from observing
// a held purchase with an empty reason.
func (ps *PurchaseStore) SetHeldWithReason(ctx context.Context, purchaseID string, reason string) error {
	if reason == "" {
		return fmt.Errorf("SetHeldWithReason: reason must not be empty")
	}
	return ps.execAndExpectRow(ctx, "set held with reason",
		`UPDATE campaign_purchases SET dh_push_status = $1, dh_hold_reason = $2, updated_at = $3 WHERE id = $4`,
		inventory.DHPushStatusHeld, reason, time.Now(), purchaseID,
	)
}

// ApproveHeldPurchase atomically clears the hold reason and sets the push
// status to pending inside a single transaction.
func (ps *PurchaseStore) ApproveHeldPurchase(ctx context.Context, purchaseID string) error {
	tx, err := ps.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()
	result, err := tx.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET dh_push_status = $1, dh_hold_reason = '', dh_push_attempts = 0, updated_at = $2
		 WHERE id = $3 AND dh_push_status = $4`,
		inventory.DHPushStatusPending, now, purchaseID, inventory.DHPushStatusHeld,
	)
	if err != nil {
		return fmt.Errorf("approve held purchase: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return tx.Commit()
}

// ResetDHFieldsForRepush atomically clears the DH inventory linkage and sets
// push status to pending. dh_card_id, dh_cert_status, and dh_candidates are
// preserved so cert resolution from the prior cycle can be reused.
// dh_hold_reason is cleared to match ApproveHeldPurchase's invariant that a
// pending row never carries a stale hold reason.
func (ps *PurchaseStore) ResetDHFieldsForRepush(ctx context.Context, purchaseID string) error {
	return ps.execAndExpectRow(ctx, "reset DH fields for repush",
		`UPDATE campaign_purchases
		 SET dh_inventory_id = 0,
		     dh_push_status = $1,
		     dh_push_attempts = 0,
		     dh_status = '',
		     dh_listing_price_cents = 0,
		     dh_channels_json = '[]',
		     dh_hold_reason = '',
		     updated_at = $2
		 WHERE id = $3`,
		inventory.DHPushStatusPending, time.Now(), purchaseID,
	)
}

// ResetDHFieldsForRepushDueToDelete mirrors ResetDHFieldsForRepush (clears
// dh_inventory_id, dh_status, dh_listing_price_cents, dh_channels_json,
// dh_hold_reason; sets dh_push_status='pending'; preserves reviewed_price_cents,
// dh_card_id, dh_cert_status, dh_candidates) and additionally stamps dh_unlisted_detected_at
// so the UI can badge the row as "unlisted on DH — will be re-pushed." Called by
// the DH reconciler when a purchase's dh_inventory_id is missing from the
// authoritative DH inventory snapshot.
func (ps *PurchaseStore) ResetDHFieldsForRepushDueToDelete(ctx context.Context, purchaseID string) error {
	now := time.Now()
	return ps.execAndExpectRow(ctx, "reset DH fields for repush (DH delete)",
		`UPDATE campaign_purchases
		 SET dh_inventory_id = 0,
		     dh_push_status = $1,
		     dh_push_attempts = 0,
		     dh_status = '',
		     dh_listing_price_cents = 0,
		     dh_channels_json = '[]',
		     dh_hold_reason = '',
		     dh_unlisted_detected_at = $2,
		     updated_at = $3
		 WHERE id = $4`,
		inventory.DHPushStatusPending, now, now, purchaseID,
	)
}

// ClearDHUnlistedDetectedAt nulls out dh_unlisted_detected_at. Called by the
// listing service when a purchase successfully transitions to 'listed' so the
// UI badge disappears after a successful re-list.
func (ps *PurchaseStore) ClearDHUnlistedDetectedAt(ctx context.Context, purchaseID string) error {
	return ps.execAndExpectRow(ctx, "clear dh_unlisted_detected_at",
		`UPDATE campaign_purchases
		 SET dh_unlisted_detected_at = NULL,
		     updated_at = $1
		 WHERE id = $2`,
		time.Now(), purchaseID,
	)
}

// SetDHSaleConflict flags a purchase for human review after a non-retryable
// DH sale-recording error, or an apparent success with delisted == false
// (spec §4). ListSalesNeedingDHRecord excludes flagged purchases until the
// flag is cleared, which is also how a resolved conflict is re-driven.
func (ps *PurchaseStore) SetDHSaleConflict(ctx context.Context, purchaseID, reason string) error {
	now := time.Now()
	return ps.execAndExpectRow(ctx, "set dh sale conflict",
		`UPDATE campaign_purchases
		 SET dh_sale_conflict = $1, dh_sale_conflict_at = $2, updated_at = $3
		 WHERE id = $4`,
		reason, now, now, purchaseID,
	)
}

// ClearDHSaleConflict clears a previously flagged conflict, re-enrolling the
// row in ListSalesNeedingDHRecord on the next recovery pass.
func (ps *PurchaseStore) ClearDHSaleConflict(ctx context.Context, purchaseID string) error {
	return ps.execAndExpectRow(ctx, "clear dh sale conflict",
		`UPDATE campaign_purchases
		 SET dh_sale_conflict = '', dh_sale_conflict_at = NULL, updated_at = $1
		 WHERE id = $2`,
		time.Now(), purchaseID,
	)
}

// ResetDHFieldsForRelistAfterVoid mirrors ResetDHFieldsForRepushDueToDelete
// (spec §7) but PRESERVES dh_inventory_id: a void keeps the DH inventory row
// alive, so a fresh push here would create a duplicate rather than relisting
// the same row. dh_status is set to in_stock — the void already returned the
// item to that state on DH's side — and dh_unlisted_detected_at is reused
// (deliberately, per spec §7) as the signal the auto-relist branch in
// dh_push.go:248 keys on, exactly as the delete-driven reset does.
// dh_push_attempts, dh_listing_price_cents, and dh_hold_reason are also
// cleared, as they are on the delete-driven reset: otherwise a purchase with
// an exhausted retry budget or a stale hold reason re-enters the pending
// relist path unable to actually relist.
func (ps *PurchaseStore) ResetDHFieldsForRelistAfterVoid(ctx context.Context, purchaseID string) error {
	now := time.Now()
	return ps.execAndExpectRow(ctx, "reset DH fields for relist after void",
		`UPDATE campaign_purchases
		 SET dh_push_status = $1,
		     dh_push_attempts = 0,
		     dh_status = $2,
		     dh_listing_price_cents = 0,
		     dh_channels_json = '[]',
		     dh_hold_reason = '',
		     dh_unlisted_detected_at = $3,
		     updated_at = $4
		 WHERE id = $5`,
		inventory.DHPushStatusPending, inventory.DHStatusInStock, now, now, purchaseID,
	)
}
