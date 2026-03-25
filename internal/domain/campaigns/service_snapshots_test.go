package campaigns

import (
	"math"
	"testing"
)

func TestApplyCLCorrection(t *testing.T) {
	tests := []struct {
		name         string
		snapshot     *MarketSnapshot
		clValueCents int
		// Expected fields after correction
		wantMedian          int
		wantGradePriceCents int
		wantConservative    int
		wantOptimistic      int
		wantP10             int
		wantP90             int
		wantCLValueCents    int
		wantCLDeviationPct  float64
		wantCLAnchorApplied bool
		wantPricingGap      bool
	}{
		{
			name:         "nil snapshot does not panic",
			snapshot:     nil,
			clValueCents: 10000,
		},
		{
			name:             "clValueCents zero leaves snapshot unchanged",
			snapshot:         &MarketSnapshot{MedianCents: 5000},
			clValueCents:     0,
			wantMedian:       5000,
			wantCLValueCents: 0,
		},
		{
			name:             "clValueCents negative leaves snapshot unchanged",
			snapshot:         &MarketSnapshot{MedianCents: 5000},
			clValueCents:     -1,
			wantMedian:       5000,
			wantCLValueCents: 0,
		},
		{
			name:                "pricing gap fills from CL",
			snapshot:            &MarketSnapshot{MedianCents: 0, GradePriceCents: 0, PricingGap: true},
			clValueCents:        10000,
			wantMedian:          10000,
			wantGradePriceCents: 10000,
			wantConservative:    8500,
			wantOptimistic:      11500,
			wantP10:             7000,
			wantP90:             13000,
			wantCLValueCents:    10000,
			wantCLDeviationPct:  1.0,
			wantCLAnchorApplied: true,
			wantPricingGap:      false,
		},
		{
			name:               "low deviation single source no correction",
			snapshot:           &MarketSnapshot{MedianCents: 9000, SourceCount: 1},
			clValueCents:       10000,
			wantMedian:         9000, // unchanged
			wantCLValueCents:   10000,
			wantCLDeviationPct: 0.10,
		},
		{
			name:                "high deviation single source corrects",
			snapshot:            &MarketSnapshot{MedianCents: 5000, SourceCount: 1},
			clValueCents:        10000,
			wantMedian:          10000,
			wantGradePriceCents: 10000,
			wantConservative:    8500,
			wantOptimistic:      11500,
			wantP10:             7000,
			wantP90:             13000,
			wantCLValueCents:    10000,
			wantCLDeviationPct:  0.50,
			wantCLAnchorApplied: true,
		},
		{
			name:               "high deviation single source market above CL trusts market",
			snapshot:           &MarketSnapshot{MedianCents: 5700, SourceCount: 1},
			clValueCents:       900,
			wantMedian:         5700, // NOT corrected — market is above CL
			wantCLValueCents:   900,
			wantCLDeviationPct: 5.333,
		},
		{
			name:               "high deviation multi-source trusts fusion",
			snapshot:           &MarketSnapshot{MedianCents: 5000, SourceCount: 2},
			clValueCents:       10000,
			wantMedian:         5000, // NOT corrected
			wantCLValueCents:   10000,
			wantCLDeviationPct: 0.50,
		},
		{
			name:               "exactly at threshold no correction",
			snapshot:           &MarketSnapshot{MedianCents: 6000, SourceCount: 1},
			clValueCents:       10000,
			wantMedian:         6000, // 40% deviation, threshold is > 0.40
			wantCLValueCents:   10000,
			wantCLDeviationPct: 0.40,
		},
		{
			name:               "CLValueCents always set on snapshot",
			snapshot:           &MarketSnapshot{MedianCents: 9500, SourceCount: 3},
			clValueCents:       10000,
			wantMedian:         9500,
			wantCLValueCents:   10000,
			wantCLDeviationPct: 0.05,
		},
		{
			name:                "no median but GradePriceCents present still anchors",
			snapshot:            &MarketSnapshot{MedianCents: 0, GradePriceCents: 8000, SourceCount: 1},
			clValueCents:        10000,
			wantMedian:          10000,
			wantGradePriceCents: 10000,
			wantConservative:    8500,
			wantOptimistic:      11500,
			wantP10:             7000,
			wantP90:             13000,
			wantCLValueCents:    10000,
			wantCLDeviationPct:  1.0,
			wantCLAnchorApplied: true,
		},
		{
			name:                "CL anchor clears IsEstimated",
			snapshot:            &MarketSnapshot{MedianCents: 5000, SourceCount: 1, IsEstimated: true},
			clValueCents:        10000,
			wantMedian:          10000,
			wantGradePriceCents: 10000,
			wantConservative:    8500,
			wantOptimistic:      11500,
			wantP10:             7000,
			wantP90:             13000,
			wantCLValueCents:    10000,
			wantCLDeviationPct:  0.50,
			wantCLAnchorApplied: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			applyCLCorrection(tc.snapshot, tc.clValueCents)

			if tc.snapshot == nil {
				return // nil test — just verify no panic
			}

			if tc.snapshot.MedianCents != tc.wantMedian {
				t.Errorf("MedianCents = %d, want %d", tc.snapshot.MedianCents, tc.wantMedian)
			}
			if tc.snapshot.GradePriceCents != tc.wantGradePriceCents {
				t.Errorf("GradePriceCents = %d, want %d", tc.snapshot.GradePriceCents, tc.wantGradePriceCents)
			}
			if tc.snapshot.ConservativeCents != tc.wantConservative {
				t.Errorf("ConservativeCents = %d, want %d", tc.snapshot.ConservativeCents, tc.wantConservative)
			}
			if tc.snapshot.OptimisticCents != tc.wantOptimistic {
				t.Errorf("OptimisticCents = %d, want %d", tc.snapshot.OptimisticCents, tc.wantOptimistic)
			}
			if tc.snapshot.P10Cents != tc.wantP10 {
				t.Errorf("P10Cents = %d, want %d", tc.snapshot.P10Cents, tc.wantP10)
			}
			if tc.snapshot.P90Cents != tc.wantP90 {
				t.Errorf("P90Cents = %d, want %d", tc.snapshot.P90Cents, tc.wantP90)
			}
			if tc.snapshot.CLValueCents != tc.wantCLValueCents {
				t.Errorf("CLValueCents = %d, want %d", tc.snapshot.CLValueCents, tc.wantCLValueCents)
			}
			if math.Abs(tc.snapshot.CLDeviationPct-tc.wantCLDeviationPct) > 0.001 {
				t.Errorf("CLDeviationPct = %.4f, want %.4f", tc.snapshot.CLDeviationPct, tc.wantCLDeviationPct)
			}
			if tc.snapshot.CLAnchorApplied != tc.wantCLAnchorApplied {
				t.Errorf("CLAnchorApplied = %v, want %v", tc.snapshot.CLAnchorApplied, tc.wantCLAnchorApplied)
			}
			if tc.wantCLAnchorApplied && tc.snapshot.IsEstimated {
				t.Error("IsEstimated should be false when CLAnchorApplied is true")
			}
			if tc.snapshot.PricingGap != tc.wantPricingGap {
				t.Errorf("PricingGap = %v, want %v", tc.snapshot.PricingGap, tc.wantPricingGap)
			}
		})
	}
}
