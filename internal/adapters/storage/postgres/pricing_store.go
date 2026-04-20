package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// PricingStore implements inventory.PricingRepository operations.
type PricingStore struct {
	base
}

// NewPricingStore creates a new Pricing store.
func NewPricingStore(db *sql.DB, logger observability.Logger) *PricingStore {
	return &PricingStore{base{db: db, logger: logger}}
}

var _ inventory.PricingRepository = (*PricingStore)(nil)

func (prs *PricingStore) UpdateReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	now := time.Now()
	if priceCents <= 0 {
		priceCents = 0
		source = ""
	}
	reviewedAt := ""
	if priceCents > 0 {
		reviewedAt = now.Format(time.RFC3339)
	}
	// When a reviewed price is committed, any outstanding AI suggestion is
	// superseded — clearing it keeps the "Needs Attention" filter honest.
	// $1 is reused in three contexts (SET int, two CASE WHEN comparisons).
	// pgx QueryExecModeExec uses extended protocol with type inference;
	// reusing $1 in heterogenous contexts trips SQLSTATE 42P08
	// ("inconsistent types deduced for parameter"). Explicit ::BIGINT casts
	// pin the type consistently.
	result, err := prs.db.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET reviewed_price_cents = $1::BIGINT, reviewed_at = $2, review_source = $3,
		     ai_suggested_price_cents = CASE WHEN $1::BIGINT > 0 THEN 0 ELSE ai_suggested_price_cents END,
		     ai_suggested_at         = CASE WHEN $1::BIGINT > 0 THEN '' ELSE ai_suggested_at END,
		     updated_at = $4
		 WHERE id = $5`,
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
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

func (prs *PricingStore) GetReviewStats(ctx context.Context, campaignID string) (inventory.ReviewStats, error) {
	var stats inventory.ReviewStats
	err := prs.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN cp.reviewed_at = '' THEN 1 ELSE 0 END), 0) as needs_review,
			COALESCE(SUM(CASE WHEN cp.reviewed_at != '' THEN 1 ELSE 0 END), 0) as reviewed,
			(SELECT COUNT(DISTINCT pf.purchase_id) FROM price_flags pf
			 JOIN campaign_purchases cp2 ON pf.purchase_id = cp2.id
			 LEFT JOIN campaign_sales cs2 ON cs2.purchase_id = cp2.id
			 WHERE cp2.campaign_id = $1 AND pf.resolved_at IS NULL AND cs2.id IS NULL) as flagged
		FROM campaign_purchases cp
		LEFT JOIN campaign_sales cs ON cs.purchase_id = cp.id
		WHERE cp.campaign_id = $2 AND cs.id IS NULL`,
		campaignID, campaignID,
	).Scan(&stats.Total, &stats.NeedsReview, &stats.Reviewed, &stats.Flagged)
	if err != nil {
		return stats, fmt.Errorf("get review stats: %w", err)
	}
	return stats, nil
}

func (prs *PricingStore) GetGlobalReviewStats(ctx context.Context) (inventory.ReviewStats, error) {
	var stats inventory.ReviewStats
	err := prs.db.QueryRowContext(ctx, `
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

func (prs *PricingStore) CreatePriceFlag(ctx context.Context, flag *inventory.PriceFlag) (int64, error) {
	var id int64
	err := prs.db.QueryRowContext(ctx,
		`INSERT INTO price_flags (purchase_id, flagged_by, flagged_at, reason) VALUES ($1, $2, $3, $4) RETURNING id`,
		flag.PurchaseID, flag.FlaggedBy, flag.FlaggedAt, string(flag.Reason),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create price flag: %w", err)
	}
	return id, nil
}

func (prs *PricingStore) ListPriceFlags(ctx context.Context, status string) ([]inventory.PriceFlagWithContext, error) {
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

	rows, err := prs.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list price flags: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var flags []inventory.PriceFlagWithContext
	for rows.Next() {
		var f inventory.PriceFlagWithContext
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
			var snap inventory.MarketSnapshot
			if err := json.Unmarshal([]byte(snapshotJSON), &snap); err == nil {
				f.MarketPriceCents = snap.MedianCents
				f.SourcePrices = snap.SourcePrices
			}
		}

		flags = append(flags, f)
	}
	if flags == nil {
		flags = []inventory.PriceFlagWithContext{}
	}
	return flags, rows.Err()
}

func (prs *PricingStore) ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error {
	result, err := prs.db.ExecContext(ctx,
		`UPDATE price_flags SET resolved_at = $1, resolved_by = $2 WHERE id = $3 AND resolved_at IS NULL`,
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
		return inventory.ErrPriceFlagNotFound
	}
	return nil
}

func (prs *PricingStore) HasOpenFlag(ctx context.Context, purchaseID string) (bool, error) {
	var count int
	err := prs.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM price_flags WHERE purchase_id = $1 AND resolved_at IS NULL`, purchaseID,
	).Scan(&count)
	return count > 0, err
}

func (prs *PricingStore) OpenFlagPurchaseIDs(ctx context.Context) (map[string]int64, error) {
	rows, err := prs.db.QueryContext(ctx,
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
			return nil, fmt.Errorf("scanning open flag purchase ID: %w", err)
		}
		result[purchaseID] = flagID
	}
	return result, rows.Err()
}
