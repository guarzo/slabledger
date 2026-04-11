package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// maxIntegrationFailureSamples caps the sample list returned by
// queryIntegrationFailures even if a caller passes a larger limit. The
// HTTP handlers already clamp via parsePagination, but this is
// defense-in-depth for any future direct caller.
const maxIntegrationFailureSamples = 200

// queryIntegrationFailures is the shared implementation behind
// MarketMoversStore.GetMMFailures and CardLadderStore.GetCLFailures.
//
// It joins unsold, non-archived purchases with a non-empty failure column
// (either mm_last_error or cl_last_error) and returns:
//   - a grouped count by reason tag
//   - a bounded sample list (sorted by most recent error first)
//
// Column names are parameterized so one function handles both integrations;
// values are constrained to a small allowlist in the per-integration wrappers,
// so the string interpolation is safe from injection.
func queryIntegrationFailures(ctx context.Context, db *sql.DB, reasonCol, reasonAtCol string, sampleLimit int) (*IntegrationFailuresReport, error) {
	if reasonCol != "mm_last_error" && reasonCol != "cl_last_error" {
		return nil, fmt.Errorf("queryIntegrationFailures: unknown reason column %q", reasonCol)
	}
	if reasonAtCol != "mm_last_error_at" && reasonAtCol != "cl_last_error_at" {
		return nil, fmt.Errorf("queryIntegrationFailures: unknown reason timestamp column %q", reasonAtCol)
	}
	if sampleLimit <= 0 || sampleLimit > maxIntegrationFailureSamples {
		sampleLimit = maxIntegrationFailureSamples
	}

	report := &IntegrationFailuresReport{
		ByReason: make(map[string]int),
	}

	// Aggregated counts by reason.
	countsSQL := fmt.Sprintf(`
		SELECT p.%s AS reason, COUNT(*) AS cnt
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed' AND p.%s != ''
		GROUP BY p.%s
	`, reasonCol, reasonCol, reasonCol)

	rows, err := db.QueryContext(ctx, countsSQL)
	if err != nil {
		return nil, fmt.Errorf("integration failure counts: %w", err)
	}
	func() {
		defer rows.Close() //nolint:errcheck
		for rows.Next() {
			var reason string
			var cnt int
			if err = rows.Scan(&reason, &cnt); err != nil {
				return
			}
			report.ByReason[reason] = cnt
		}
		err = rows.Err()
	}()
	if err != nil {
		return nil, fmt.Errorf("integration failure counts scan: %w", err)
	}

	// Bounded sample list, most recent errors first.
	samplesSQL := fmt.Sprintf(`
		SELECT p.id, COALESCE(p.cert_number, ''), COALESCE(p.card_name, ''), p.%s, p.%s
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed' AND p.%s != ''
		ORDER BY p.%s DESC
		LIMIT ?
	`, reasonCol, reasonAtCol, reasonCol, reasonAtCol)

	sampleRows, err := db.QueryContext(ctx, samplesSQL, sampleLimit)
	if err != nil {
		return nil, fmt.Errorf("integration failure samples: %w", err)
	}
	defer sampleRows.Close() //nolint:errcheck

	for sampleRows.Next() {
		var s IntegrationFailureSample
		if err := sampleRows.Scan(&s.PurchaseID, &s.CertNumber, &s.CardName, &s.Reason, &s.ErrorAt); err != nil {
			return nil, fmt.Errorf("integration failure sample scan: %w", err)
		}
		report.Samples = append(report.Samples, s)
	}
	if err := sampleRows.Err(); err != nil {
		return nil, fmt.Errorf("integration failure sample rows: %w", err)
	}

	return report, nil
}
