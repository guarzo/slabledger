package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

func (r *CampaignsRepository) GetReviewStats(ctx context.Context, campaignID string) (campaigns.ReviewStats, error) {
	var stats campaigns.ReviewStats
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN cp.reviewed_at = '' THEN 1 ELSE 0 END), 0) as needs_review,
			COALESCE(SUM(CASE WHEN cp.reviewed_at != '' THEN 1 ELSE 0 END), 0) as reviewed,
			(SELECT COUNT(DISTINCT pf.purchase_id) FROM price_flags pf
			 JOIN campaign_purchases cp2 ON pf.purchase_id = cp2.id
			 LEFT JOIN campaign_sales cs2 ON cs2.purchase_id = cp2.id
			 WHERE cp2.campaign_id = ? AND pf.resolved_at IS NULL AND cs2.id IS NULL) as flagged
		FROM campaign_purchases cp
		LEFT JOIN campaign_sales cs ON cs.purchase_id = cp.id
		WHERE cp.campaign_id = ? AND cs.id IS NULL`,
		campaignID, campaignID,
	).Scan(&stats.Total, &stats.NeedsReview, &stats.Reviewed, &stats.Flagged)
	if err != nil {
		return stats, fmt.Errorf("get review stats: %w", err)
	}
	return stats, nil
}

func (r *CampaignsRepository) GetGlobalReviewStats(ctx context.Context) (campaigns.ReviewStats, error) {
	var stats campaigns.ReviewStats
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN cp.reviewed_at = '' THEN 1 ELSE 0 END), 0) as needs_review,
			COALESCE(SUM(CASE WHEN cp.reviewed_at != '' THEN 1 ELSE 0 END), 0) as reviewed,
			(SELECT COUNT(DISTINCT pf.purchase_id) FROM price_flags pf
			 JOIN campaign_purchases cp2 ON pf.purchase_id = cp2.id
			 LEFT JOIN campaign_sales cs2 ON cs2.purchase_id = cp2.id
			 WHERE pf.resolved_at IS NULL AND cs2.id IS NULL) as flagged
		FROM campaign_purchases cp
		LEFT JOIN campaign_sales cs ON cs.purchase_id = cp.id
		WHERE cs.id IS NULL`,
	).Scan(&stats.Total, &stats.NeedsReview, &stats.Reviewed, &stats.Flagged)
	if err != nil {
		return stats, fmt.Errorf("get global review stats: %w", err)
	}
	return stats, nil
}

func (r *CampaignsRepository) CreatePriceFlag(ctx context.Context, flag *campaigns.PriceFlag) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO price_flags (purchase_id, flagged_by, flagged_at, reason) VALUES (?, ?, ?, ?)`,
		flag.PurchaseID, flag.FlaggedBy, flag.FlaggedAt, string(flag.Reason),
	)
	if err != nil {
		return 0, fmt.Errorf("create price flag: %w", err)
	}
	return result.LastInsertId()
}

func (r *CampaignsRepository) ListPriceFlags(ctx context.Context, status string) ([]campaigns.PriceFlagWithContext, error) {
	var where string
	switch status {
	case "open":
		where = "AND pf.resolved_at IS NULL"
	case "resolved":
		where = "AND pf.resolved_at IS NOT NULL"
	default:
		where = ""
	}

	query := fmt.Sprintf(`
		SELECT pf.id, pf.purchase_id, pf.flagged_by, pf.flagged_at, pf.reason,
			pf.resolved_at, pf.resolved_by,
			cp.card_name, cp.set_name, cp.card_number, cp.grade_value, cp.cert_number,
			u.email,
			cp.cl_value_cents, cp.reviewed_price_cents, cp.snapshot_json
		FROM price_flags pf
		JOIN campaign_purchases cp ON pf.purchase_id = cp.id
		JOIN users u ON pf.flagged_by = u.id
		WHERE 1=1 %s
		ORDER BY pf.flagged_at DESC`, where)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list price flags: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var flags []campaigns.PriceFlagWithContext
	for rows.Next() {
		var f campaigns.PriceFlagWithContext
		var resolvedAt sql.NullTime
		var resolvedBy sql.NullInt64
		var snapshotJSON string

		err := rows.Scan(
			&f.ID, &f.PurchaseID, &f.FlaggedBy, &f.FlaggedAt, &f.Reason,
			&resolvedAt, &resolvedBy,
			&f.CardName, &f.SetName, &f.CardNumber, &f.Grade, &f.CertNumber,
			&f.FlaggedByEmail,
			&f.CLValueCents, &f.ReviewedPriceCents, &snapshotJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan price flag: %w", err)
		}
		if resolvedAt.Valid {
			f.ResolvedAt = &resolvedAt.Time
		}
		if resolvedBy.Valid {
			v := resolvedBy.Int64
			f.ResolvedBy = &v
		}

		if snapshotJSON != "" {
			var snap campaigns.MarketSnapshot
			if err := json.Unmarshal([]byte(snapshotJSON), &snap); err == nil {
				f.MarketPriceCents = snap.MedianCents
				f.SourcePrices = snap.SourcePrices
			}
		}

		flags = append(flags, f)
	}
	if flags == nil {
		flags = []campaigns.PriceFlagWithContext{}
	}
	return flags, rows.Err()
}

func (r *CampaignsRepository) ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE price_flags SET resolved_at = ?, resolved_by = ? WHERE id = ? AND resolved_at IS NULL`,
		time.Now(), resolvedBy, flagID,
	)
	if err != nil {
		return fmt.Errorf("resolve price flag: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return campaigns.ErrPriceFlagNotFound
	}
	return nil
}

func (r *CampaignsRepository) HasOpenFlag(ctx context.Context, purchaseID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM price_flags WHERE purchase_id = ? AND resolved_at IS NULL`, purchaseID,
	).Scan(&count)
	return count > 0, err
}

func (r *CampaignsRepository) OpenFlagPurchaseIDs(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT purchase_id, MIN(id) FROM price_flags WHERE resolved_at IS NULL GROUP BY purchase_id`)
	if err != nil {
		return nil, fmt.Errorf("open flag purchase IDs: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	result := make(map[string]int64)
	for rows.Next() {
		var purchaseID string
		var flagID int64
		if err := rows.Scan(&purchaseID, &flagID); err != nil {
			return nil, err
		}
		result[purchaseID] = flagID
	}
	return result, rows.Err()
}
