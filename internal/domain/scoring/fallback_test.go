package scoring

import (
	"testing"
	"time"
)

func TestFallbackResult(t *testing.T) {
	tests := []struct {
		name           string
		sc             ScoreCard
		wantVerdict    Verdict
		wantAdjNil     bool
		wantInsightNon bool
		wantSignals    int
		wantDirections map[string]string
	}{
		{
			name: "basic purchase with gaps",
			sc: ScoreCard{
				EntityID:   "test-1",
				EntityType: "purchase",
				Factors: []Factor{
					{Name: FactorMarketTrend, Value: 0.65, Confidence: 0.9, Source: "doubleholo"},
					{Name: FactorLiquidity, Value: 0.5, Confidence: 0.8, Source: "ebay"},
					{Name: FactorROIPotential, Value: 0.3, Confidence: 0.7, Source: "campaigns"},
				},
				Composite:  0.45,
				Confidence: 0.75,
				Verdict:    VerdictBuy,
				DataGaps:   []DataGap{{FactorName: FactorScarcity, Reason: "no_population_data"}},
				ScoredAt:   time.Now(),
			},
			wantVerdict:    VerdictBuy,
			wantAdjNil:     true,
			wantInsightNon: true,
			wantSignals:    3,
		},
		{
			name: "direction mapping",
			sc: ScoreCard{
				Factors: []Factor{
					{Name: FactorMarketTrend, Value: 0.5, Confidence: 0.9, Source: "test"},
					{Name: FactorLiquidity, Value: -0.5, Confidence: 0.8, Source: "test"},
					{Name: FactorROIPotential, Value: 0.02, Confidence: 0.7, Source: "test"},
				},
				Composite: 0.1, Confidence: 0.6, Verdict: VerdictLeanBuy, ScoredAt: time.Now(),
			},
			wantVerdict:    VerdictLeanBuy,
			wantAdjNil:     true,
			wantInsightNon: true,
			wantSignals:    3,
			wantDirections: map[string]string{
				FactorMarketTrend:  "bullish",
				FactorLiquidity:    "bearish",
				FactorROIPotential: "neutral",
			},
		},
		{
			name: "empty factors",
			sc: ScoreCard{
				Verdict:  VerdictHold,
				ScoredAt: time.Now(),
			},
			wantVerdict:    VerdictHold,
			wantAdjNil:     true,
			wantInsightNon: true,
			wantSignals:    0,
		},
		{
			name: "single factor",
			sc: ScoreCard{
				Factors:  []Factor{{Name: FactorMarketTrend, Value: -0.8, Confidence: 0.5, Source: "test"}},
				Verdict:  VerdictSell,
				ScoredAt: time.Now(),
			},
			wantVerdict:    VerdictSell,
			wantAdjNil:     true,
			wantInsightNon: true,
			wantSignals:    1,
			wantDirections: map[string]string{FactorMarketTrend: "bearish"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FallbackResult(tt.sc)
			if result.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %s, want %s", result.Verdict, tt.wantVerdict)
			}
			if tt.wantAdjNil && result.AdjustmentReason != nil {
				t.Error("AdjustmentReason should be nil")
			}
			if tt.wantInsightNon && result.KeyInsight == "" {
				t.Error("KeyInsight should not be empty")
			}
			if len(result.Signals) != tt.wantSignals {
				t.Errorf("Signals count = %d, want %d", len(result.Signals), tt.wantSignals)
			}
			for _, sig := range result.Signals {
				if sig.Factor == "" {
					t.Error("signal missing Factor")
				}
				if sig.Direction == "" {
					t.Error("signal missing Direction")
				}
				if sig.Title == "" {
					t.Error("signal missing Title")
				}
			}
			if tt.wantDirections != nil {
				directions := make(map[string]string)
				for _, sig := range result.Signals {
					directions[sig.Factor] = sig.Direction
				}
				for factor, want := range tt.wantDirections {
					if directions[factor] != want {
						t.Errorf("%s direction = %s, want %s", factor, directions[factor], want)
					}
				}
			}
		})
	}
}
