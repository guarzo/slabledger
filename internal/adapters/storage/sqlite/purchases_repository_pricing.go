package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

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
