package demand_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// velocityJSON builds a velocity_json blob mirroring DH's wire format: median
// and avg days come back as strings; velocity_change_pct is a float.
func velocityJSON(medianDays, vChangePct float64, sample int) string {
	return `{
		"median_days_to_sell": "` + strconv.FormatFloat(medianDays, 'f', -1, 64) + `",
		"avg_days_to_sell": "` + strconv.FormatFloat(medianDays, 'f', -1, 64) + `",
		"sell_through": {},
		"sample_size": ` + strconv.Itoa(sample) + `,
		"velocity_change_pct": ` + strconv.FormatFloat(vChangePct, 'f', -1, 64) + `
	}`
}

// velocityJSONNoChange omits velocity_change_pct — used to verify the service
// excludes characters with no change metric from contributors.
func velocityJSONNoChange() string {
	return `{"median_days_to_sell": "10", "avg_days_to_sell": "10", "sell_through": {}, "sample_size": 5}`
}

func charRow(name string, medianDays, vChangePct float64, sample int, computed time.Time) demand.CharacterCache {
	vj := velocityJSON(medianDays, vChangePct, sample)
	return demand.CharacterCache{
		Character:           name,
		Window:              "30d",
		VelocityJSON:        &vj,
		AnalyticsComputedAt: &computed,
	}
}

func charRowNoChange(name string, computed time.Time) demand.CharacterCache {
	vj := velocityJSONNoChange()
	return demand.CharacterCache{
		Character:           name,
		Window:              "30d",
		VelocityJSON:        &vj,
		AnalyticsComputedAt: &computed,
	}
}

// campaignLookupWith builds a CampaignCoverageLookupMock whose ActiveCampaigns
// returns the given list. Leaves the other Fn fields nil so the defaults
// (empty coverage, zero unsold) apply — CampaignSignals only reads
// ActiveCampaigns.
func campaignLookupWith(campaigns []demand.ActiveCampaign) *mocks.CampaignCoverageLookupMock {
	return &mocks.CampaignCoverageLookupMock{
		ActiveCampaignsFn: func(ctx context.Context) ([]demand.ActiveCampaign, error) {
			return campaigns, nil
		},
	}
}

