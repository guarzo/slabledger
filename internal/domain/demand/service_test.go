package demand_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

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

// uncoveredCampaigns is a CampaignCoverageLookup that returns no coverage for
// any bucket — useful as a baseline.
type uncoveredCampaigns struct{}

func (uncoveredCampaigns) CampaignsCovering(ctx context.Context, character, era string, grade int) ([]int64, error) {
	return nil, nil
}
func (uncoveredCampaigns) UnsoldCountFor(ctx context.Context, character, era string, grade int) (int, error) {
	return 0, nil
}
func (uncoveredCampaigns) ActiveCampaigns(ctx context.Context) ([]demand.ActiveCampaign, error) {
	return []demand.ActiveCampaign{}, nil
}

// coveredOnlyFor covers a single bucket; everything else is uncovered.
type coveredOnlyFor struct {
	character string
	era       string
	grade     int
	ids       []int64
	unsold    int
}

func (c coveredOnlyFor) CampaignsCovering(ctx context.Context, character, era string, grade int) ([]int64, error) {
	if character == c.character && era == c.era && grade == c.grade {
		return c.ids, nil
	}
	return nil, nil
}
func (c coveredOnlyFor) UnsoldCountFor(ctx context.Context, character, era string, grade int) (int, error) {
	if character == c.character && era == c.era && grade == c.grade {
		return c.unsold, nil
	}
	return 0, nil
}
func (coveredOnlyFor) ActiveCampaigns(ctx context.Context) ([]demand.ActiveCampaign, error) {
	return []demand.ActiveCampaign{}, nil
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
	svc := demand.NewService(repo, uncoveredCampaigns{})

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
	svc := demand.NewService(newRepoWithRows(nil), uncoveredCampaigns{})
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
	svc := demand.NewService(repo, uncoveredCampaigns{})

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
	svc := demand.NewService(newRepoWithRows(rows), uncoveredCampaigns{})

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
	svc := demand.NewService(newRepoWithRows(rows), uncoveredCampaigns{})

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
	svc := demand.NewService(newRepoWithRows(rows), coveredOnlyFor{
		character: "CoveredChar",
		era:       "sword_shield",
		grade:     10,
		ids:       []int64{42},
		unsold:    3,
	})

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
	svc := demand.NewService(newRepoWithRows(rows), uncoveredCampaigns{})

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
	svc := demand.NewService(newRepoWithRows(rows), uncoveredCampaigns{})

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
	svc := demand.NewService(newRepoWithRows(rows), uncoveredCampaigns{})

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
