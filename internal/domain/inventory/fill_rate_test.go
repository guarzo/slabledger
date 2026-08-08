package inventory

import "testing"

func TestFillRatePct(t *testing.T) {
	tests := []struct {
		name       string
		spendCents int
		capCents   int
		want       float64
	}{
		{name: "half the cap", spendCents: 500, capCents: 1000, want: 0.5},
		{name: "exactly at cap", spendCents: 1000, capCents: 1000, want: 1.0},
		{name: "over cap", spendCents: 1500, capCents: 1000, want: 1.5},
		{name: "zero spend", spendCents: 0, capCents: 1000, want: 0},
		{name: "zero cap yields zero, not a division by zero", spendCents: 500, capCents: 0, want: 0},
		{name: "negative cap yields zero", spendCents: 500, capCents: -100, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FillRatePct(tt.spendCents, tt.capCents); got != tt.want {
				t.Errorf("FillRatePct(%d, %d) = %v, want %v", tt.spendCents, tt.capCents, got, tt.want)
			}
		})
	}
}

func TestEnrichDailySpendFillRate(t *testing.T) {
	tests := []struct {
		name     string
		daily    []DailySpend
		capCents int
		want     []DailySpend
	}{
		{
			name:     "populates cap and rate on every row",
			daily:    []DailySpend{{Date: "2025-01-01", SpendCents: 500}, {Date: "2025-01-02", SpendCents: 250}},
			capCents: 1000,
			want: []DailySpend{
				{Date: "2025-01-01", SpendCents: 500, CapCents: 1000, FillRatePct: 0.5},
				{Date: "2025-01-02", SpendCents: 250, CapCents: 1000, FillRatePct: 0.25},
			},
		},
		{
			name:     "zero cap leaves rate at zero but still records the cap",
			daily:    []DailySpend{{Date: "2025-01-01", SpendCents: 500}},
			capCents: 0,
			want:     []DailySpend{{Date: "2025-01-01", SpendCents: 500, CapCents: 0, FillRatePct: 0}},
		},
		{
			name:     "empty slice is a no-op",
			daily:    []DailySpend{},
			capCents: 1000,
			want:     []DailySpend{},
		},
		{
			name:     "nil slice is a no-op",
			daily:    nil,
			capCents: 1000,
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			EnrichDailySpendFillRate(tt.daily, tt.capCents)
			if len(tt.daily) != len(tt.want) {
				t.Fatalf("length = %d, want %d", len(tt.daily), len(tt.want))
			}
			for i := range tt.want {
				if tt.daily[i] != tt.want[i] {
					t.Errorf("row %d = %+v, want %+v", i, tt.daily[i], tt.want[i])
				}
			}
		})
	}
}
