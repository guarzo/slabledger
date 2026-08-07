package postgres

import (
	"context"
	"fmt"
	"math"
	"time"
)

// CLCoverageEraStart is the instant CardLadder first wrote a value to
// campaign_purchases in production, verified 2026-08-07.
//
// It is PINNED, deliberately. The obvious alternative -- deriving it as
// MIN(cl_value_updated_at) -- is wrong: UpdatePurchaseCLValue overwrites that
// timestamp on every successful refresh (purchase_store.go:269), so the minimum
// creeps forward as rows are re-swept or deleted. A derived boundary would
// silently migrate rows from pre_cl into pending and rewrite historical months.
//
// If CardLadder history is ever re-imported from earlier than this date, update
// this constant by hand -- AND the hardcoded `TIMESTAMP '...'` literal at
// scripts/cl-coverage.sql:48, which does not read this constant and will not
// change with it. TestCLCoverageEraStart_IsPinned asserts the value so the
// change cannot happen by accident.
const CLCoverageEraStart = "2026-04-13T04:00:13Z"

// Bucket names. These are the only values the coverage query's CASE can emit,
// and they must match scripts/cl-coverage.sql.
const (
	clBucketResolved   = "resolved"
	clBucketUnresolved = "unresolved"
	clBucketPending    = "pending"
	clBucketStranded   = "stranded"
	clBucketPreCL      = "pre_cl"
)

const clCohortCampaign = "campaign"

// clCoverageEraTimestamp renders CLCoverageEraStart in the form Postgres accepts
// for a `timestamp` (no zone) comparison against campaign_purchases.created_at.
//
// created_at is TIMESTAMP WITHOUT TIME ZONE and is written from Go time values
// normalized to UTC, so comparing it against the UTC wall-clock form of the era
// start is correct. Verified against production: the resulting pre_cl count is
// 339, exactly matching the independently-established count of rows the Shopify
// import valued before CardLadder existed.
func clCoverageEraTimestamp() (string, error) {
	t, err := time.Parse(time.RFC3339, CLCoverageEraStart)
	if err != nil {
		return "", fmt.Errorf("parse CL coverage era start %q: %w", CLCoverageEraStart, err)
	}
	return t.UTC().Format("2006-01-02 15:04:05"), nil
}

// clCoverageRow is one aggregated group returned by the coverage query: a
// (month, cohort, bucket, reason) tuple and how many purchases fell into it.
// Reason is populated only for the unresolved bucket.
type clCoverageRow struct {
	Month      string
	Cohort     string
	Bucket     string
	Reason     string
	N          int
	Reassigned int
}

// CLCoverageCohort is the per-cohort breakdown for one month.
//
// Rows is the full total and deliberately does NOT equal the Pct denominator:
// Rows = Resolved + Unresolved + Pending + Stranded + PreCL. It is reported so
// the excluded counts can be reconciled by a reader.
type CLCoverageCohort struct {
	Rows       int      `json:"rows"`
	Resolved   int      `json:"resolved"`
	Unresolved int      `json:"unresolved"`
	Pending    int      `json:"pending"`
	Stranded   int      `json:"stranded"`
	PreCL      int      `json:"preCL"`
	Pct        *float64 `json:"pct"`
}

// CLCoverageMonth is one purchase month, split by intake cohort.
//
// Reassigned counts rows whose purchase_source is set but whose campaign_id is
// 'external' -- i.e. the two possible cohort definitions disagree. It is
// reported so that drift between them is visible rather than silent.
type CLCoverageMonth struct {
	Month              string           `json:"month"`
	Reassigned         int              `json:"reassigned"`
	Campaign           CLCoverageCohort `json:"campaign"`
	External           CLCoverageCohort `json:"external"`
	UnresolvedByReason map[string]int   `json:"unresolvedByReason"`
}

// CLCoverageReport is the full response. EraStart echoes the pinned constant so
// a reader can see what PreCL was measured against.
type CLCoverageReport struct {
	EraStart string            `json:"eraStart"`
	Months   []CLCoverageMonth `json:"months"`
}

