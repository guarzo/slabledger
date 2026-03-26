package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// --- CL Refresh ---

func (r *CampaignsRepository) GetPurchaseByCertNumber(ctx context.Context, grader string, certNumber string) (*campaigns.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE grader = ? AND cert_number = ?`
	var p campaigns.Purchase
	err := scanPurchase(r.db.QueryRowContext(ctx, query, grader, certNumber), &p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, campaigns.ErrPurchaseNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPurchasesByGraderAndCertNumbers retrieves multiple purchases by grader and cert numbers.
// Large inputs are chunked to stay within SQLite's parameter limit.
// Returns a map keyed by cert number.
func (r *CampaignsRepository) GetPurchasesByGraderAndCertNumbers(ctx context.Context, grader string, certNumbers []string) (map[string]*campaigns.Purchase, error) {
	if len(certNumbers) == 0 {
		return make(map[string]*campaigns.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*campaigns.Purchase, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)+1)
		args = append(args, grader)
		for i, cn := range chunk {
			placeholders[i] = "?"
			args = append(args, cn)
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases
			WHERE grader = ? AND cert_number IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			select {
			case <-ctx.Done():
				rows.Close() //nolint:errcheck // best-effort close on ctx cancel
				return nil, ctx.Err()
			default:
			}

			var p campaigns.Purchase
			if err := scanPurchase(rows, &p); err != nil {
				rows.Close() //nolint:errcheck // best-effort close on scan error
				return nil, err
			}
			result[p.CertNumber] = &p
		}
		if err := rows.Err(); err != nil {
			rows.Close() //nolint:errcheck // best-effort close on iteration error
			return nil, err
		}
		if cerr := rows.Close(); cerr != nil {
			return nil, cerr
		}
	}
	return result, nil
}

// GetPurchasesByCertNumbers retrieves purchases by cert numbers across all graders.
// Large inputs are chunked to stay within SQLite's parameter limit.
func (r *CampaignsRepository) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*campaigns.Purchase, error) {
	if len(certNumbers) == 0 {
		return make(map[string]*campaigns.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*campaigns.Purchase, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, cn := range chunk {
			placeholders[i] = "?"
			args[i] = cn
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases
			WHERE cert_number IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			select {
			case <-ctx.Done():
				rows.Close() //nolint:errcheck // best-effort close on ctx cancel
				return nil, ctx.Err()
			default:
			}

			var p campaigns.Purchase
			if err := scanPurchase(rows, &p); err != nil {
				rows.Close() //nolint:errcheck // best-effort close on scan error
				return nil, err
			}
			result[p.CertNumber] = &p
		}
		if err := rows.Err(); err != nil {
			rows.Close() //nolint:errcheck // best-effort close on iteration error
			return nil, err
		}
		if cerr := rows.Close(); cerr != nil {
			return nil, cerr
		}
	}

	return result, nil
}

// GetPurchasesByIDs retrieves multiple purchases by their IDs in a single query.
// Large inputs are chunked to stay within SQLite's parameter limit.
func (r *CampaignsRepository) GetPurchasesByIDs(ctx context.Context, ids []string) (map[string]*campaigns.Purchase, error) {
	if len(ids) == 0 {
		return make(map[string]*campaigns.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*campaigns.Purchase, len(ids))

	for start := 0; start < len(ids); start += chunkSize {
		end := min(start+chunkSize, len(ids))
		chunk := ids[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE id IN (` + strings.Join(placeholders, ",") + `)`
		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}

		purchases, err := scanRows(ctx, rows, func(rs *sql.Rows) (campaigns.Purchase, error) {
			var p campaigns.Purchase
			err := scanPurchase(rs, &p)
			return p, err
		})
		if err != nil {
			return nil, err
		}
		for i := range purchases {
			result[purchases[i].ID] = &purchases[i]
		}
	}

	return result, nil
}

