package scoring

import (
	"testing"
	"time"
)

func TestFallbackResult(t *testing.T) {
	sc := ScoreCard{
		EntityID:   "test-1",
		EntityType: "purchase",
		Factors: []Factor{
			{Name: FactorMarketTrend, Value: 0.65, Confidence: 0.9, Source: "pricecharting"},
			{Name: FactorLiquidity, Value: 0.5, Confidence: 0.8, Source: "ebay"},
			{Name: FactorROIPotential, Value: 0.3, Confidence: 0.7, Source: "campaigns"},
		},
		Composite:  0.45,
		Confidence: 0.75,
		Verdict:    VerdictBuy,
		DataGaps:   []DataGap{{FactorName: FactorScarcity, Reason: "no_population_data"}},
		ScoredAt:   time.Now(),
	}
	result := FallbackResult(sc)
	if result.Verdict != sc.Verdict {
		t.Errorf("Verdict = %s, want %s", result.Verdict, sc.Verdict)
	}
	if result.AdjustmentReason != nil {
		t.Error("AdjustmentReason should be nil")
	}
	if result.KeyInsight == "" {
		t.Error("KeyInsight should not be empty")
	}
	if len(result.Signals) != len(sc.Factors) {
		t.Errorf("Signals count = %d, want %d", len(result.Signals), len(sc.Factors))
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
}

func TestFallbackResult_DirectionMapping(t *testing.T) {
	sc := ScoreCard{
		Factors: []Factor{
			{Name: FactorMarketTrend, Value: 0.5, Confidence: 0.9, Source: "test"},
			{Name: FactorLiquidity, Value: -0.5, Confidence: 0.8, Source: "test"},
			{Name: FactorROIPotential, Value: 0.02, Confidence: 0.7, Source: "test"},
		},
		Composite: 0.1, Confidence: 0.6, Verdict: VerdictLeanBuy, ScoredAt: time.Now(),
	}
	result := FallbackResult(sc)
	directions := make(map[string]string)
	for _, sig := range result.Signals {
		directions[sig.Factor] = sig.Direction
	}
	if directions[FactorMarketTrend] != "bullish" {
		t.Errorf("market_trend direction = %s, want bullish", directions[FactorMarketTrend])
	}
	if directions[FactorLiquidity] != "bearish" {
		t.Errorf("liquidity direction = %s, want bearish", directions[FactorLiquidity])
	}
	if directions[FactorROIPotential] != "neutral" {
		t.Errorf("roi_potential direction = %s, want neutral", directions[FactorROIPotential])
	}
}
