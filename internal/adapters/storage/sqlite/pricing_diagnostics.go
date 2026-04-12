package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// PricingDiagnosticsRepository implements pricing.PricingDiagnosticsProvider using SQLite.
type PricingDiagnosticsRepository struct {
	db *sql.DB
}

var _ pricing.PricingDiagnosticsProvider = (*PricingDiagnosticsRepository)(nil)

func NewPricingDiagnosticsRepository(db *sql.DB) *PricingDiagnosticsRepository {
	return &PricingDiagnosticsRepository{db: db}
}

func (r *PricingDiagnosticsRepository) GetPricingDiagnostics(ctx context.Context) (*pricing.PricingDiagnostics, error) {
	diag := &pricing.PricingDiagnostics{}

	if err := r.queryMappingCoverage(ctx, diag); err != nil {
		return nil, err
	}

	if err := r.queryPriceCoverage(ctx, diag); err != nil {
		return nil, err
	}

	if err := r.queryRecentFailures(ctx, diag); err != nil {
		return nil, err
	}

	return diag, nil
}

// queryMappingCoverage counts how many inventory cards have DH mappings vs not.
func (r *PricingDiagnosticsRepository) queryMappingCoverage(ctx context.Context, diag *pricing.PricingDiagnostics) error {
	row := r.db.QueryRowContext(ctx, `
		WITH inventory AS (
			SELECT DISTINCT cp.card_name, cp.set_name, COALESCE(cp.card_number, '') AS card_number
			FROM campaign_purchases cp
			JOIN campaigns c ON cp.campaign_id = c.id
			LEFT JOIN campaign_sales cs ON cp.id = cs.purchase_id
			WHERE cs.id IS NULL AND c.phase != 'closed'
		)
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN m.external_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS mapped
		FROM inventory inv
		LEFT JOIN card_id_mappings m
			ON m.card_name = inv.card_name
			AND m.set_name = inv.set_name
			AND m.collector_number = inv.card_number
			AND m.provider = 'doubleholo'
	`)
	var total, mapped int
	if err := row.Scan(&total, &mapped); err != nil {
		return fmt.Errorf("query mapping coverage: %w", err)
	}
	diag.TotalMappedCards = mapped
	diag.UnmappedCards = total - mapped
	return nil
}

// queryPriceCoverage counts unsold inventory cards with CL and MM prices.
func (r *PricingDiagnosticsRepository) queryPriceCoverage(ctx context.Context, diag *pricing.PricingDiagnostics) error {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total_unsold,
			COALESCE(SUM(CASE WHEN cp.cl_value_cents > 0 THEN 1 ELSE 0 END), 0) AS cl_priced,
			COALESCE(SUM(CASE WHEN cp.mm_value_cents > 0 THEN 1 ELSE 0 END), 0) AS mm_priced
		FROM campaign_purchases cp
		JOIN campaigns c ON cp.campaign_id = c.id
		LEFT JOIN campaign_sales cs ON cp.id = cs.purchase_id
		WHERE cs.id IS NULL AND c.phase != 'closed'
	`)
	if err := row.Scan(&diag.TotalUnsold, &diag.CLPricedCards, &diag.MMPricedCards); err != nil {
		return fmt.Errorf("query price coverage: %w", err)
	}
	return nil
}

func (r *PricingDiagnosticsRepository) queryRecentFailures(ctx context.Context, diag *pricing.PricingDiagnostics) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			provider,
			CASE
				WHEN status_code = 429 THEN 'rate_limited'
				WHEN status_code >= 500 THEN 'server_error'
				WHEN error LIKE '%not found%' OR error LIKE '%no match%' THEN 'not_found'
				WHEN error LIKE '%timeout%' THEN 'timeout'
				ELSE 'other'
			END AS error_type,
			COUNT(*) AS cnt,
			MAX(timestamp) AS last_seen
		FROM api_calls
		WHERE (status_code >= 400 OR error != '')
			AND timestamp > datetime('now', '-24 hours')
		GROUP BY provider, error_type
		ORDER BY cnt DESC
		LIMIT 50
	`)
	if err != nil {
		return fmt.Errorf("query recent failures: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var f pricing.FailureSummary
		var lastSeen string
		if err := rows.Scan(&f.Provider, &f.ErrorType, &f.Count, &lastSeen); err != nil {
			return fmt.Errorf("scan failure row: %w", err)
		}
		f.LastSeen = parseSQLiteTime(lastSeen)
		diag.RecentFailures = append(diag.RecentFailures, f)
	}
	return rows.Err()
}
