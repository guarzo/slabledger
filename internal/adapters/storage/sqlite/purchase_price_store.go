package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"strings"
	"time"
)

func (ps *PurchaseStore) UpdatePurchasePriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error {
	now := time.Now()
	if priceCents <= 0 {
		priceCents = 0
		source = ""
	}
	setAt := ""
	if priceCents > 0 {
		setAt = now.Format(time.RFC3339)
	}
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET override_price_cents = ?, override_source = ?, override_set_at = ?, updated_at = ? WHERE id = ?`,
		priceCents, source, setAt, now, purchaseID,
	)
	if err != nil {
		return fmt.Errorf("update purchase price override: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

func (ps *PurchaseStore) ClearPurchaseAISuggestion(ctx context.Context, purchaseID string) error {
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET ai_suggested_price_cents = 0, ai_suggested_at = '', updated_at = ? WHERE id = ?`,
		time.Now(), purchaseID,
	)
	if err != nil {
		return fmt.Errorf("clear ai suggestion: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

func (ps *PurchaseStore) AcceptAISuggestion(ctx context.Context, purchaseID string, priceCents int) error {
	if priceCents <= 0 {
		return inventory.ErrNoAISuggestion
	}

	tx, err := ps.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
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
		return fmt.Errorf("accept ai suggestion: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrNoAISuggestion
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit accept ai suggestion transaction: %w", err)
	}
	return nil
}

func (ps *PurchaseStore) SetEbayExportFlag(ctx context.Context, purchaseID string, flaggedAt time.Time) error {
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET ebay_export_flagged_at = ? WHERE id = ?`,
		flaggedAt, purchaseID)
	if err != nil {
		return fmt.Errorf("set ebay export flag: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

func (ps *PurchaseStore) ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}

	tx, err := ps.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
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
			return fmt.Errorf("clear ebay export flags: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit clear ebay export flags transaction: %w", err)
	}
	return nil
}

func (ps *PurchaseStore) ListEbayFlaggedPurchases(ctx context.Context) ([]inventory.Purchase, error) {
	query := `SELECT ` + purchaseColumnsAliased + `
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed' AND p.ebay_export_flagged_at IS NOT NULL AND p.grader = 'PSA'
		ORDER BY c.created_at DESC, p.purchase_date DESC`
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query ebay flagged purchases: %w", err)
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

func (ps *PurchaseStore) UpdatePurchaseAISuggestion(ctx context.Context, purchaseID string, priceCents int) error {
	now := time.Now()
	suggestedAt := ""
	if priceCents > 0 {
		suggestedAt = now.Format(time.RFC3339)
	} else {
		priceCents = 0
	}
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET ai_suggested_price_cents = ?, ai_suggested_at = ?, updated_at = ? WHERE id = ?`,
		priceCents, suggestedAt, now, purchaseID,
	)
	if err != nil {
		return fmt.Errorf("update ai suggestion: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

func (ps *PurchaseStore) GetPriceOverrideStats(ctx context.Context) (*inventory.PriceOverrideStats, error) {
	var stats inventory.PriceOverrideStats
	err := ps.db.QueryRowContext(ctx, `
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
		return nil, fmt.Errorf("query price override stats: %w", err)
	}
	stats.OverrideTotalUsd = float64(stats.OverrideTotalCents) / 100
	stats.SuggestionTotalUsd = float64(stats.SuggestionTotalCents) / 100
	return &stats, nil
}

// UpdateExternalPurchaseFields updates all fields that come from an external import
// (card metadata, grader, grade, cost, value, images).
func (ps *PurchaseStore) UpdateExternalPurchaseFields(ctx context.Context, id string, p *inventory.Purchase) error {
	result, err := ps.db.ExecContext(ctx,
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
		return fmt.Errorf("update external purchase fields for %s: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected on update external purchase fields: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

// UpdatePurchaseMarketSnapshot persists refreshed market snapshot fields on a purchase.
func (ps *PurchaseStore) UpdatePurchaseMarketSnapshot(ctx context.Context, id string, snap inventory.MarketSnapshotData) error {
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET last_sold_cents = ?, lowest_list_cents = ?, conservative_cents = ?,
			median_cents = ?, active_listings = ?, sales_last_30d = ?, trend_30d = ?, snapshot_date = ?,
			snapshot_json = ?, updated_at = ?
		WHERE id = ?`,
		snap.LastSoldCents, snap.LowestListCents, snap.ConservativeCents,
		snap.MedianCents, snap.ActiveListings, snap.SalesLast30d, snap.Trend30d, snap.SnapshotDate,
		snap.SnapshotJSON, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update market snapshot for purchase %s: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected on update market snapshot: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}
