package demand_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func strPtr(s string) *string { return &s }

// newRepoWithRows builds a DemandRepositoryMock whose ListCharacterCache
// returns the given rows verbatim.
func newRepoWithRows(rows []demand.CharacterCache) *mocks.DemandRepositoryMock {
	return &mocks.DemandRepositoryMock{
		ListCharacterCacheFn: func(ctx context.Context, window string) ([]demand.CharacterCache, error) {
			return rows, nil
		},
	}
}

// uncoveredLookup returns a CampaignCoverageLookupMock baseline: no coverage
// on any bucket, no active campaigns. The mock's default Fn-nil behavior
// already does this, but the named helper makes test intent explicit.
func uncoveredLookup() *mocks.CampaignCoverageLookupMock {
	return &mocks.CampaignCoverageLookupMock{}
}

// coveredOnlyForLookup returns a lookup mock that claims coverage for a
// single (character, era, grade) bucket; everything else is uncovered.
func coveredOnlyForLookup(character, era string, grade int, ids []int64, unsold int) *mocks.CampaignCoverageLookupMock {
	return &mocks.CampaignCoverageLookupMock{
		CampaignsCoveringFn: func(_ context.Context, c, e string, g int) ([]int64, error) {
			if c == character && e == era && g == grade {
				return ids, nil
			}
			return nil, nil
		},
		UnsoldCountForFn: func(_ context.Context, c, e string, g int) (int, error) {
			if c == character && e == era && g == grade {
				return unsold, nil
			}
			return 0, nil
		},
	}
}

// --- Fixtures ---

func floatStr(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// demandJSONWithEras constructs a demand_json blob for a character with two
// eras. baseScore is the character-level avg; per-era scores are baseScore±0.05.
func demandJSONWithEras(character string, baseScore float64, quality string) string {
	return `{
		"character_name": "` + character + `",
		"card_count": 10,
		"avg_demand_score": ` + floatStr(baseScore) + `,
		"total_views": 400,
		"total_search_clicks": 80,
		"total_wishlist_adds": 20,
		"data_quality": "` + quality + `",
		"by_era": {
			"sword_shield":    {"card_count": 6, "avg_demand_score": ` + floatStr(baseScore+0.05) + `, "total_views": 240, "total_wishlist_adds": 12, "data_quality": "` + quality + `"},
			"scarlet_violet":  {"card_count": 4, "avg_demand_score": ` + floatStr(baseScore-0.05) + `, "total_views": 160, "total_wishlist_adds":  8, "data_quality": "` + quality + `"}
		}
	}`
}

// --- Tests ---

func TestService_Leaderboard_EmptyCache(t *testing.T) {
	repo := newRepoWithRows(nil)
	svc := demand.NewService(repo, uncoveredLookup())

	out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
		Window: "30d",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty leaderboard; got %d", len(out))
	}
}

func TestService_Leaderboard_InvalidWindow(t *testing.T) {
	svc := demand.NewService(newRepoWithRows(nil), uncoveredLookup())
	_, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{Window: "60d"})
	if !errors.Is(err, demand.ErrInvalidWindow) {
		t.Fatalf("expected ErrInvalidWindow; got %v", err)
	}
}

func TestService_Leaderboard_GradeFilter_BucketCountAndSort(t *testing.T) {
	// 3 characters, each with 2 eras in demand_json. Grade filter 10 → exactly
	// one grade bucket per (character, era) → 3 * 2 = 6 buckets.
	rows := []demand.CharacterCache{
		{Character: "Umbreon", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("Umbreon", 0.9, "full"))},
		{Character: "Charizard", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("Charizard", 0.6, "full"))},
		{Character: "Blastoise", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("Blastoise", 0.3, "full"))},
	}
	repo := newRepoWithRows(rows)
	svc := demand.NewService(repo, uncoveredLookup())

	out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
		Window: "30d",
		Grade:  10,
		Sort:   demand.SortOpportunityScore,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 buckets (3 chars × 2 eras × 1 grade); got %d", len(out))
	}
	for _, o := range out {
		if o.Grade != 10 {
			t.Fatalf("bucket has grade=%d; want 10", o.Grade)
		}
	}
	// Sort order: opportunity score desc. With uncovered coverage, nil velocity
	// and no saturation, OpportunityScore == demand.Score. So Umbreon first,
	// Blastoise last.
	if out[0].Character != "Umbreon" {
		t.Fatalf("first bucket should be Umbreon; got %s", out[0].Character)
	}
	if out[len(out)-1].Character != "Blastoise" {
		t.Fatalf("last bucket should be Blastoise; got %s", out[len(out)-1].Character)
	}
}

