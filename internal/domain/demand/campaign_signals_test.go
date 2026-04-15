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

// activeCampaignsStub is a CampaignCoverageLookup test double that returns a
// fixed list of active campaigns. The other methods are no-ops.
type activeCampaignsStub struct {
	campaigns []demand.ActiveCampaign
}

func (s activeCampaignsStub) CampaignsCovering(ctx context.Context, character, era string, grade int) ([]int64, error) {
	return nil, nil
}
func (s activeCampaignsStub) UnsoldCountFor(ctx context.Context, character, era string, grade int) (int, error) {
	return 0, nil
}
func (s activeCampaignsStub) ActiveCampaigns(ctx context.Context) ([]demand.ActiveCampaign, error) {
	return s.campaigns, nil
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
			wantQual:  demand.DataQualityEmpty,
		},
		{
			name: "inclusion list campaign one accelerator",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
				charRow("Charizard", 8, 2.0, 52, computed),  // below accel threshold
				charRow("Umbreon", 21, -8.3, 18, computed),  // decelerating
			},
			campaigns: []demand.ActiveCampaign{{ID: 1, Name: "Vintage Core", InclusionList: "Charizard,Pikachu,Umbreon", GradeRange: "9-10"}},
			wantSigs:  1,
			wantTop:   "Pikachu",
			wantQual:  demand.DataQualityFull,
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
			wantQual:  demand.DataQualityFull,
		},
		{
			name: "inclusion list with no cache overlap produces no signal",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
			},
			campaigns: []demand.ActiveCampaign{{ID: 7, Name: "Crystal", InclusionList: "Kingdra,Kabutops"}},
			wantSigs:  0,
			wantQual:  demand.DataQualityEmpty,
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
			wantQual:  demand.DataQualityFull,
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
			wantQual:  demand.DataQualityFull,
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
			svc := demand.NewService(repo, activeCampaignsStub{campaigns: tc.campaigns})

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
