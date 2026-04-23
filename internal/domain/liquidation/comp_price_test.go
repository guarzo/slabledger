package liquidation

import (
	"testing"
	"time"
)

func dateStr(daysAgo int) string {
	return time.Now().AddDate(0, 0, -daysAgo).Format("2006-01-02")
}

func TestComputeCompPrice(t *testing.T) {
	tests := []struct {
		name           string
		comps          []SaleComp
		clValueCents   int
		wantPriceCents int
		wantConfidence ConfidenceLevel
	}{
		{
			name:           "no comps returns zero",
			comps:          nil,
			clValueCents:   10000,
			wantPriceCents: 0,
			wantConfidence: ConfidenceNone,
		},
		{
			name:           "single recent comp",
			comps:          []SaleComp{{SaleDate: dateStr(5), PriceCents: 10000}},
			clValueCents:   10000,
			wantPriceCents: 10000,
			wantConfidence: ConfidenceLow,
		},
		{
			name: "5 comps trims outliers",
			comps: []SaleComp{
				{SaleDate: dateStr(5), PriceCents: 10000},
				{SaleDate: dateStr(6), PriceCents: 10200},
				{SaleDate: dateStr(7), PriceCents: 10100},
				{SaleDate: dateStr(8), PriceCents: 50000},
				{SaleDate: dateStr(9), PriceCents: 1000},
			},
			clValueCents:   10000,
			wantPriceCents: 10100, // mean of 10000, 10100, 10200
			wantConfidence: ConfidenceMedium,
		},
		{
			name: "10 recent comps with close CL = high confidence",
			comps: func() []SaleComp {
				comps := make([]SaleComp, 10)
				for i := 0; i < 10; i++ {
					comps[i] = SaleComp{SaleDate: dateStr(i + 1), PriceCents: 10000 + i*100}
				}
				return comps
			}(),
			clValueCents:   10500,
			wantPriceCents: 10450, // trimmed mean of middle 8: 10100..10800 => mean=10450
			wantConfidence: ConfidenceHigh,
		},
		{
			name: "old comps within 180 day expansion",
			comps: []SaleComp{
				{SaleDate: dateStr(100), PriceCents: 8000},
				{SaleDate: dateStr(110), PriceCents: 8500},
			},
			clValueCents:   10000,
			wantPriceCents: 8250,
			wantConfidence: ConfidenceLow,
		},
		{
			name: "large CL gap = lower confidence",
			comps: func() []SaleComp {
				comps := make([]SaleComp, 10)
				for i := 0; i < 10; i++ {
					comps[i] = SaleComp{SaleDate: dateStr(i + 1), PriceCents: 5000}
				}
				return comps
			}(),
			clValueCents:   10000,
			wantPriceCents: 5000,
			wantConfidence: ConfidenceMedium, // gap >25% lowers from high to medium
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeCompPrice(tt.comps, tt.clValueCents)
			if result.CompPriceCents != tt.wantPriceCents {
				t.Errorf("CompPriceCents = %d, want %d", result.CompPriceCents, tt.wantPriceCents)
			}
			if result.ConfidenceLevel != tt.wantConfidence {
				t.Errorf("ConfidenceLevel = %q, want %q", result.ConfidenceLevel, tt.wantConfidence)
			}
		})
	}
}
