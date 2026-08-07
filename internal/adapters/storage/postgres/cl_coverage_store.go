package postgres

import (
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
// this constant by hand. TestCLCoverageEraStart_IsPinned asserts the value so
// the change cannot happen by accident.
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
