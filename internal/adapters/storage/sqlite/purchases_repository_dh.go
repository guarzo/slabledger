package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// UpdatePurchaseDHFields updates DH v2 tracking fields on a purchase.
func (r *CampaignsRepository) UpdatePurchaseDHFields(ctx context.Context, id string, update campaigns.DHFieldsUpdate) error {
	return r.execAndExpectRow(ctx, "update DH fields",
		`UPDATE campaign_purchases
		 SET dh_card_id = ?, dh_inventory_id = ?, dh_cert_status = ?,
		     dh_listing_price_cents = ?, dh_channels_json = ?, dh_status = ?, updated_at = ?
		 WHERE id = ?`,
		update.CardID, update.InventoryID, update.CertStatus, update.ListingPriceCents, update.ChannelsJSON, update.DHStatus, time.Now(), id,
	)
}

// UpdatePurchaseDHFieldsAndPushStatus atomically updates DH tracking fields
// and the push status in a single UPDATE to prevent inconsistent state where
// fields are saved (inventory ID set) but the status remains "pending".
func (r *CampaignsRepository) UpdatePurchaseDHFieldsAndPushStatus(ctx context.Context, id string, update campaigns.DHFieldsUpdate, pushStatus string) error {
	return r.execAndExpectRow(ctx, "update DH fields + push status",
		`UPDATE campaign_purchases
		 SET dh_card_id = ?, dh_inventory_id = ?, dh_cert_status = ?,
		     dh_listing_price_cents = ?, dh_channels_json = ?, dh_status = ?,
		     dh_push_status = ?, updated_at = ?
		 WHERE id = ?`,
		update.CardID, update.InventoryID, update.CertStatus, update.ListingPriceCents, update.ChannelsJSON, update.DHStatus,
		pushStatus, time.Now(), id,
	)
}

// GetPurchasesByDHCertStatus returns purchases with the given DH cert resolution status.
func (r *CampaignsRepository) GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases WHERE dh_cert_status = ? ORDER BY updated_at ASC LIMIT ?`,
		purchaseColumns,
	)
	rows, err := r.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (campaigns.Purchase, error) {
		var p campaigns.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

// UpdatePurchaseDHPushStatus updates the dh_push_status field on a purchase.
func (r *CampaignsRepository) UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error {
	return r.execAndExpectRow(ctx, "update DH push status",
		`UPDATE campaign_purchases SET dh_push_status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now(), id,
	)
}

// UpdatePurchaseDHCandidates stores disambiguation candidates JSON on a purchase.
func (r *CampaignsRepository) UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error {
	return r.execAndExpectRow(ctx, "update DH candidates",
		`UPDATE campaign_purchases SET dh_candidates = ?, updated_at = ? WHERE id = ?`,
		candidatesJSON, time.Now(), id,
	)
}

// UpdatePurchaseDHHoldReason stores the hold reason on a purchase.
func (r *CampaignsRepository) UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error {
	return r.execAndExpectRow(ctx, "update DH hold reason",
		`UPDATE campaign_purchases SET dh_hold_reason = ?, updated_at = ? WHERE id = ?`,
		reason, time.Now(), id,
	)
}

// SetHeldWithReason atomically sets the push status to held and records
// the hold reason in a single UPDATE, preventing any reader from observing
// a held purchase with an empty reason.
func (r *CampaignsRepository) SetHeldWithReason(ctx context.Context, purchaseID string, reason string) error {
	return r.execAndExpectRow(ctx, "set held with reason",
		`UPDATE campaign_purchases SET dh_push_status = ?, dh_hold_reason = ?, updated_at = ? WHERE id = ?`,
		campaigns.DHPushStatusHeld, reason, time.Now(), purchaseID,
	)
}

// ApproveHeldPurchase atomically clears the hold reason and sets the push
// status to pending inside a single transaction.
func (r *CampaignsRepository) ApproveHeldPurchase(ctx context.Context, purchaseID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()
	result, err := tx.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET dh_push_status = ?, dh_hold_reason = '', updated_at = ?
		 WHERE id = ?`,
		campaigns.DHPushStatusPending, now, purchaseID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return tx.Commit()
}

// GetPurchasesByDHPushStatus returns purchases with the given DH push status.
func (r *CampaignsRepository) GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases WHERE dh_push_status = ? ORDER BY updated_at ASC LIMIT ?`,
		purchaseColumns,
	)
	rows, err := r.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (campaigns.Purchase, error) {
		var p campaigns.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

// CountUnsoldByDHPushStatus returns counts of unsold purchases grouped by dh_push_status.
// Purchases with no status are reported under an empty string key; purchases with a
// DHCardID but no push status are counted as "matched" for legacy compatibility.
func (r *CampaignsRepository) CountUnsoldByDHPushStatus(ctx context.Context) (map[string]int, error) {
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
	rows, err := r.db.QueryContext(ctx, query)
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

// UpdatePurchaseGemRateID sets the CL gemRateID on a purchase.
func (r *CampaignsRepository) UpdatePurchaseGemRateID(ctx context.Context, id, gemRateID string) error {
	return r.execAndExpectRow(ctx, "update gem rate ID",
		`UPDATE campaign_purchases SET gem_rate_id = ?, updated_at = ? WHERE id = ?`,
		gemRateID, time.Now(), id,
	)
}

// UpdatePurchasePSASpecID sets the PSA spec ID on a purchase.
func (r *CampaignsRepository) UpdatePurchasePSASpecID(ctx context.Context, id string, psaSpecID int) error {
	return r.execAndExpectRow(ctx, "update PSA spec ID",
		`UPDATE campaign_purchases SET psa_spec_id = ?, updated_at = ? WHERE id = ?`,
		psaSpecID, time.Now(), id,
	)
}

// UpdatePurchaseCLCardMetadata persists Card Ladder catalog metadata (player, variation, category) on a purchase.
func (r *CampaignsRepository) UpdatePurchaseCLCardMetadata(ctx context.Context, id, player, variation, category string) error {
	return r.execAndExpectRow(ctx, "update CL card metadata",
		`UPDATE campaign_purchases SET card_player = ?, card_variation = ?, card_category = ?, updated_at = ? WHERE id = ?`,
		player, variation, category, time.Now(), id,
	)
}

// GetPurchaseIDByCertNumber returns the purchase ID for a given cert number.
func (r *CampaignsRepository) GetPurchaseIDByCertNumber(ctx context.Context, certNumber string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM campaign_purchases WHERE cert_number = ?`, certNumber,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