// UpdateExternalPurchaseFields updates all fields that come from an external import
// (card metadata, grader, grade, cost, value, images).
func (r *CampaignsRepository) UpdateExternalPurchaseFields(ctx context.Context, id string, p *campaigns.Purchase) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET
			card_name = ?, card_number = ?, set_name = ?,
			grader = ?, grade_value = ?,
			buy_cost_cents = ?, cl_value_cents = ?,
			front_image_url = ?, back_image_url = ?,
			updated_at = ?
		WHERE id = ?`,
		p.CardName, p.CardNumber, p.SetName,
		p.Grader, p.GradeValue,
		p.BuyCostCents, p.CLValueCents,
		p.FrontImageURL, p.BackImageURL,
		time.Now(), id,
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

// UpdatePurchaseMarketSnapshot persists refreshed market snapshot fields on a purchase.
func (r *CampaignsRepository) UpdatePurchaseMarketSnapshot(ctx context.Context, id string, snap campaigns.MarketSnapshotData) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET last_sold_cents = ?, lowest_list_cents = ?, conservative_cents = ?,
			median_cents = ?, active_listings = ?, sales_last_30d = ?, trend_30d = ?, snapshot_date = ?,
			snapshot_json = ?, updated_at = ?
		WHERE id = ?`,
		snap.LastSoldCents, snap.LowestListCents, snap.ConservativeCents,
		snap.MedianCents, snap.ActiveListings, snap.SalesLast30d, snap.Trend30d, snap.SnapshotDate,
		snap.SnapshotJSON, time.Now(), id,
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

// --- PSA Fields Update ---

