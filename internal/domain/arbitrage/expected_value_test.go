package arbitrage

import "testing"

func TestComputeExpectedValue(t *testing.T) {
	ev := computeExpectedValue(EVInput{
		CardName:               "Charizard",
		CertNumber:             "12345",
		Grade:                  9,
		CostBasis:              8500,
		SegmentSellThrough:     0.72,
		SegmentMedianMarginPct: 0.15,
		LiquidityFactor:        1.0,
		TrendAdjustment:        0.0,
		AvgDaysUnsold:          45.0,
		AnnualCapitalCostRate:  0.05,
		DataPoints:             25,
	})

	if ev.EVCents <= 0 {
		t.Errorf("expected positive EV for profitable segment, got %d", ev.EVCents)
	}
	if ev.Confidence != "high" {
		t.Errorf("expected high confidence with 25 data points, got %s", ev.Confidence)
	}
	if ev.SellProbability < 0.5 || ev.SellProbability > 1.0 {
		t.Errorf("expected sell probability in (0.5, 1.0), got %f", ev.SellProbability)
	}
}

func TestComputeExpectedValue_Negative(t *testing.T) {
	ev := computeExpectedValue(EVInput{
		CardName:               "Pikachu",
		CertNumber:             "67890",
		Grade:                  8,
		CostBasis:              10000,
		SegmentSellThrough:     0.20,
		SegmentMedianMarginPct: -0.10,
		LiquidityFactor:        0.5,
		TrendAdjustment:        -0.05,
		AvgDaysUnsold:          90.0,
		AnnualCapitalCostRate:  0.05,
		DataPoints:             3,
	})

	if ev.EVCents >= 0 {
		t.Errorf("expected negative EV for unprofitable segment, got %d", ev.EVCents)
	}
	if ev.Confidence != "low" {
		t.Errorf("expected low confidence with 3 data points, got %s", ev.Confidence)
	}
}
