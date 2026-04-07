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
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET dh_card_id = ?, dh_inventory_id = ?, dh_cert_status = ?,
		     dh_listing_price_cents = ?, dh_channels_json = ?, dh_status = ?, updated_at = ?
		 WHERE id = ?`,
		update.CardID, update.InventoryID, update.CertStatus, update.ListingPriceCents, update.ChannelsJSON, update.DHStatus, time.Now(), id,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
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
	defer rows.Close() //nolint:errcheck // best-effort close

	var purchases []campaigns.Purchase
	for rows.Next() {
		var p campaigns.Purchase
		if err := scanPurchase(rows, &p); err != nil {
			return nil, err
		}
		purchases = append(purchases, p)
	}
	return purchases, rows.Err()
}

// UpdatePurchaseDHPushStatus updates the dh_push_status field on a purchase.
func (r *CampaignsRepository) UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET dh_push_status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now(), id,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

// UpdatePurchaseDHCandidates stores disambiguation candidates JSON on a purchase.
func (r *CampaignsRepository) UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET dh_candidates = ?, updated_at = ? WHERE id = ?`,
		candidatesJSON, time.Now(), id,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
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
	defer rows.Close() //nolint:errcheck // best-effort close

	var purchases []campaigns.Purchase
	for rows.Next() {
		var p campaigns.Purchase
		if err := scanPurchase(rows, &p); err != nil {
			return nil, err
		}
		purchases = append(purchases, p)
	}
	return purchases, rows.Err()
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
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET gem_rate_id = ?, updated_at = ? WHERE id = ?`,
		gemRateID, time.Now(), id,
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
	return nil
}

// UpdatePurchasePSASpecID sets the PSA spec ID on a purchase.
func (r *CampaignsRepository) UpdatePurchasePSASpecID(ctx context.Context, id string, psaSpecID int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET psa_spec_id = ?, updated_at = ? WHERE id = ?`,
		psaSpecID, time.Now(), id,
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
	return nil
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