func (r *CampaignsRepository) ListSnapshotPurchasesByStatus(ctx context.Context, status campaigns.SnapshotStatus, limit int) ([]campaigns.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE snapshot_status = ? ORDER BY updated_at ASC LIMIT ?`
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

func (r *CampaignsRepository) UpdatePurchaseSnapshotStatus(ctx context.Context, id string, status campaigns.SnapshotStatus, retryCount int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET snapshot_status = ?, snapshot_retry_count = ?, updated_at = ? WHERE id = ?`,
		status, retryCount, time.Now(), id,
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

func (r *CampaignsRepository) UpdatePurchasePSAFields(ctx context.Context, id string, fields campaigns.PSAUpdateFields) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET vault_status = ?, invoice_date = ?, was_refunded = ?,
			front_image_url = ?, back_image_url = ?, purchase_source = ?, psa_listing_title = ?, updated_at = ?
		WHERE id = ?`,
		fields.VaultStatus, fields.InvoiceDate, fields.WasRefunded,
		fields.FrontImageURL, fields.BackImageURL, fields.PurchaseSource, fields.PSAListingTitle,
		time.Now(), id,
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

// ListPurchasesMissingImages returns purchases with empty front_image_url that have cert numbers.
func (r *CampaignsRepository) ListPurchasesMissingImages(ctx context.Context, limit int) ([]struct {
	ID         string
	CertNumber string
}, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, cert_number FROM campaign_purchases
		 WHERE (front_image_url = '' OR front_image_url IS NULL)
		 AND cert_number <> ''
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []struct {
		ID         string
		CertNumber string
	}
	for rows.Next() {
		var r struct {
			ID         string
			CertNumber string
		}
		if err := rows.Scan(&r.ID, &r.CertNumber); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpdatePurchaseImageURLs updates the front and back image URLs for a purchase.
// Empty values are treated as no-ops — only non-empty fields are written.
func (r *CampaignsRepository) UpdatePurchaseImageURLs(ctx context.Context, id, frontURL, backURL string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var query string
	var args []any

	switch {
	case frontURL != "" && backURL != "":
		query = `UPDATE campaign_purchases SET front_image_url = ?, back_image_url = ?, updated_at = ? WHERE id = ?`
		args = []any{frontURL, backURL, now, id}
	case frontURL != "":
		query = `UPDATE campaign_purchases SET front_image_url = ?, updated_at = ? WHERE id = ?`
		args = []any{frontURL, now, id}
	case backURL != "":
		query = `UPDATE campaign_purchases SET back_image_url = ?, updated_at = ? WHERE id = ?`
		args = []any{backURL, now, id}
	default:
		return nil // nothing to update
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

// --- Price Override ---

func (r *CampaignsRepository) UpdatePurchasePriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error {
	now := time.Now()
	setAt := ""
	if priceCents > 0 {
		setAt = now.Format(time.RFC3339)
	} else {
		source = ""
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET override_price_cents = ?, override_source = ?, override_set_at = ?, updated_at = ? WHERE id = ?`,
		priceCents, source, setAt, now, purchaseID,
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

func (r *CampaignsRepository) UpdateReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	now := time.Now()
	reviewedAt := ""
	if priceCents > 0 {
		reviewedAt = now.Format(time.RFC3339)
	} else {
		source = ""
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET reviewed_price_cents = ?, reviewed_at = ?, review_source = ?, updated_at = ? WHERE id = ?`,
		priceCents, reviewedAt, source, now, purchaseID,
	)
	if err != nil {
		return fmt.Errorf("update reviewed price: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

func (r *CampaignsRepository) UpdatePurchaseAISuggestion(ctx context.Context, purchaseID string, priceCents int) error {
	now := time.Now()
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET ai_suggested_price_cents = ?, ai_suggested_at = ?, updated_at = ? WHERE id = ?`,
		priceCents, now.Format(time.RFC3339), now, purchaseID,
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

func (r *CampaignsRepository) GetPriceOverrideStats(ctx context.Context) (*campaigns.PriceOverrideStats, error) {
	var stats campaigns.PriceOverrideStats
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN p.override_price_cents > 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN p.override_price_cents > 0 AND p.override_source = 'manual' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN p.override_price_cents > 0 AND p.override_source = 'cost_markup' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN p.override_price_cents > 0 AND p.override_source = 'ai_accepted' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN p.ai_suggested_price_cents > 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(p.override_price_cents), 0),
			COALESCE(SUM(p.ai_suggested_price_cents), 0)
		FROM campaign_purchases p
		JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
	`).Scan(
		&stats.TotalUnsold,
		&stats.OverrideCount,
		&stats.ManualCount,
		&stats.CostMarkupCount,
		&stats.AIAcceptedCount,
		&stats.PendingSuggestions,
		&stats.OverrideTotalCents,
		&stats.SuggestionTotalCents,
	)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *CampaignsRepository) ClearPurchaseAISuggestion(ctx context.Context, purchaseID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET ai_suggested_price_cents = 0, ai_suggested_at = '', updated_at = ? WHERE id = ?`,
		time.Now(), purchaseID,
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

func (r *CampaignsRepository) AcceptAISuggestion(ctx context.Context, purchaseID string, priceCents int) error {
	if priceCents <= 0 {
		return campaigns.ErrNoAISuggestion
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()
	setAt := now.Format(time.RFC3339)

	result, err := tx.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET override_price_cents = ?, override_source = 'ai_accepted', override_set_at = ?,
		     ai_suggested_price_cents = 0, ai_suggested_at = '',
		     updated_at = ?
		 WHERE id = ? AND ai_suggested_price_cents = ? AND ai_suggested_at != ''`,
		priceCents, setAt, now, purchaseID, priceCents,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrNoAISuggestion
	}
	return tx.Commit()
}

// --- eBay Export ---

func (r *CampaignsRepository) SetEbayExportFlag(ctx context.Context, purchaseID string, flaggedAt time.Time) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET ebay_export_flagged_at = ? WHERE id = ?`,
		flaggedAt, purchaseID)
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

func (r *CampaignsRepository) ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	const chunkSize = 500
	for start := 0; start < len(purchaseIDs); start += chunkSize {
		end := min(start+chunkSize, len(purchaseIDs))
		chunk := purchaseIDs[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}
		query := `UPDATE campaign_purchases SET ebay_export_flagged_at = NULL WHERE id IN (` + strings.Join(placeholders, ",") + `)`
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *CampaignsRepository) ListEbayFlaggedPurchases(ctx context.Context) ([]campaigns.Purchase, error) {
	query := `SELECT ` + purchaseColumnsAliased + `
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed' AND p.ebay_export_flagged_at IS NOT NULL AND p.grader = 'PSA'
		ORDER BY c.created_at DESC, p.purchase_date DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (campaigns.Purchase, error) {
		var p campaigns.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}
