package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// IntegrationFailureSample is a single failed-mapping row returned by the
// /failures admin endpoints. Shared by MM and CL.
type IntegrationFailureSample struct {
	PurchaseID string `json:"purchaseId"`
	CertNumber string `json:"certNumber"`
	CardName   string `json:"cardName"`
	Reason     string `json:"reason"`
	ErrorAt    string `json:"errorAt"`
}

// IntegrationFailuresReport groups per-purchase failure reasons for an integration.
type IntegrationFailuresReport struct {
	ByReason map[string]int             `json:"byReason"`
	Samples  []IntegrationFailureSample `json:"samples"`
}

// ReasonUnprocessed is a synthetic reason tag used for purchases that have no
// provider value and no recorded error. These rows slip through the scheduler's
// tagging paths (cancelled ctx, clobbered value, never-enumerated) and are
// otherwise invisible to the admin UI. Surfacing them as a distinct bucket is
// the observability fix for "many products with no price data and no reason."
const ReasonUnprocessed = "unprocessed"

// maxIntegrationFailureSamples caps the sample list returned by
// queryIntegrationFailures even if a caller passes a larger limit. The
// HTTP handlers already clamp via parsePagination, but this is
// defense-in-depth for any future direct caller.
const maxIntegrationFailureSamples = 200

// queryIntegrationFailures is the shared implementation behind
// MarketMoversStore.GetMMFailures and CardLadderStore.GetCLFailures.
//
// It returns:
//   - grouped counts by reason tag for rows whose reason column is non-empty
//   - a synthetic "unprocessed" count for rows whose value column is 0 and
//     whose reason column is empty (silent misses the scheduler never tagged)
//   - a bounded sample list combining both, sorted by most recent first;
//     unprocessed rows are sorted by the purchase's updated_at since they
//     have no error timestamp of their own.
//
// Column names are parameterized so one function handles both integrations;
// values are constrained to a small allowlist, so the string interpolation
// is safe from injection.
func queryIntegrationFailures(ctx context.Context, db *sql.DB, reasonCol, reasonAtCol, valueCol string, sampleLimit int) (*IntegrationFailuresReport, error) {
	// Validate reason + reason-timestamp + value column as a triple so a
	// caller can't mix mm_last_error with cl_last_error_at or cl_value_cents
	// with mm_last_error. The allowlist doubles as the only-ever safe values
	// for the string interpolation below.
	switch {
	case reasonCol == "mm_last_error" && reasonAtCol == "mm_last_error_at" && valueCol == "mm_value_cents":
	case reasonCol == "cl_last_error" && reasonAtCol == "cl_last_error_at" && valueCol == "cl_value_cents":
	default:
		return nil, fmt.Errorf("queryIntegrationFailures: invalid column triple (%q, %q, %q)", reasonCol, reasonAtCol, valueCol)
	}
	if sampleLimit <= 0 || sampleLimit > maxIntegrationFailureSamples {
		sampleLimit = maxIntegrationFailureSamples
	}

	report := &IntegrationFailuresReport{
		ByReason: make(map[string]int),
	}

	// Aggregated counts by reason (tagged failures only).
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
	for rows.Next() {
		var reason string
		var cnt int
		if err := rows.Scan(&reason, &cnt); err != nil {
			rows.Close() //nolint:errcheck
			return nil, fmt.Errorf("integration failure counts scan: %w", err)
		}
		report.ByReason[reason] = cnt
	}
	if err := rows.Err(); err != nil {
		rows.Close() //nolint:errcheck
		return nil, fmt.Errorf("integration failure counts rows: %w", err)
	}
	// Close before opening the second cursor — some drivers don't like two
	// open result sets at once, and the second query is independent.
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("integration failure counts close: %w", err)
	}

	// Synthetic "unprocessed" count — rows with no value AND no error tag.
	// These are silent misses (cancelled ctx mid-loop, clobbered value, never
	// reached by scheduler) that /failures would otherwise hide.
	unprocessedSQL := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
		  AND (p.%s = 0 OR p.%s IS NULL)
		  AND (p.%s = '' OR p.%s IS NULL)
	`, valueCol, valueCol, reasonCol, reasonCol)

	var unprocessedCount int
	if err := db.QueryRowContext(ctx, unprocessedSQL).Scan(&unprocessedCount); err != nil {
		return nil, fmt.Errorf("integration unprocessed count: %w", err)
	}
	if unprocessedCount > 0 {
		report.ByReason[ReasonUnprocessed] = unprocessedCount
	}

	// Bounded sample list, most recent first. We UNION the tagged failures
	// (sorted by error timestamp) with unprocessed rows (sorted by updated_at)
	// so the UI shows a coherent "here's what needs attention" view.
	samplesSQL := fmt.Sprintf(`
		SELECT id, cert_number, card_name, reason, error_at, sort_key FROM (
			SELECT p.id AS id,
			       COALESCE(p.cert_number, '') AS cert_number,
			       COALESCE(p.card_name, '') AS card_name,
			       p.%s AS reason,
			       p.%s AS error_at,
			       p.%s AS sort_key
			FROM campaign_purchases p
			INNER JOIN campaigns c ON c.id = p.campaign_id
			LEFT JOIN campaign_sales s ON s.purchase_id = p.id
			WHERE s.id IS NULL AND c.phase != 'closed' AND p.%s != ''

			UNION ALL

			SELECT p.id AS id,
			       COALESCE(p.cert_number, '') AS cert_number,
			       COALESCE(p.card_name, '') AS card_name,
			       '%s' AS reason,
			       '' AS error_at,
			       TO_CHAR(p.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS sort_key
			FROM campaign_purchases p
			INNER JOIN campaigns c ON c.id = p.campaign_id
			LEFT JOIN campaign_sales s ON s.purchase_id = p.id
			WHERE s.id IS NULL AND c.phase != 'closed'
			  AND (p.%s = 0 OR p.%s IS NULL)
			  AND (p.%s = '' OR p.%s IS NULL)
		) u
		ORDER BY sort_key DESC
		LIMIT $1
	`, reasonCol, reasonAtCol, reasonAtCol, reasonCol,
		ReasonUnprocessed,
		valueCol, valueCol, reasonCol, reasonCol)

	sampleRows, err := db.QueryContext(ctx, samplesSQL, sampleLimit)
	if err != nil {
		return nil, fmt.Errorf("integration failure samples: %w", err)
	}
	defer sampleRows.Close() //nolint:errcheck

	for sampleRows.Next() {
		var s IntegrationFailureSample
		var sortKey string
		if err := sampleRows.Scan(&s.PurchaseID, &s.CertNumber, &s.CardName, &s.Reason, &s.ErrorAt, &sortKey); err != nil {
			return nil, fmt.Errorf("integration failure sample scan: %w", err)
		}
		report.Samples = append(report.Samples, s)
	}
	if err := sampleRows.Err(); err != nil {
		return nil, fmt.Errorf("integration failure sample rows: %w", err)
	}

	return report, nil
}