func TestService_Leaderboard_MinDataQualityFull_ExcludesProxy(t *testing.T) {
	rows := []demand.CharacterCache{
		{Character: "ProxyChar", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("ProxyChar", 0.9, "proxy"))},
		{Character: "FullChar", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("FullChar", 0.6, "full"))},
	}
	svc := demand.NewService(newRepoWithRows(rows), uncoveredLookup())

	out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
		Window:         "30d",
		Grade:          10,
		MinDataQuality: demand.QualityFull,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, o := range out {
		if o.Character == "ProxyChar" {
			t.Fatalf("proxy row should have been filtered out")
		}
		if o.Demand == nil || o.Demand.DataQuality != demand.QualityFull {
			t.Fatalf("expected full-quality demand; got %+v", o.Demand)
		}
	}
	if len(out) == 0 {
		t.Fatalf("want at least the FullChar buckets; got 0")
	}
}

func TestService_Leaderboard_SortDemandScoreDesc(t *testing.T) {
	rows := []demand.CharacterCache{
		{Character: "Low", Window: "7d", DemandJSON: strPtr(demandJSONWithEras("Low", 0.2, "full"))},
		{Character: "High", Window: "7d", DemandJSON: strPtr(demandJSONWithEras("High", 0.8, "full"))},
	}
	svc := demand.NewService(newRepoWithRows(rows), uncoveredLookup())

	out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
		Window: "7d",
		Grade:  10,
		Sort:   demand.SortDemandScore,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out[0].Character != "High" {
		t.Fatalf("expected High first under demand_score sort; got %s", out[0].Character)
	}
}

func TestService_Leaderboard_SortLowCoverage_UncoveredFirst(t *testing.T) {
	rows := []demand.CharacterCache{
		{Character: "CoveredChar", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("CoveredChar", 0.9, "full"))},
		{Character: "UncoveredChar", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("UncoveredChar", 0.5, "full"))},
	}
	// Only a single grade-10 sword_shield bucket for CoveredChar is covered;
	// everything else (including its scarlet_violet bucket) is uncovered.
	svc := demand.NewService(newRepoWithRows(rows), coveredOnlyForLookup("CoveredChar", "sword_shield", 10, []int64{42}, 3))

	out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
		Window: "30d",
		Grade:  10,
		Sort:   demand.SortLowCoverage,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sawCovered := false
	for _, o := range out {
		if o.Coverage.Covered {
			sawCovered = true
			continue
		}
		if sawCovered {
			t.Fatalf("uncovered bucket after covered one: %+v", o)
		}
	}
}

func TestService_Leaderboard_EraFilter(t *testing.T) {
	rows := []demand.CharacterCache{
		{Character: "C1", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("C1", 0.7, "full"))},
	}
	svc := demand.NewService(newRepoWithRows(rows), uncoveredLookup())

	out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
		Window: "30d",
		Era:    "sword_shield",
		Grade:  10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 bucket (1 char × 1 era × 1 grade); got %d", len(out))
	}
	if out[0].Era != "sword_shield" {
		t.Fatalf("want era=sword_shield; got %q", out[0].Era)
	}
}

func TestService_Leaderboard_DefaultGrades_EmitsAllFour(t *testing.T) {
	rows := []demand.CharacterCache{
		{Character: "C1", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("C1", 0.7, "full"))},
	}
	svc := demand.NewService(newRepoWithRows(rows), uncoveredLookup())

	out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
		Window: "30d",
		Era:    "sword_shield",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("want 4 buckets (default grade set 7/8/9/10); got %d", len(out))
	}
	gradesSeen := map[int]bool{}
	for _, o := range out {
		gradesSeen[o.Grade] = true
	}
	for _, g := range []int{7, 8, 9, 10} {
		if !gradesSeen[g] {
			t.Fatalf("grade %d missing from default-grade output", g)
		}
	}
}

func TestService_Leaderboard_LimitTruncates(t *testing.T) {
	rows := []demand.CharacterCache{
		{Character: "A", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("A", 0.9, "full"))},
		{Character: "B", Window: "30d", DemandJSON: strPtr(demandJSONWithEras("B", 0.5, "full"))},
	}
	svc := demand.NewService(newRepoWithRows(rows), uncoveredLookup())

	out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
		Window: "30d",
		Grade:  10,
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want limit=2; got %d", len(out))
	}
}