// clCoveragePct is Resolved / (Resolved + Unresolved), as a percentage rounded
// to one decimal place.
//
// Pending, Stranded and PreCL are excluded from the denominator on purpose:
// CardLadder never returned an answer for those rows, so they are evidence
// neither for nor against coverage. Including them is exactly the defect that
// made 25 freshly-imported August purchases read as 28% coverage.
//
// Returns nil -- JSON null -- rather than 0 on an empty denominator, so that
// "no rows to judge" is distinguishable from "judged, and nothing resolved".
func clCoveragePct(c CLCoverageCohort) *float64 {
	den := c.Resolved + c.Unresolved
	if den == 0 {
		return nil
	}
	v := math.Round(float64(c.Resolved)*1000/float64(den)) / 10
	return &v
}

// foldCLCoverage folds the flat per-group counts into the nested response.
//
// Month ordering is inherited from the query's ORDER BY month DESC rather than
// re-sorted here; first appearance wins.
func foldCLCoverage(rows []clCoverageRow) *CLCoverageReport {
	byMonth := make(map[string]*CLCoverageMonth, len(rows))
	order := make([]string, 0, len(rows))

	for _, r := range rows {
		m, ok := byMonth[r.Month]
		if !ok {
			m = &CLCoverageMonth{
				Month:              r.Month,
				UnresolvedByReason: map[string]int{},
			}
			byMonth[r.Month] = m
			order = append(order, r.Month)
		}
		m.Reassigned += r.Reassigned

		// No rejection guard here, unlike clKnownBuckets below: the cohort CASE
		// at line 219 is binary ('campaign' or 'external' -- nothing else it can
		// emit), so an unrecognized value would mean the SQL itself changed, not
		// bad data reaching this fold.
		cohort := &m.External
		if r.Cohort == clCohortCampaign {
			cohort = &m.Campaign
		}
		cohort.Rows += r.N

		switch r.Bucket {
		case clBucketResolved:
			cohort.Resolved += r.N
		case clBucketUnresolved:
			cohort.Unresolved += r.N
			if r.Reason != "" {
				m.UnresolvedByReason[r.Reason] += r.N
			}
		case clBucketPending:
			cohort.Pending += r.N
		case clBucketStranded:
			cohort.Stranded += r.N
		case clBucketPreCL:
			cohort.PreCL += r.N
		}
	}

	report := &CLCoverageReport{
		EraStart: CLCoverageEraStart,
		Months:   make([]CLCoverageMonth, 0, len(order)),
	}
	for _, key := range order {
		m := byMonth[key]
		m.Campaign.Pct = clCoveragePct(m.Campaign)
		m.External.Pct = clCoveragePct(m.External)
		report.Months = append(report.Months, *m)
	}
	return report
}

// clKnownBuckets is the set of bucket values GetCLCoverageByMonth's query can
// emit. foldCLCoverage's switch has no default arm, so a bucket outside this
// set would silently vanish from every counter but Rows -- validated here,
// at the boundary where the value is first read back from the database,
// rather than left to go unnoticed downstream.
var clKnownBuckets = map[string]bool{
	clBucketResolved:   true,
	clBucketUnresolved: true,
	clBucketPending:    true,
	clBucketStranded:   true,
	clBucketPreCL:      true,
}

