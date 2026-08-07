package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// The era literal in scripts/cl-coverage.sql is a hand-maintained copy of
// CLCoverageEraStart -- the script must run with zero application dependencies
// during an incident, so it cannot read the constant. This test is what makes
// that copy enforced rather than merely documented: change the constant without
// updating the script and this fails.
func TestCLCoverageScript_MatchesEraStart(t *testing.T) {
	ts, err := clCoverageEraTimestamp()
	require.NoError(t, err)

	// Test binaries run in their package directory; the repo root is four up.
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "cl-coverage.sql"))
	require.NoError(t, err)

	// Comment lines are excluded: the script's header quotes the same literal
	// while explaining the hand-sync rule, and matching that would let the
	// executable literal drift while the test stayed green.
	var stmt strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		stmt.WriteString(line)
		stmt.WriteString("\n")
	}

	assert.Contains(t, stmt.String(), "TIMESTAMP '"+ts+"'",
		"scripts/cl-coverage.sql must carry the CLCoverageEraStart literal")
}

func TestFoldCLCoverage(t *testing.T) {
	pct := func(f float64) *float64 { return &f }

	tests := []struct {
		name string
		rows []clCoverageRow
		want *CLCoverageReport
		// wantMonthOrder, when set, asserts the month sequence instead of the
		// full report. Used for the ordering case, where the report body is
		// uninteresting; want stays nil-able for its own sake elsewhere.
		wantMonthOrder []string
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
			wantMonthOrder: []string{"2026-08", "2026-07", "2026-06"},
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
			if tt.wantMonthOrder != nil {
				var months []string
				for _, m := range got.Months {
					months = append(months, m.Month)
				}
				assert.Equal(t, tt.wantMonthOrder, months)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// coverageSeed is one fixture row for seedCoveragePurchase.
type coverageSeed struct {
	ID, CampaignID, PurchaseDate, Source, CLUpdatedAt, LastError string
	CLValueCents                                                 int
	CreatedAt                                                    time.Time
}

// seedCoveragePurchase inserts one purchase with the columns this query reads,
// plus the three NOT NULL columns that have no schema default (card_name,
// cert_number, purchase_date). cert_number must be distinct per row --
// UNIQUE(grader, cert_number) at initial_schema:232 -- so it is derived from ID.
func seedCoveragePurchase(t *testing.T, db *DB, p coverageSeed) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO campaign_purchases
		   (id, campaign_id, card_name, cert_number, purchase_date, purchase_source,
		    cl_value_updated_at, cl_last_error, cl_value_cents, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		p.ID, p.CampaignID, "Card "+p.ID, "cert-"+p.ID, p.PurchaseDate, p.Source,
		p.CLUpdatedAt, p.LastError, p.CLValueCents, p.CreatedAt)
	require.NoError(t, err, "seed purchase %s", p.ID)
}

func TestGetCLCoverageByMonth_Classification(t *testing.T) {
	db := setupTestDB(t)
	store := NewCardLadderStore(db.DB, nil)
	ctx := context.Background()

	preEra := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	postEra := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase) VALUES
		   ('open-camp', 'Open', 'active'),
		   ('closed-camp', 'Closed', 'closed'),
		   ('external', 'External', 'active')`)
	require.NoError(t, err)

	for _, p := range []coverageSeed{
		// resolved: timestamp set, value 0. CL spoke; cl_value_cents is not
		// the authority.
		{ID: "r1", CampaignID: "open-camp", PurchaseDate: "2026-06-01", Source: "Grading",
			CLUpdatedAt: "2026-06-02T00:00:00Z", CLValueCents: 0, CreatedAt: postEra},

		// pre_cl, NOT resolved: positive cl_value_cents with no CL timestamp,
		// created before the era. This is the Shopify-contamination case and is
		// the single most important assertion in this suite -- under the old
		// cl_value_cents > 0 predicate this row counts as CL-priced, and it
		// never was.
		//
		// It is given a linked sale below (sale-3) so it is sweep-INELIGIBLE.
		// That is not incidental: eligibility is tested before the era cutoff,
		// so a pre-era row that is still unsold and in an open campaign is
		// correctly 'pending' (the sweep will get to it), not 'pre_cl'. All 339
		// real pre_cl rows in production are already sold, which is why this
		// fixture models a sold one.
		{ID: "p1", CampaignID: "external", PurchaseDate: "2026-06-01",
			CLValueCents: 12345, CreatedAt: preEra},

		// unresolved: asked, no comparable.
		{ID: "u1", CampaignID: "open-camp", PurchaseDate: "2026-06-01",
			LastError: "no_value", CreatedAt: postEra},

		// pending via quota, POST-era.
		{ID: "q1", CampaignID: "open-camp", PurchaseDate: "2026-06-01",
			LastError: "quota_exhausted", CreatedAt: postEra},

		// pending via quota, PRE-era. NOTE: this row is classified 'pending' by
		// eligibility alone (open campaign, unsold) -- it does NOT, by itself,
		// pin either the eligibility-before-era-cutoff ordering or the
		// cl_last_error = '' guard on the pre_cl arm, because eligibility
		// catches it before either mutation could matter. It is kept as a
		// sanity check that a quota-marked pre-era row still reads as pending
		// while it remains sweep-reachable. gapA and gapB below are the rows
		// that actually discriminate the ordering and the guard.
		{ID: "q2", CampaignID: "open-camp", PurchaseDate: "2026-06-01",
			LastError: "quota_exhausted", CreatedAt: preEra},

		// gapA: pins eligibility-before-era-cutoff. Open campaign, unsold,
		// cl_last_error '', created before the era. Under the committed
		// classifier eligibility is tested first, so this reads 'pending'.
		// If the eligibility and era-cutoff arms were swapped, the era check
		// would fire first and this row would misclassify as 'pre_cl'.
		{ID: "gapA", CampaignID: "open-camp", PurchaseDate: "2026-06-01", CreatedAt: preEra},

		// gapB: pins the cl_last_error = '' guard on the pre_cl arm. Sold (so
		// ineligible for the sweep), cl_last_error 'quota_exhausted', created
		// before the era. Under the committed classifier the guard rejects it
		// from pre_cl, so it falls to 'stranded'. If the guard were dropped,
		// this row would misclassify as 'pre_cl' even though the sweep
		// demonstrably reached it.
		{ID: "gapB", CampaignID: "open-camp", PurchaseDate: "2026-06-01",
			LastError: "quota_exhausted", CreatedAt: preEra},

		// pending: post-era, no trace, still sweep-eligible.
		{ID: "n1", CampaignID: "open-camp", PurchaseDate: "2026-06-01", CreatedAt: postEra},

		// stranded: post-era, no trace, campaign closed -> never swept again.
		{ID: "s1", CampaignID: "closed-camp", PurchaseDate: "2026-06-01", CreatedAt: postEra},

		// stranded: post-era, no trace, has a linked sale -> never swept again.
		{ID: "s2", CampaignID: "open-camp", PurchaseDate: "2026-06-01", CreatedAt: postEra},

		// stranded: quota-marked AND sold. The sweep skipped it at the quota
		// wall, then it sold, and ListAllUnsoldPurchases (purchase_store.go:177)
		// will never return it again -- nothing clears cl_last_error on sale.
		// This row must NOT read as pending: it is waiting for a retry that can
		// never come. Ordering the quota arm before the eligibility arm parks it
		// in pending forever, which is exactly the false-reassurance this whole
		// endpoint exists to prevent.
		{ID: "s3", CampaignID: "open-camp", PurchaseDate: "2026-06-01",
			LastError: "quota_exhausted", CreatedAt: postEra},
	} {
		seedCoveragePurchase(t, db, p)
	}

	// Sales for p1, s2, s3 and gapB. Also proves sold rows stay IN the report --
	// GetCLPriceStats would drop them. sale_channel is NOT NULL with no default,
	// and campaign_sales carries UNIQUE(purchase_id), so one row per purchase.
	_, err = db.ExecContext(ctx,
		`INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_date, sale_price_cents)
		 VALUES ('sale-1', 's2', 'ebay', '2026-06-15', 5000),
		        ('sale-2', 's3', 'ebay', '2026-06-16', 6000),
		        ('sale-3', 'p1', 'ebay', '2026-06-17', 7000),
		        ('sale-4', 'gapB', 'ebay', '2026-06-18', 8000)`)
	require.NoError(t, err)

	got, err := store.GetCLCoverageByMonth(ctx)
	require.NoError(t, err)
	require.Len(t, got.Months, 1)

	m := got.Months[0]
	assert.Equal(t, "2026-06", m.Month)
	assert.Equal(t, CLCoverageEraStart, got.EraStart)

	// campaign cohort = purchase_source non-empty = r1 only.
	assert.Equal(t, 1, m.Campaign.Rows)
	assert.Equal(t, 1, m.Campaign.Resolved)

	// external cohort = the other ten (eight originals + gapA + gapB).
	assert.Equal(t, 10, m.External.Rows)
	assert.Equal(t, 0, m.External.Resolved)
	assert.Equal(t, 1, m.External.Unresolved, "u1")
	assert.Equal(t, 4, m.External.Pending, "q1, q2, n1, gapA")
	assert.Equal(t, 4, m.External.Stranded, "s1 closed, s2 sold, s3 quota+sold, gapB quota+sold+pre-era")
	assert.Equal(t, 1, m.External.PreCL, "p1 -- Shopify value, never CL-priced")

	// Denominator excludes pending/stranded/preCL: 0 resolved of 1 judged.
	require.NotNil(t, m.External.Pct)
	assert.Equal(t, 0.0, *m.External.Pct)
	assert.Equal(t, map[string]int{"no_value": 1}, m.UnresolvedByReason)
}

func TestGetCLCoverageByMonth_ReassignedCohortDrift(t *testing.T) {
	db := setupTestDB(t)
	store := NewCardLadderStore(db.DB, nil)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase) VALUES ('external', 'External', 'active')`)
	require.NoError(t, err)

	// purchase_source set but campaign_id reassigned to 'external': counts in
	// the campaign cohort (intake origin wins) AND increments reassigned.
	seedCoveragePurchase(t, db, coverageSeed{
		ID: "x1", CampaignID: "external", PurchaseDate: "2026-05-04", Source: "Vault",
		CLUpdatedAt: "2026-05-05T00:00:00Z",
		CreatedAt:   time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
	})

	got, err := store.GetCLCoverageByMonth(ctx)
	require.NoError(t, err)
	require.Len(t, got.Months, 1)

	assert.Equal(t, 1, got.Months[0].Campaign.Rows, "classified by intake origin")
	assert.Equal(t, 0, got.Months[0].External.Rows)
	assert.Equal(t, 1, got.Months[0].Reassigned, "drift is surfaced, not hidden")
}