func TestCampaignSignals(t *testing.T) {
	computed := time.Date(2026, 4, 15, 3, 15, 0, 0, time.UTC)

	tests := []struct {
		name      string
		rows      []demand.CharacterCache
		campaigns []demand.ActiveCampaign
		wantSigs  int
		wantTop   string // first signal's top accelerator name (empty = skip check)
		wantQual  string
	}{
		{
			name:      "empty cache",
			rows:      nil,
			campaigns: []demand.ActiveCampaign{{ID: 1, Name: "Modern", InclusionList: ""}},
			wantSigs:  0,
			wantQual:  demand.QualityEmpty,
		},
		{
			name: "inclusion list campaign one accelerator",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
				charRow("Charizard", 8, 2.0, 52, computed), // below accel threshold
				charRow("Umbreon", 21, -8.3, 18, computed), // decelerating
			},
			campaigns: []demand.ActiveCampaign{{ID: 1, Name: "Vintage Core", InclusionList: "Charizard,Pikachu,Umbreon", GradeRange: "9-10"}},
			wantSigs:  1,
			wantTop:   "Pikachu",
			wantQual:  demand.QualityFull,
		},
		{
			name: "open net campaign matches all cached characters",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
				charRow("Gengar", 12, 10.0, 20, computed),
			},
			campaigns: []demand.ActiveCampaign{{ID: 4, Name: "Modern", InclusionList: ""}},
			wantSigs:  1,
			wantTop:   "Pikachu",
			wantQual:  demand.QualityFull,
		},
		{
			name: "inclusion list with no cache overlap produces no signal",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
			},
			campaigns: []demand.ActiveCampaign{{ID: 7, Name: "Crystal", InclusionList: "Kingdra,Kabutops"}},
			wantSigs:  0,
			wantQual:  demand.QualityEmpty,
		},
		{
			name: "character with null velocity_change_pct is excluded from contributors",
			rows: []demand.CharacterCache{
				charRowNoChange("Pikachu", computed),
				charRow("Charizard", 8, 15.7, 52, computed),
			},
			campaigns: []demand.ActiveCampaign{{ID: 1, Name: "Vintage Core", InclusionList: "Pikachu,Charizard"}},
			wantSigs:  1,
			wantTop:   "Charizard",
			wantQual:  demand.QualityFull,
		},
		{
			name: "top accelerating list capped at 5",
			rows: []demand.CharacterCache{
				charRow("A", 5, 30.0, 20, computed),
				charRow("B", 5, 28.0, 20, computed),
				charRow("C", 5, 26.0, 20, computed),
				charRow("D", 5, 24.0, 20, computed),
				charRow("E", 5, 22.0, 20, computed),
				charRow("F", 5, 20.0, 20, computed),
				charRow("G", 5, 18.0, 20, computed),
			},
			campaigns: []demand.ActiveCampaign{{ID: 4, Name: "Modern", InclusionList: ""}},
			wantSigs:  1,
			wantTop:   "A",
			wantQual:  demand.QualityFull,
		},
		{
			name: "exclusion mode excludes listed characters",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
				charRow("Charizard", 8, 15.7, 52, computed),
				charRow("Gengar", 12, 10.0, 20, computed),
			},
			campaigns: []demand.ActiveCampaign{{
				ID:            5,
				Name:          "No Pikachu",
				InclusionList: "Pikachu",
				ExclusionMode: true,
			}},
			wantSigs: 1,
			wantTop:  "Charizard",
			wantQual: demand.QualityFull,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows := tc.rows
			repo := &mocks.DemandRepositoryMock{
				ListCharacterCacheFn: func(ctx context.Context, window string) ([]demand.CharacterCache, error) {
					return rows, nil
				},
			}
			svc := demand.NewService(repo, campaignLookupWith(tc.campaigns))

			resp, err := svc.CampaignSignals(context.Background())
			if err != nil {
				t.Fatalf("CampaignSignals: %v", err)
			}

			if len(resp.Signals) != tc.wantSigs {
				t.Fatalf("want %d signals, got %d: %+v", tc.wantSigs, len(resp.Signals), resp.Signals)
			}
			if resp.DataQuality != tc.wantQual {
				t.Errorf("want quality=%s, got %s", tc.wantQual, resp.DataQuality)
			}
			if tc.wantTop != "" {
				if len(resp.Signals[0].TopAccelerating) == 0 {
					t.Fatalf("want top accelerating, got none")
				}
				if resp.Signals[0].TopAccelerating[0].Character != tc.wantTop {
					t.Errorf("want top=%s, got %s", tc.wantTop, resp.Signals[0].TopAccelerating[0].Character)
				}
				if len(resp.Signals[0].TopAccelerating) > demand.TopContributorsLimit {
					t.Errorf("top list exceeds cap: %d", len(resp.Signals[0].TopAccelerating))
				}
			}
		})
	}
}

// TestCampaignSignals_MedianVelocity verifies the median calculation via
// the public CampaignSignals surface. Even count: average of two middles.
// Odd count: middle element.
func TestCampaignSignals_MedianVelocity(t *testing.T) {
	computed := time.Date(2026, 4, 15, 3, 15, 0, 0, time.UTC)

	tests := []struct {
		name       string
		vChanges   []float64 // velocity_change_pct for each character
		wantMedian float64
	}{
		{
			// Odd count: sorted [10, 20, 30] → middle = 20
			name:       "odd count uses middle element",
			vChanges:   []float64{30.0, 10.0, 20.0},
			wantMedian: 20.0,
		},
		{
			// Even count: sorted [10, 20] → (10+20)/2 = 15
			name:       "even count averages two middles",
			vChanges:   []float64{20.0, 10.0},
			wantMedian: 15.0,
		},
		{
			// Open net case: 22.1 and 10.0 → (10.0+22.1)/2 = 16.05
			name:       "open net two characters",
			vChanges:   []float64{22.1, 10.0},
			wantMedian: 16.05,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rows []demand.CharacterCache
			for i, v := range tc.vChanges {
				rows = append(rows, charRow("Char"+strconv.Itoa(i), 10, v, 10, computed))
			}
			repo := &mocks.DemandRepositoryMock{
				ListCharacterCacheFn: func(ctx context.Context, window string) ([]demand.CharacterCache, error) {
					return rows, nil
				},
			}
			campaign := demand.ActiveCampaign{ID: 1, Name: "Test", InclusionList: ""}
			svc := demand.NewService(repo, campaignLookupWith([]demand.ActiveCampaign{campaign}))

			resp, err := svc.CampaignSignals(context.Background())
			if err != nil {
				t.Fatalf("CampaignSignals: %v", err)
			}
			if len(resp.Signals) != 1 {
				t.Fatalf("want 1 signal, got %d", len(resp.Signals))
			}
			got := resp.Signals[0].MedianVelocityChangePct
			if got != tc.wantMedian {
				t.Errorf("want median=%v, got %v", tc.wantMedian, got)
			}
		})
	}
}