// GetCLCoverageByMonth reports CardLadder value coverage per purchase month and
// intake cohort.
//
// Coverage is measured on cl_value_updated_at, NOT cl_value_cents. The latter
// has a second writer -- the Shopify external import (purchase_price_store.go:231)
// -- which never calls CardLadder, so 339 production rows carry a positive
// cl_value_cents CardLadder never produced. cl_value_updated_at has exactly one
// writer (purchase_store.go:269) reachable only past a positive-value guard, and
// nothing ever clears it.
//
// Months are grouped by purchase_date, so a backdated bulk import lands in the
// month the purchase happened. created_at is used only for the era comparison,
// where import time is the correct notion.
//
// Kept in sync with scripts/cl-coverage.sql -- change the bucket CASE in one and
// you must change it in the other.
func (s *CardLadderStore) GetCLCoverageByMonth(ctx context.Context) (*CLCoverageReport, error) {
	eraStart, err := clCoverageEraTimestamp()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH classified AS (
		    SELECT
		        left(p.purchase_date, 7) AS month,
		        CASE WHEN p.purchase_source <> '' THEN 'campaign' ELSE 'external' END AS cohort,
		        CASE
		            -- CardLadder returned a positive value at least once.
		            WHEN p.cl_value_updated_at <> ''                    THEN 'resolved'
		            -- Asked and failed for a real reason (today: always 'no_value').
		            WHEN p.cl_last_error NOT IN ('', 'quota_exhausted') THEN 'unresolved'
		            -- Still reachable by the sweep: no linked sale, campaign open.
		            -- Matches ListAllUnsoldPurchases (purchase_store.go:177-183).
		            -- Tested BEFORE both the quota arm and the era cutoff, because
		            -- eligibility is what actually decides whether a row will ever be
		            -- answered. A quota-marked row that is still eligible is pending;
		            -- one that has since been sold or closed is NOT -- the sweep can
		            -- no longer see it, and nothing clears cl_last_error on sale
		            -- (sale_store.go writes no CL column; purchase_store.go:284 is the
		            -- sole writer). Ordering quota first would park such a row in
		            -- 'pending' forever.
		            WHEN c.phase <> 'closed'
		                 AND NOT EXISTS (
		                     SELECT 1 FROM campaign_sales s WHERE s.purchase_id = p.id
		                 )                                              THEN 'pending'
		            -- Created before CardLadder ever ran, and never touched by it.
		            -- The cl_last_error = '' guard matters: a quota-marked row proves
		            -- the sweep DID reach it, so it can never honestly be pre_cl no
		            -- matter how old it is.
		            WHEN p.cl_last_error = ''
		                 AND p.created_at < $1::timestamp               THEN 'pre_cl'
		            -- Never answered and no longer reachable by the sweep.
		            ELSE 'stranded'
		        END AS bucket,
		        p.cl_last_error AS reason,
		        CASE
		            WHEN p.purchase_source <> '' AND p.campaign_id = 'external'
		            THEN 1 ELSE 0
		        END AS reassigned
		    FROM campaign_purchases p
		    INNER JOIN campaigns c ON c.id = p.campaign_id
		    -- NOTE: both filters from GetCLPriceStats (s.id IS NULL, phase != 'closed')
		    -- are deliberately ABSENT. This query covers ALL purchases, including sold
		    -- ones -- 1,473 of 1,542 priced rows are already sold, which is precisely
		    -- why the freshness panel could not have caught the original incident. The
		    -- join and the NOT EXISTS above are used to LABEL rows, not to exclude
		    -- them. Do not "fix" this by adding a WHERE clause.
		)
		SELECT
		    month,
		    cohort,
		    bucket,
		    CASE WHEN bucket = 'unresolved' THEN reason ELSE '' END AS reason,
		    count(*) AS n,
		    sum(reassigned) AS reassigned
		FROM classified
		GROUP BY month, cohort, bucket, CASE WHEN bucket = 'unresolved' THEN reason ELSE '' END
		ORDER BY month DESC, cohort
	`, eraStart)
	if err != nil {
		return nil, fmt.Errorf("cl coverage by month: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []clCoverageRow
	for rows.Next() {
		var r clCoverageRow
		if err := rows.Scan(&r.Month, &r.Cohort, &r.Bucket, &r.Reason, &r.N, &r.Reassigned); err != nil {
			return nil, fmt.Errorf("scan cl coverage row: %w", err)
		}
		if !clKnownBuckets[r.Bucket] {
			return nil, fmt.Errorf("cl coverage: unknown bucket %q for month %s cohort %s", r.Bucket, r.Month, r.Cohort)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cl coverage rows: %w", err)
	}

	return foldCLCoverage(out), nil
}
