package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"time"
)

func (ps *PurchaseStore) UpdatePurchaseDHFields(ctx context.Context, id string, update inventory.DHFieldsUpdate) error {
	return ps.execAndExpectRow(ctx, "update DH fields",
		`UPDATE campaign_purchases
		 SET dh_card_id = ?, dh_inventory_id = ?, dh_cert_status = ?,
		     dh_listing_price_cents = ?, dh_channels_json = ?, dh_status = ?,
		     dh_last_synced_at = ?, updated_at = ?
		 WHERE id = ?`,
		update.CardID, update.InventoryID, update.CertStatus, update.ListingPriceCents, update.ChannelsJSON, update.DHStatus, update.LastSyncedAt, time.Now(), id,
	)
}

// UpdatePurchaseDHFieldsAndPushStatus atomically updates DH tracking fields
// and the push status in a single UPDATE to prevent inconsistent state where
// fields are saved (inventory ID set) but the status remains "pending".
func (ps *PurchaseStore) UpdatePurchaseDHFieldsAndPushStatus(ctx context.Context, id string, update inventory.DHFieldsUpdate, pushStatus string) error {
	return ps.execAndExpectRow(ctx, "update DH fields + push status",
		`UPDATE campaign_purchases
		 SET dh_card_id = ?, dh_inventory_id = ?, dh_cert_status = ?,
		     dh_listing_price_cents = ?, dh_channels_json = ?, dh_status = ?,
		     dh_last_synced_at = ?, dh_push_status = ?, updated_at = ?
		 WHERE id = ?`,
		update.CardID, update.InventoryID, update.CertStatus, update.ListingPriceCents,
		update.ChannelsJSON, update.DHStatus, update.LastSyncedAt,
		pushStatus, time.Now(), id,
	)
}

// GetPurchasesByDHCertStatus returns purchases with the given DH cert resolution status.
func (ps *PurchaseStore) GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases WHERE dh_cert_status = ? ORDER BY updated_at ASC LIMIT ?`,
		purchaseColumns,
	)
	rows, err := ps.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

// UpdatePurchaseDHPushStatus updates the dh_push_status field on a purchase.
func (ps *PurchaseStore) UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error {
	return ps.execAndExpectRow(ctx, "update DH push status",
		`UPDATE campaign_purchases SET dh_push_status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now(), id,
	)
}

// UpdatePurchaseDHCandidates stores disambiguation candidates JSON on a purchase.
func (ps *PurchaseStore) UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error {
	return ps.execAndExpectRow(ctx, "update DH candidates",
		`UPDATE campaign_purchases SET dh_candidates = ?, updated_at = ? WHERE id = ?`,
		candidatesJSON, time.Now(), id,
	)
}

// UpdatePurchaseDHHoldReason stores the hold reason on a purchase.
func (ps *PurchaseStore) UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error {
	return ps.execAndExpectRow(ctx, "update DH hold reason",
		`UPDATE campaign_purchases SET dh_hold_reason = ?, updated_at = ? WHERE id = ?`,
		reason, time.Now(), id,
	)
}

// SetHeldWithReason atomically sets the push status to held and records
// the hold reason in a single UPDATE, preventing any reader from observing
// a held purchase with an empty reason.
func (ps *PurchaseStore) SetHeldWithReason(ctx context.Context, purchaseID string, reason string) error {
	return ps.execAndExpectRow(ctx, "set held with reason",
		`UPDATE campaign_purchases SET dh_push_status = ?, dh_hold_reason = ?, updated_at = ? WHERE id = ?`,
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
		 SET dh_push_status = ?, dh_hold_reason = '', updated_at = ?
		 WHERE id = ?`,
		inventory.DHPushStatusPending, now, purchaseID,
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
		     dh_push_status = ?,
		     dh_status = '',
		     dh_listing_price_cents = 0,
		     dh_channels_json = '[]',
		     dh_hold_reason = '',
		     updated_at = ?
		 WHERE id = ?`,
		inventory.DHPushStatusPending, time.Now(), purchaseID,
	)
}

// GetPurchasesByDHPushStatus returns received, unsold purchases with the given DH push status.
// Items that have not yet been received (received_at IS NULL) are excluded so that only
// physically-in-hand inventory is sent to DH.
func (ps *PurchaseStore) GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases p
		 LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		 WHERE p.dh_push_status = ?
		   AND p.received_at IS NOT NULL
		   AND s.id IS NULL
		 ORDER BY p.updated_at ASC LIMIT ?`,
		purchaseColumnsAliased,
	)
	rows, err := ps.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

// CountUnsoldByDHPushStatus returns counts of unsold purchases grouped by dh_push_status.
// Purchases with no status are reported under an empty string key; purchases with a
// DHCardID but no push status are counted as "matched" for legacy compatibility.
func (ps *PurchaseStore) CountUnsoldByDHPushStatus(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT
			CASE
				WHEN p.dh_push_status != '' THEN p.dh_push_status
				WHEN p.dh_card_id != 0 THEN 'matched'
				ELSE ''
			END AS status_bucket,
			COUNT(*) AS cnt
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
		GROUP BY status_bucket`
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	counts := make(map[string]int)
	for rows.Next() {
		var bucket string
		var cnt int
		if err := rows.Scan(&bucket, &cnt); err != nil {
			return nil, err
		}
		counts[bucket] = cnt
	}
	return counts, rows.Err()
}

