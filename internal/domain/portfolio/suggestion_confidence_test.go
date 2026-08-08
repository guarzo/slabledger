package portfolio

import "testing"

func TestConfidenceLabelWithAge(t *testing.T) {
	tests := []struct {
		name           string
		n              int
		latestSaleDate string
		now            string
		want           string
	}{
		{"high stays high with recent data", 25, "2026-02-01", "2026-03-01", "high"},
		{"high decays to medium with old data", 25, "2025-06-01", "2026-03-01", "medium"},
		{"medium decays to low with old data", 10, "2025-06-01", "2026-03-01", "low"},
		{"low stays low with old data", 3, "2025-06-01", "2026-03-01", "low"},
		{"empty date uses base confidence", 25, "", "2026-03-01", "high"},
		{"exactly 6 months stays same", 25, "2025-09-01", "2026-03-01", "high"},
		{"7 months decays", 25, "2025-08-01", "2026-03-01", "medium"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := confidenceLabelWithAge(tc.n, tc.latestSaleDate, tc.now)
			if got != tc.want {
				t.Errorf("confidenceLabelWithAge(%d, %q, %q) = %q, want %q",
					tc.n, tc.latestSaleDate, tc.now, got, tc.want)
			}
		})
	}
}
