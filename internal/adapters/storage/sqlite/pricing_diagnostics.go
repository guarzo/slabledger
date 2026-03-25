package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// PricingDiagnosticsRepository implements pricing.PricingDiagnosticsProvider using SQLite.
type PricingDiagnosticsRepository struct {
	db *sql.DB
}

// Compile-time interface check
var _ pricing.PricingDiagnosticsProvider = (*PricingDiagnosticsRepository)(nil)

// NewPricingDiagnosticsRepository creates a new diagnostics repository.
func NewPricingDiagnosticsRepository(db *sql.DB) *PricingDiagnosticsRepository {
	return &PricingDiagnosticsRepository{db: db}
}

// GetPricingDiagnostics queries existing price_history and api_calls tables to
// surface pricing data quality metrics.
func (r *PricingDiagnosticsRepository) GetPricingDiagnostics(ctx context.Context) (*pricing.PricingDiagnostics, error) {
	diag := &pricing.PricingDiagnostics{
		SourceCoverage: make(map[string]int),
	}

	// 1. Source coverage: count distinct cards per source (last 7 days)
	if err := r.querySourceCoverage(ctx, diag); err != nil {
		return nil, err
	}

	// 2. Card quality breakdown by fusion_source_count
	if err := r.queryCardQuality(ctx, diag); err != nil {
		return nil, err
	}

	// 3. PC-only card list (limited to 100)
	if err := r.queryPCOnlyCards(ctx, diag); err != nil {
		return nil, err
	}

	// 4. Recent API failure patterns (last 24h)
	if err := r.queryRecentFailures(ctx, diag); err != nil {
		return nil, err
	}

	// 5. Discovery failure count
	if err := r.queryDiscoveryFailureCount(ctx, diag); err != nil {
		return nil, err
	}

	return diag, nil
}

func (r *PricingDiagnosticsRepository) querySourceCoverage(ctx context.Context, diag *pricing.PricingDiagnostics) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT source, COUNT(DISTINCT card_name || '|' || set_name || '|' || COALESCE(card_number, ''))
		FROM price_history
		WHERE updated_at > datetime('now', '-7 days')
		GROUP BY source
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return err
		}
		diag.SourceCoverage[source] = count
	}
	return rows.Err()
}

func (r *PricingDiagnosticsRepository) queryCardQuality(ctx context.Context, diag *pricing.PricingDiagnostics) error {
	row := r.db.QueryRowContext(ctx, `
		WITH card_best AS (
			SELECT
				card_name, set_name, card_number,
				MAX(CASE WHEN source = 'fusion' THEN fusion_source_count ELSE 0 END) AS best_count,
				GROUP_CONCAT(DISTINCT source) AS sources
			FROM price_history
			WHERE updated_at > datetime('now', '-7 days')
			GROUP BY card_name, set_name, card_number
		)
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN best_count >= 2 THEN 1 ELSE 0 END), 0) AS full_fusion,
			COALESCE(SUM(CASE WHEN best_count = 1 THEN 1 ELSE 0 END), 0) AS partial,
			COALESCE(SUM(CASE WHEN best_count < 1 THEN 1 ELSE 0 END), 0) AS pc_only
		FROM card_best
	`)
	return row.Scan(&diag.TotalCards, &diag.FullFusionCards, &diag.PartialCards, &diag.PCOnlyCards)
}

func (r *PricingDiagnosticsRepository) queryPCOnlyCards(ctx context.Context, diag *pricing.PricingDiagnostics) error {
	rows, err := r.db.QueryContext(ctx, `
		WITH card_best AS (
			SELECT
				card_name, set_name, card_number,
				MAX(CASE WHEN source = 'fusion' THEN fusion_source_count ELSE 0 END) AS best_count,
				GROUP_CONCAT(DISTINCT source) AS sources,
				MAX(price_cents) AS max_price,
				MAX(updated_at) AS last_updated
			FROM price_history
			WHERE updated_at > datetime('now', '-7 days')
			GROUP BY card_name, set_name, card_number
		)
		SELECT card_name, set_name, card_number, sources, max_price, last_updated
		FROM card_best
		WHERE best_count < 1
		ORDER BY max_price DESC
		LIMIT 100
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	for rows.Next() {
		var c pricing.DiagnosticCard
		var sources, updatedAt string
		var priceCents int64
		if err := rows.Scan(&c.CardName, &c.SetName, &c.CardNumber, &sources, &priceCents, &updatedAt); err != nil {
			return err
		}
		c.PriceUsd = float64(priceCents) / 100.0
		c.Sources = strings.Split(sources, ",")
		c.UpdatedAt = parseSQLiteTime(updatedAt)
		diag.PCOnlyCardList = append(diag.PCOnlyCardList, c)
	}
	return rows.Err()
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
		return err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	for rows.Next() {
		var f pricing.FailureSummary
		var lastSeen string
		if err := rows.Scan(&f.Provider, &f.ErrorType, &f.Count, &lastSeen); err != nil {
			return err
		}
		f.LastSeen = parseSQLiteTime(lastSeen)
		diag.RecentFailures = append(diag.RecentFailures, f)
	}
	return rows.Err()
}

func (r *PricingDiagnosticsRepository) queryDiscoveryFailureCount(ctx context.Context, diag *pricing.PricingDiagnostics) error {
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM discovery_failures
	`).Scan(&diag.DiscoveryFailures)
	if err != nil {
		// Table may not exist during migration transitions — treat as 0.
		// Any other DB error should surface to the caller.
		if strings.Contains(err.Error(), "no such table") {
			diag.DiscoveryFailures = 0
			return nil
		}
		return err
	}
	return nil
}
