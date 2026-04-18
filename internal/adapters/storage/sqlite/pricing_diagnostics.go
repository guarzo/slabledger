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

// queryMappingCoverage buckets unsold purchases into the DH pipeline stages.
// Precedence (first match wins) reflects the pipeline direction:
//  1. listed            — dh_status in (listed, sold); live on DH
//  2. ready_to_list     — matched/manual + dh_status in_stock; waiting for user
//  3. unmatched         — unmatched/dismissed; needs human reconcile
//  4. matching          — pending/held/blank + received_at NOT NULL; scheduler will drain
//  5. awaiting_receipt  — anything else (includes pending without received_at)
//
// The five stages sum to TotalUnsold and unmatched aligns with the DH Unmatched
// Cards reconcile screen.
func (r *PricingDiagnosticsRepository) queryMappingCoverage(ctx context.Context, diag *pricing.PricingDiagnostics) error {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN bucket = 'listed' THEN 1 ELSE 0 END), 0) AS listed,
			COALESCE(SUM(CASE WHEN bucket = 'ready_to_list' THEN 1 ELSE 0 END), 0) AS ready_to_list,
			COALESCE(SUM(CASE WHEN bucket = 'unmatched' THEN 1 ELSE 0 END), 0) AS unmatched,
			COALESCE(SUM(CASE WHEN bucket = 'matching' THEN 1 ELSE 0 END), 0) AS matching,
			COALESCE(SUM(CASE WHEN bucket = 'awaiting_receipt' THEN 1 ELSE 0 END), 0) AS awaiting_receipt
		FROM (
			SELECT
				CASE
					WHEN cp.dh_status IN ('listed', 'sold') THEN 'listed'
					WHEN cp.dh_push_status IN ('matched', 'manual') THEN 'ready_to_list'
					WHEN cp.dh_push_status IN ('unmatched', 'dismissed') THEN 'unmatched'
					WHEN cp.received_at IS NOT NULL THEN 'matching'
					ELSE 'awaiting_receipt'
				END AS bucket
			FROM campaign_purchases cp
			JOIN campaigns c ON cp.campaign_id = c.id
			LEFT JOIN campaign_sales cs ON cp.id = cs.purchase_id
			WHERE cs.id IS NULL AND c.phase != 'closed'
		)
	`)
	if err := row.Scan(
		&diag.ListedCards,
		&diag.ReadyToListCards,
		&diag.UnmatchedCards,
		&diag.MatchingCards,
		&diag.AwaitingReceiptCards,
	); err != nil {
		return fmt.Errorf("query mapping coverage: %w", err)
	}
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
