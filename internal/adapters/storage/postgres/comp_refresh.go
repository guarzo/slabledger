package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// UnsoldCardForComps represents an unsold purchase needing fresh sales comps.
type UnsoldCardForComps struct {
	PurchaseID string
	GemRateID  string
	Condition  string
}

// CompRefreshStore provides queries for the decoupled comp refresh pipeline.
type CompRefreshStore struct {
	db *sql.DB
}

// NewCompRefreshStore creates a new comp refresh store.
func NewCompRefreshStore(db *sql.DB) *CompRefreshStore {
	return &CompRefreshStore{db: db}
}

// ListUnsoldCardsNeedingComps returns one row per unique (gemRateID, condition)
// pair for unsold purchases where either no comps exist in cl_sales_comps or the
// most recent comp is older than cutoffDays.
func (s *CompRefreshStore) ListUnsoldCardsNeedingComps(ctx context.Context, cutoffDays int) ([]UnsoldCardForComps, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (cp.gem_rate_id, 'g' || REPLACE(cp.grade_value::text, '.', '_'))
			cp.id AS purchase_id,
			cp.gem_rate_id,
			'g' || REPLACE(cp.grade_value::text, '.', '_') AS condition
		FROM campaign_purchases cp
		LEFT JOIN campaign_sales cs ON cs.purchase_id = cp.id
		LEFT JOIN LATERAL (
			SELECT MAX(sale_date) AS latest
			FROM cl_sales_comps sc
			WHERE sc.gem_rate_id = cp.gem_rate_id
			  AND sc.condition = 'g' || REPLACE(cp.grade_value::text, '.', '_')
		) lc ON true
		WHERE cp.gem_rate_id != ''
		  AND cp.grade_value > 0
		  AND cs.id IS NULL
		  AND (lc.latest IS NULL OR lc.latest < to_char(NOW() - make_interval(days => $1), 'YYYY-MM-DD'))
		ORDER BY cp.gem_rate_id, 'g' || REPLACE(cp.grade_value::text, '.', '_'), cp.id DESC
	`, cutoffDays)
	if err != nil {
		return nil, fmt.Errorf("list unsold cards needing comps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []UnsoldCardForComps
	for rows.Next() {
		var c UnsoldCardForComps
		if err := rows.Scan(&c.PurchaseID, &c.GemRateID, &c.Condition); err != nil {
			return nil, fmt.Errorf("scan unsold card: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// BackfillLastSoldFromComps bulk-updates last_sold_date and last_sold_cents on
// unsold purchases from the most recent comp in cl_sales_comps matching each
// card's (gem_rate_id, condition). Only updates rows where the existing
// last_sold_date is empty or older than the comp.
func (s *CompRefreshStore) BackfillLastSoldFromComps(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE campaign_purchases cp
		SET last_sold_date = sub.sale_date,
			last_sold_cents = sub.price_cents
		FROM (
			SELECT DISTINCT ON (sc.gem_rate_id, sc.condition)
				sc.gem_rate_id, sc.condition, sc.sale_date, sc.price_cents
			FROM cl_sales_comps sc
			ORDER BY sc.gem_rate_id, sc.condition, sc.sale_date DESC
		) sub
		WHERE cp.gem_rate_id = sub.gem_rate_id
		  AND 'g' || REPLACE(cp.grade_value::text, '.', '_') = sub.condition
		  AND NOT EXISTS (SELECT 1 FROM campaign_sales cs WHERE cs.purchase_id = cp.id)
		  AND (cp.last_sold_date = '' OR cp.last_sold_date < sub.sale_date)
	`)
	if err != nil {
		return 0, fmt.Errorf("backfill last sold from comps: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("backfill last sold rows affected: %w", err)
	}
	return int(n), nil
}