func TestService_Leaderboard_Acceleration(t *testing.T) {
	computedAt := time.Date(2026, 4, 15, 3, 0, 0, 0, time.UTC)
	velocityWithChange := `{"median_days_to_sell":9.5,"sample_size":120,"velocity_change_pct":14.2}`

	tests := []struct {
		name                    string
		rows                    []demand.CharacterCache
		wantAccelerationPresent bool
		wantMedianVelocity      float64
		wantAcceleratingCount   int
		wantTotalCount          int
	}{
		{
			name: "populated when row has velocity_change_pct",
			rows: []demand.CharacterCache{{
				Character:           "Umbreon",
				Window:              "30d",
				DemandJSON:          strPtr(demandJSONWithEras("Umbreon", 0.9, "full")),
				VelocityJSON:        strPtr(velocityWithChange),
				AnalyticsComputedAt: &computedAt,
			}},
			wantAccelerationPresent: true,
			wantMedianVelocity:      14.2,
			wantAcceleratingCount:   1,
			wantTotalCount:          1,
		},
		{
			name: "nil when row has no velocity data",
			rows: []demand.CharacterCache{{
				Character:  "Umbreon",
				Window:     "30d",
				DemandJSON: strPtr(demandJSONWithEras("Umbreon", 0.9, "full")),
			}},
			wantAccelerationPresent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := demand.NewService(newRepoWithRows(tc.rows), uncoveredLookup())
			out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{
				Window: "30d",
				Era:    "sword_shield",
				Grade:  10,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out) != 1 {
				t.Fatalf("want 1 bucket; got %d", len(out))
			}
			o := out[0]
			if !tc.wantAccelerationPresent {
				if o.Acceleration != nil {
					t.Fatalf("want Acceleration nil; got %+v", o.Acceleration)
				}
				return
			}
			if o.Acceleration == nil {
				t.Fatalf("want Acceleration populated; got nil")
			}
			if o.Acceleration.MedianVelocityChangePct != tc.wantMedianVelocity {
				t.Errorf("want MedianVelocityChangePct=%v; got %v", tc.wantMedianVelocity, o.Acceleration.MedianVelocityChangePct)
			}
			if o.Acceleration.TotalCount != tc.wantTotalCount {
				t.Errorf("want TotalCount=%d; got %d", tc.wantTotalCount, o.Acceleration.TotalCount)
			}
			if o.Acceleration.AcceleratingCount != tc.wantAcceleratingCount {
				t.Errorf("want AcceleratingCount=%d; got %d", tc.wantAcceleratingCount, o.Acceleration.AcceleratingCount)
			}
			if o.Acceleration.DataQuality != demand.QualityFull {
				t.Errorf("want DataQuality=%q; got %q", demand.QualityFull, o.Acceleration.DataQuality)
			}
		})
	}
}

// --- Standalone scoring tests ---

func TestOpportunityScore_CoverageAndSaturation(t *testing.T) {
	base := demand.OpportunityScore(0.8, nil, 10, demand.NicheCoverage{})
	covered := demand.OpportunityScore(0.8, nil, 10, demand.NicheCoverage{Covered: true, ActiveCampaignIDs: []int64{1}})
	saturated := demand.OpportunityScore(0.8, nil, 500, demand.NicheCoverage{})

	if !(base > covered) {
		t.Fatalf("coverage penalty not applied: base=%f covered=%f", base, covered)
	}
	if !(base > saturated) {
		t.Fatalf("saturation penalty not applied: base=%f saturated=%f", base, saturated)
	}
}

func TestOpportunityScore_VelocityClamp(t *testing.T) {
	// velocityChangePct is DH's percentage-point form (15.2 = +15.2%), so
	// anything with |v| > 50 must clamp to the ±0.5 fractional ceiling.
	big := 100.0
	neg := -100.0
	highClamp := demand.OpportunityScore(1.0, &big, 0, demand.NicheCoverage{})
	lowClamp := demand.OpportunityScore(1.0, &neg, 0, demand.NicheCoverage{})
	if highClamp != 1.5 {
		t.Fatalf("velocity high-clamp wrong; want 1.5, got %f", highClamp)
	}
	if lowClamp != 0.5 {
		t.Fatalf("velocity low-clamp wrong; want 0.5, got %f", lowClamp)
	}
}
