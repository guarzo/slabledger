package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The era start is a pinned historical fact, not a tunable. This test exists so
// that changing it is a deliberate act with a visible diff rather than a silent
// reclassification of every pre-2026-04-13 row.
func TestCLCoverageEraStart_IsPinned(t *testing.T) {
	assert.Equal(t, "2026-04-13T04:00:13Z", CLCoverageEraStart)

	ts, err := clCoverageEraTimestamp()
	require.NoError(t, err)
	assert.Equal(t, "2026-04-13 04:00:13", ts)
}

func TestFoldCLCoverage(t *testing.T) {
	pct := func(f float64) *float64 { return &f }

	tests := []struct {
		name string
		rows []clCoverageRow
		want *CLCoverageReport
	}{
		{
			name: "pending and preCL are excluded from the denominator",
			// This is the regression case. Under the old cl_value_cents > 0
			// predicate these 25 rows read as 7/25 = 28%. Here the 18 unswept
			// rows are pending, so the denominator is 7 and the answer is 100%.
			rows: []clCoverageRow{
				{Month: "2026-08", Cohort: "campaign", Bucket: "resolved", N: 7},
				{Month: "2026-08", Cohort: "campaign", Bucket: "pending", N: 18},
			},
			want: &CLCoverageReport{
				EraStart: CLCoverageEraStart,
				Months: []CLCoverageMonth{{
					Month:              "2026-08",
					UnresolvedByReason: map[string]int{},
					Campaign: CLCoverageCohort{
						Rows: 25, Resolved: 7, Pending: 18, Pct: pct(100.0),
					},
					External: CLCoverageCohort{Pct: nil},
				}},
			},
		},
		{
			name: "empty denominator yields null pct, not zero",
			rows: []clCoverageRow{
				{Month: "2026-03", Cohort: "external", Bucket: "pre_cl", N: 144},
			},
			want: &CLCoverageReport{
				EraStart: CLCoverageEraStart,
				Months: []CLCoverageMonth{{
					Month:              "2026-03",
					UnresolvedByReason: map[string]int{},
					Campaign:           CLCoverageCohort{Pct: nil},
					External:           CLCoverageCohort{Rows: 144, PreCL: 144, Pct: nil},
				}},
			},
		},
		{
			name: "unresolved reasons roll up per month across cohorts",
			rows: []clCoverageRow{
				{Month: "2026-07", Cohort: "external", Bucket: "resolved", N: 56},
				{Month: "2026-07", Cohort: "external", Bucket: "unresolved", Reason: "no_value", N: 15},
				{Month: "2026-07", Cohort: "campaign", Bucket: "unresolved", Reason: "api_error", N: 2},
			},
			want: &CLCoverageReport{
				EraStart: CLCoverageEraStart,
				Months: []CLCoverageMonth{{
					Month:              "2026-07",
					UnresolvedByReason: map[string]int{"no_value": 15, "api_error": 2},
					Campaign:           CLCoverageCohort{Rows: 2, Unresolved: 2, Pct: pct(0.0)},
					External:           CLCoverageCohort{Rows: 71, Resolved: 56, Unresolved: 15, Pct: pct(78.9)},
				}},
			},
		},
		{
			name: "reassigned accumulates across every group in a month",
			rows: []clCoverageRow{
				{Month: "2026-05", Cohort: "campaign", Bucket: "resolved", N: 100, Reassigned: 5},
				{Month: "2026-05", Cohort: "campaign", Bucket: "stranded", N: 37, Reassigned: 2},
			},
			want: &CLCoverageReport{
				EraStart: CLCoverageEraStart,
				Months: []CLCoverageMonth{{
					Month:              "2026-05",
					Reassigned:         7,
					UnresolvedByReason: map[string]int{},
					Campaign: CLCoverageCohort{
						Rows: 137, Resolved: 100, Stranded: 37, Pct: pct(100.0),
					},
					External: CLCoverageCohort{Pct: nil},
				}},
			},
		},
		{
			name: "month order follows query order, newest first",
			rows: []clCoverageRow{
				{Month: "2026-08", Cohort: "campaign", Bucket: "resolved", N: 1},
				{Month: "2026-07", Cohort: "campaign", Bucket: "resolved", N: 1},
				{Month: "2026-06", Cohort: "campaign", Bucket: "resolved", N: 1},
			},
			want: nil, // asserted separately below
		},
		{
			name: "no rows yields an empty months slice, not nil",
			rows: nil,
			want: &CLCoverageReport{EraStart: CLCoverageEraStart, Months: []CLCoverageMonth{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := foldCLCoverage(tt.rows)
			if tt.want == nil {
				var months []string
				for _, m := range got.Months {
					months = append(months, m.Month)
				}
				assert.Equal(t, []string{"2026-08", "2026-07", "2026-06"}, months)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
