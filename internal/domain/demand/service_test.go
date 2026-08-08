package demand_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

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
func coveredOnlyForLookup(character, era string, grade int, ids []string, unsold int) *mocks.CampaignCoverageLookupMock {
	return &mocks.CampaignCoverageLookupMock{
		CampaignsCoveringFn: func(_ context.Context, c, e string, g int) ([]string, error) {
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

func f64Ptr(f float64) *float64 { return &f }

// demandPayload constructs a CharacterDemand payload for a character with two
// eras. baseScore is the character-level avg; per-era scores are baseScore±0.05.
func demandPayload(character string, baseScore float64, quality string) *demand.CharacterDemand {
	return &demand.CharacterDemand{
		CharacterName:     character,
		CardCount:         10,
		AvgDemandScore:    baseScore,
		TotalViews:        400,
		TotalSearchClicks: 80,
		TotalWishlistAdds: 20,
		DataQuality:       quality,
		ByEra: map[string]demand.ByEraDemand{
			"sword_shield":   {CardCount: 6, AvgDemandScore: baseScore + 0.05, TotalViews: 240, TotalWishlistAdds: 12},
			"scarlet_violet": {CardCount: 4, AvgDemandScore: baseScore - 0.05, TotalViews: 160, TotalWishlistAdds: 8},
		},
	}
}

// --- Tests ---

// demandWithComputedAt builds a payload carrying a response-level computed_at.
// withEras controls whether the character has a ByEra map, which is what
// decides between era buckets and the single "" character-overall bucket.
func demandWithComputedAt(character, computedAt string, withEras bool) *demand.CharacterDemand {
	d := &demand.CharacterDemand{
		CharacterName:     character,
		CardCount:         10,
		AvgDemandScore:    0.8,
		TotalViews:        400,
		TotalSearchClicks: 80,
		TotalWishlistAdds: 20,
		ComputedAt:        computedAt,
	}
	if withEras {
		d.ByEra = map[string]demand.ByEraDemand{
			"sword_shield": {CardCount: 6, AvgDemandScore: 0.85, TotalViews: 240, TotalWishlistAdds: 12},
		}
	}
	return d
}

// TestService_Leaderboard_ComputedAt covers the timestamp the scheduler now
// writes into the demand payload. Era rows deliberately inherit the
// character-level value — DH reports computed_at once per response, not per
// by_era bucket (see eraDemandFor).
func TestService_Leaderboard_ComputedAt(t *testing.T) {
	const stamp = "2026-04-15T00:00:00Z"
	want, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("bad fixture timestamp: %v", err)
	}

	tests := []struct {
		name     string
		withEras bool
		wantEra  string
	}{
		{name: "character overall row", withEras: false, wantEra: ""},
		{name: "era row inherits character timestamp", withEras: true, wantEra: "sword_shield"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows := []demand.CharacterCache{{
				Character: "Umbreon",
				Window:    "30d",
				Demand:    demandWithComputedAt("Umbreon", stamp, tc.withEras),
			}}
			svc := demand.NewService(newRepoWithRows(rows), uncoveredLookup())

			out, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{Window: "30d", Limit: 10})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out) == 0 {
				t.Fatalf("expected at least one opportunity")
			}
			got := out[0]
			if got.Era != tc.wantEra {
				t.Errorf("Era = %q; want %q", got.Era, tc.wantEra)
			}
			if got.Demand == nil {
				t.Fatalf("Demand is nil")
			}
			if !got.Demand.ComputedAt.Equal(want) {
				t.Errorf("ComputedAt = %s; want %s", got.Demand.ComputedAt, want)
			}
		})
	}
}

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
		{Character: "Umbreon", Window: "30d", Demand: demandPayload("Umbreon", 0.9, "full")},
		{Character: "Charizard", Window: "30d", Demand: demandPayload("Charizard", 0.6, "full")},
		{Character: "Blastoise", Window: "30d", Demand: demandPayload("Blastoise", 0.3, "full")},
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
		{Character: "ProxyChar", Window: "30d", Demand: demandPayload("ProxyChar", 0.9, "proxy")},
		{Character: "FullChar", Window: "30d", Demand: demandPayload("FullChar", 0.6, "full")},
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

