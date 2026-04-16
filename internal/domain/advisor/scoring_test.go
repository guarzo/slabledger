package advisor

import (
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/scoring"
)

func TestBuildScoreCard(t *testing.T) {
	tests := []struct {
		name         string
		entityID     string
		entityType   string
		data         any
		profile      scoring.WeightProfile
		minFactors   int
		exactFactors int // if > 0, check exact count
		maxGaps      int
		exactGaps    int // if >= 0, check exact count; use -1 to skip
		wantErr      bool
		wantErrType  error
	}{
		{
			name:       "campaign with partial data",
			entityID:   "camp-1",
			entityType: "campaign",
			data: &CampaignFactorData{
				ROIPct:          ptrFloat(15.0),
				SellThroughPct:  ptrFloat(60.0),
				Trend30dPct:     ptrFloat(5.0),
				PriceConfidence: 0.8,
				MarketSource:    "test",
			},
			profile:    scoring.CampaignAnalysisProfile,
			minFactors: 3,
			exactGaps:  -1,
		},
		{
			name:       "liquidation with data",
			entityID:   "purch-1",
			entityType: "inventory_item",
			data: &LiquidationFactorData{
				DaysHeld:        90,
				WeeksToCover:    ptrFloat(8.0),
				PriceChangePct:  ptrFloat(-5.0),
				SalesPerMonth:   ptrFloat(3.0),
				PriceConfidence: 0.7,
				MarketSource:    "test",
			},
			profile:    scoring.LiquidationProfile,
			minFactors: 4,
			exactGaps:  -1,
		},
		{
			name:       "suggestion with full data",
			entityID:   "suggestion-1",
			entityType: "suggestion",
			data: &SuggestionFactorData{
				ProjectedROIPct: ptrFloat(25.0),
				FillsGap:        true,
				OverlapCount:    0,
				Trend30dPct:     ptrFloat(8.0),
				SalesPerMonth:   ptrFloat(12.0),
				PriceChangePct:  ptrFloat(10.0),
				PriceConfidence: 0.85,
				MarketSource:    "doubleholo",
			},
			profile:      scoring.CampaignSuggestionsProfile,
			exactFactors: 5,
			exactGaps:    0,
		},
		{
			name:       "unsupported data type",
			entityID:   "bad-1",
			entityType: "unknown",
			data:       "not a valid type",
			profile:    scoring.CampaignAnalysisProfile,
			wantErr:    true,
			exactGaps:  -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := BuildScoreCard(tt.entityID, tt.entityType, tt.data, tt.profile)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErrType != nil {
					var insuffErr *scoring.ErrInsufficientData
					if !errors.As(err, &insuffErr) {
						t.Fatalf("expected *ErrInsufficientData, got %T: %v", err, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildScoreCard: %v", err)
			}
			if sc.EntityID != tt.entityID {
				t.Errorf("EntityID = %s, want %s", sc.EntityID, tt.entityID)
			}
			if tt.exactFactors > 0 && len(sc.Factors) != tt.exactFactors {
				t.Errorf("Factors count = %d, want %d", len(sc.Factors), tt.exactFactors)
			}
			if tt.minFactors > 0 && len(sc.Factors) < tt.minFactors {
				t.Errorf("Factors count = %d, want >= %d", len(sc.Factors), tt.minFactors)
			}
			if tt.maxGaps > 0 && len(sc.DataGaps) > tt.maxGaps {
				t.Errorf("too many gaps: %d", len(sc.DataGaps))
			}
			if tt.exactGaps >= 0 && len(sc.DataGaps) != tt.exactGaps {
				t.Errorf("DataGaps count = %d, want %d", len(sc.DataGaps), tt.exactGaps)
			}
		})
	}
}

func ptrFloat(f float64) *float64 { return &f }