// CountDHPipelineHealth returns the two counts the DH status dashboard needs to
// reconcile its aggregate with the actual queue: how many received-and-pending
// rows there are (matches ListDHPendingItems' draining query) and how many
// received rows have never been enrolled in the push pipeline at all.
// Both counts exclude sold rows and rows in closed campaigns.
func (ps *PurchaseStore) CountDHPipelineHealth(ctx context.Context) (inventory.DHPipelineHealth, error) {
	var health inventory.DHPipelineHealth
	err := ps.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN p.dh_push_status = 'pending' THEN 1 ELSE 0 END), 0) AS pending_received,
			COALESCE(SUM(CASE
				WHEN (p.dh_push_status IS NULL OR p.dh_push_status = '')
				     AND p.dh_inventory_id = 0
				THEN 1 ELSE 0 END), 0) AS unenrolled_received
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL
		  AND p.received_at IS NOT NULL
		  AND c.phase != 'closed'
	`).Scan(&health.PendingReceived, &health.UnenrolledReceived)
	if err != nil {
		return inventory.DHPipelineHealth{}, fmt.Errorf("count dh pipeline health: %w", err)
	}
	return health, nil
}

// ListDHPendingItems returns received, unsold purchases that are queued for the DH push
// pipeline (dh_push_status = 'pending') with an active parent campaign. Each row carries
// a confidence label based on when DH inventory was last synced for the purchase.
func (ps *PurchaseStore) ListDHPendingItems(ctx context.Context) ([]inventory.DHPendingItem, error) {
	query := `
		SELECT p.id, p.card_name, p.set_name, p.grade_value,
		       p.mid_price_cents, p.dh_last_synced_at, p.created_at
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.dh_push_status = 'pending'
		  AND p.received_at IS NOT NULL
		  AND s.id IS NULL
		  AND c.phase != 'closed'
		ORDER BY p.updated_at DESC`
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query dh pending items: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	items := make([]inventory.DHPendingItem, 0)
	now := time.Now()
	for rows.Next() {
		var (
			purchaseID     string
			cardName       string
			setName        string
			grade          float64
			midPriceCents  int
			dhLastSyncedAt string
			createdAt      time.Time
		)
		if err := rows.Scan(&purchaseID, &cardName, &setName, &grade,
			&midPriceCents, &dhLastSyncedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scan dh pending item: %w", err)
		}

		confidence := "low"
		var daysQueued int
		if dhLastSyncedAt != "" {
			if syncedAt, parseErr := time.Parse(time.RFC3339, dhLastSyncedAt); parseErr == nil {
				elapsed := now.Sub(syncedAt)
				daysQueued = int(elapsed.Hours() / 24)
				switch {
				case elapsed < 24*time.Hour:
					confidence = "high"
				case elapsed < 7*24*time.Hour:
					confidence = "medium"
				default:
					confidence = "low"
				}
			} else {
				// Unparseable timestamp — fall back to created_at age.
				daysQueued = int(now.Sub(createdAt).Hours() / 24)
			}
		} else {
			daysQueued = int(now.Sub(createdAt).Hours() / 24)
		}

		items = append(items, inventory.DHPendingItem{
			PurchaseID:            purchaseID,
			CardName:              cardName,
			SetName:               setName,
			Grade:                 grade,
			RecommendedPriceCents: midPriceCents,
			DaysQueued:            daysQueued,
			DHConfidence:          confidence,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dh pending items: %w", err)
	}
	return items, nil
}

// UpdatePurchaseGemRateID sets the CL gemRateID on a purchase.
func (ps *PurchaseStore) UpdatePurchaseGemRateID(ctx context.Context, id, gemRateID string) error {
	return ps.execAndExpectRow(ctx, "update gem rate ID",
		`UPDATE campaign_purchases SET gem_rate_id = ?, updated_at = ? WHERE id = ?`,
		gemRateID, time.Now(), id,
	)
}

// UpdatePurchasePSASpecID sets the PSA spec ID on a purchase.
func (ps *PurchaseStore) UpdatePurchasePSASpecID(ctx context.Context, id string, psaSpecID int) error {
	return ps.execAndExpectRow(ctx, "update PSA spec ID",
		`UPDATE campaign_purchases SET psa_spec_id = ?, updated_at = ? WHERE id = ?`,
		psaSpecID, time.Now(), id,
	)
}

// ListUnsoldDHCardIDs returns the distinct dh_card_id values for unsold purchases
// in non-closed campaigns. Used by the DH demand analytics refresh scheduler to
// seed per-card analytics calls with our own inventory. Rows where dh_card_id is
// zero or NULL are skipped.
func (ps *PurchaseStore) ListUnsoldDHCardIDs(ctx context.Context) ([]int, error) {
	query := `
		SELECT DISTINCT p.dh_card_id
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL
		  AND c.phase != 'closed'
		  AND p.dh_card_id IS NOT NULL
		  AND p.dh_card_id != 0`
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list unsold dh card ids: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan dh card id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dh card ids: %w", err)
	}
	return ids, nil
}

// UpdatePurchaseCLCardMetadata persists Card Ladder catalog metadata (player, variation, category) on a purchase.
func (ps *PurchaseStore) UpdatePurchaseCLCardMetadata(ctx context.Context, id, player, variation, category string) error {
	return ps.execAndExpectRow(ctx, "update CL card metadata",
		`UPDATE campaign_purchases SET card_player = ?, card_variation = ?, card_category = ?, updated_at = ? WHERE id = ?`,
		player, variation, category, time.Now(), id,
	)
}