func TestService_Leaderboard_EraBucketsInheritCharacterQuality(t *testing.T) {
	// The shape the refresh scheduler actually writes: data_quality is stamped
	// on the character (rolled up from its cards) and DH's by_era entries carry
	// none. Era buckets therefore have to inherit it, or MinDataQuality would
	// discard every era bucket in production (SLA-63).
	payload := func(character, quality string, score float64) *demand.CharacterDemand {
		return &demand.CharacterDemand{
			CharacterName:     character,
			CardCount:         10,
			AvgDemandScore:    score,
			TotalViews:        400,
			TotalWishlistAdds: 20,
			DataQuality:       quality,
			ByEra: map[string]demand.ByEraDemand{
				"sword_shield": {CardCount: 6, AvgDemandScore: score, TotalViews: 240, TotalWishlistAdds: 12},
			},
		}
	}
	rows := []demand.CharacterCache{
		{Character: "FullChar", Window: "30d", Demand: payload("FullChar", demand.QualityFull, 0.6)},
		{Character: "ProxyChar", Window: "30d", Demand: payload("ProxyChar", demand.QualityProxy, 0.9)},
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
	if len(out) == 0 {
		t.Fatalf("want the FullChar era bucket; got 0 — era buckets are not inheriting character quality")
	}
	for _, o := range out {
		if o.Character != "FullChar" {
			t.Fatalf("bucket for %q survived a full-quality filter", o.Character)
		}
		if o.Demand == nil || o.Demand.DataQuality != demand.QualityFull {
			t.Fatalf("bucket %q/%q has demand %+v; want quality %q", o.Character, o.Era, o.Demand, demand.QualityFull)
		}
	}
}

func TestService_Leaderboard_SortDemandScoreDesc(t *testing.T) {
	rows := []demand.CharacterCache{
		{Character: "Low", Window: "7d", Demand: demandPayload("Low", 0.2, "full")},
		{Character: "High", Window: "7d", Demand: demandPayload("High", 0.8, "full")},
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
		{Character: "CoveredChar", Window: "30d", Demand: demandPayload("CoveredChar", 0.9, "full")},
		{Character: "UncoveredChar", Window: "30d", Demand: demandPayload("UncoveredChar", 0.5, "full")},
	}
	// Only a single grade-10 sword_shield bucket for CoveredChar is covered;
	// everything else (including its scarlet_violet bucket) is uncovered.
	svc := demand.NewService(newRepoWithRows(rows), coveredOnlyForLookup("CoveredChar", "sword_shield", 10, []string{"c42"}, 3))

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
		{Character: "C1", Window: "30d", Demand: demandPayload("C1", 0.7, "full")},
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
		{Character: "C1", Window: "30d", Demand: demandPayload("C1", 0.7, "full")},
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
		{Character: "A", Window: "30d", Demand: demandPayload("A", 0.9, "full")},
		{Character: "B", Window: "30d", Demand: demandPayload("B", 0.5, "full")},
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
	velocityWithChange := &demand.CharacterVelocity{MedianDaysToSell: f64Ptr(9.5), SampleSize: 120, VelocityChangePct: f64Ptr(14.2)}

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
				Demand:              demandPayload("Umbreon", 0.9, "full"),
				Velocity:            velocityWithChange,
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
				Character: "Umbreon",
				Window:    "30d",
				Demand:    demandPayload("Umbreon", 0.9, "full"),
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

// TestService_Leaderboard_ActiveListingCount asserts that a row's Saturation
// payload reaches NicheOpportunity.Market.ActiveListingCount unchanged. This
// guards the read side of the Defect-1 fix: the write side
// (mapCharacterSaturation) and the JSON decode are both tested elsewhere, but
// nothing previously checked that the decoded value survives all the way to
// what the API returns. A regression here — e.g. mapCharacterSaturation
// reverting to marshal the whole nested entry — would otherwise pass every
// existing test and silently zero the count again.
// TestService_Leaderboard_ActiveListingCount asserts that a row's Saturation
// payload reaches NicheOpportunity.Market.ActiveListingCount unchanged. This
// guards the read side of the Defect-1 fix: the write side
// (mapCharacterSaturation) and the JSON decode are both tested elsewhere, but
// nothing previously checked that the decoded value survives all the way to
// what the API returns. A regression here — e.g. mapCharacterSaturation
// reverting to marshal the whole nested entry — would otherwise pass every
// existing test and silently zero the count again.
func TestService_Leaderboard_ActiveListingCount(t *testing.T) {
	tests := []struct {
		name       string
		row        demand.CharacterCache
		wantMarket bool
		wantCount  int
	}{
		{
			name: "decoded saturation survives to the API view",
			row: demand.CharacterCache{
				Character:  "Pikachu",
				Window:     "30d",
				Demand:     demandPayload("Pikachu", 0.9, "full"),
				Saturation: &demand.CharacterSaturation{ActiveListingCount: 42},
			},
			wantMarket: true,
			wantCount:  42,
		},
		{
			name: "malformed saturation falls back to zero without dropping the row",
			row: demand.CharacterCache{
				Character: "Pikachu",
				Window:    "30d",
				Demand:    demandPayload("Pikachu", 0.9, "full"),
				MalformedPayloads: []demand.MalformedPayload{
					{Column: demand.MalformedColumnSaturation, Err: errors.New("unexpected end of JSON input")},
				},
				Velocity: &demand.CharacterVelocity{SampleSize: 5},
			},
			wantMarket: true,
			wantCount:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := demand.NewService(newRepoWithRows([]demand.CharacterCache{tc.row}), uncoveredLookup()).
				WithLogger(mocks.NewCapturingLogger())
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
			if (out[0].Market != nil) != tc.wantMarket {
				t.Fatalf("Market populated = %v, want %v", out[0].Market != nil, tc.wantMarket)
			}
			if !tc.wantMarket {
				return
			}
			if out[0].Market.ActiveListingCount != tc.wantCount {
				t.Errorf("Market.ActiveListingCount = %d, want %d", out[0].Market.ActiveListingCount, tc.wantCount)
			}
		})
	}
}

// --- Standalone scoring tests ---

func TestOpportunityScore_CoverageAndSaturation(t *testing.T) {
	base := demand.OpportunityScore(0.8, nil, 10, demand.NicheCoverage{})
	covered := demand.OpportunityScore(0.8, nil, 10, demand.NicheCoverage{Covered: true, ActiveCampaignIDs: []string{"c1"}})
	saturated := demand.OpportunityScore(0.8, nil, 500, demand.NicheCoverage{})

	if !(base > covered) {
		t.Fatalf("coverage penalty not applied: base=%f covered=%f", base, covered)
	}
	if !(base > saturated) {
		t.Fatalf("saturation penalty not applied: base=%f saturated=%f", base, saturated)
	}
}

// TestService_Leaderboard_MalformedPayloads_LogsWarn asserts that each
// malformed-payload column the service knows how to report produces exactly
// one Warn naming the character, and that the near-miss row alongside it
// produces none. The near misses are the point: a malformed "demand" entry on
// a row with a non-nil Demand must stay quiet (the demand warn only fires when
// row.Demand is actually nil), and an ordinary row with a legitimately absent
// demand payload must stay quiet too. Getting that distinction wrong either
// spams the log for every routine nil-demand row or silently drops a character
// from the leaderboard with no signal at all (SLA-41 item 1).
func TestService_Leaderboard_MalformedPayloads_LogsWarn(t *testing.T) {
	tests := []struct {
		name        string
		rows        []demand.CharacterCache
		wantMessage string
		wantChar    string
	}{
		{
			name: "velocity decode failure warns; malformed demand beside a live payload does not",
			rows: []demand.CharacterCache{
				{
					Character: "Pikachu",
					Window:    "7d",
					Demand: &demand.CharacterDemand{
						CharacterName:  "Pikachu",
						CardCount:      5,
						AvgDemandScore: 0.8,
					},
					MalformedPayloads: []demand.MalformedPayload{
						{Column: demand.MalformedColumnVelocity, Err: errors.New("unexpected end of JSON input")},
					},
				},
				{
					Character: "Charizard",
					Window:    "7d",
					Demand: &demand.CharacterDemand{
						CharacterName:  "Charizard",
						CardCount:      3,
						AvgDemandScore: 0.6,
					},
					MalformedPayloads: []demand.MalformedPayload{
						{Column: demand.MalformedColumnDemand, Err: errors.New("some demand decode error")},
					},
				},
			},
			wantMessage: "velocity_json unmarshal failed",
			wantChar:    "Pikachu",
		},
		{
			name: "demand decode failure warns; a plain absent payload does not",
			rows: []demand.CharacterCache{
				{
					Character: "Pikachu",
					Window:    "7d",
					Demand:    nil,
					MalformedPayloads: []demand.MalformedPayload{
						{Column: demand.MalformedColumnDemand, Err: errors.New("unexpected end of JSON input")},
					},
				},
				{
					Character: "Charizard",
					Window:    "7d",
					Demand:    nil,
				},
			},
			wantMessage: "demand_json unmarshal failed",
			wantChar:    "Pikachu",
		},
		{
			// Saturation failing to decode used to be swallowed entirely: the
			// row still yields a NicheMarket, just with ActiveListingCount
			// stuck at 0, which the saturation penalty reads as "no
			// competition". The fallback stays (see
			// TestService_Leaderboard_ActiveListingCount) but must be
			// attributable.
			name: "saturation decode failure warns",
			rows: []demand.CharacterCache{
				{
					Character: "Pikachu",
					Window:    "7d",
					Demand: &demand.CharacterDemand{
						CharacterName:  "Pikachu",
						CardCount:      5,
						AvgDemandScore: 0.8,
					},
					MalformedPayloads: []demand.MalformedPayload{
						{Column: demand.MalformedColumnSaturation, Err: errors.New("unexpected end of JSON input")},
					},
				},
			},
			wantMessage: "saturation_json unmarshal failed",
			wantChar:    "Pikachu",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := mocks.NewCapturingLogger()
			svc := demand.NewService(newRepoWithRows(tc.rows), uncoveredLookup()).WithLogger(logger)

			if _, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{Window: "7d"}); err != nil {
				t.Fatalf("Leaderboard: unexpected error: %v", err)
			}

			var warns []mocks.LogEntry
			for _, e := range logger.Entries() {
				if e.Level == "warn" {
					warns = append(warns, e)
				}
			}
			if len(warns) != 1 {
				t.Fatalf("got %d warn entries, want 1: %+v", len(warns), warns)
			}

			entry := warns[0]
			if entry.Message != tc.wantMessage {
				t.Errorf("Message = %q, want %q", entry.Message, tc.wantMessage)
			}
			character, ok := entry.FindField("character")
			if !ok {
				t.Fatal("warn entry has no \"character\" field")
			}
			if character != tc.wantChar {
				t.Errorf("character field = %v, want %q", character, tc.wantChar)
			}
		})
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
