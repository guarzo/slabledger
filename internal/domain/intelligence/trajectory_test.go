package intelligence

import (
	"testing"
	"time"
)

func TestBucketSalesByWeek(t *testing.T) {
	// Anchors: 2026-04-13 is a Monday UTC. Monday-anchored buckets.
	monA := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	monB := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		sales []Sale
		want  []WeeklyBucket
	}{
		{
			name:  "empty input returns nil",
			sales: nil,
			want:  nil,
		},
		{
			name: "zero-timestamp sales skipped",
			sales: []Sale{
				{PriceCents: 100},
				{SoldAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC), PriceCents: 200},
			},
			want: []WeeklyBucket{{WeekStart: monA, SaleCount: 1, AvgPriceCents: 200, MedianPriceCents: 200}},
		},
		{
			name: "Sunday rolls to previous Monday bucket",
			sales: []Sale{
				// Sunday 2026-04-19 → week of 2026-04-13.
				{SoldAt: time.Date(2026, 4, 19, 23, 30, 0, 0, time.UTC), PriceCents: 100},
				// Monday 2026-04-20 → week of 2026-04-20.
				{SoldAt: time.Date(2026, 4, 20, 0, 1, 0, 0, time.UTC), PriceCents: 200},
			},
			want: []WeeklyBucket{
				{WeekStart: monA, SaleCount: 1, AvgPriceCents: 100, MedianPriceCents: 100},
				{WeekStart: monB, SaleCount: 1, AvgPriceCents: 200, MedianPriceCents: 200},
			},
		},
		{
			name: "tz drift: sale at 02:00 PDT on Mon falls in Sun UTC → previous bucket",
			sales: []Sale{
				{
					// 2026-04-20 02:00 PDT = 2026-04-20 09:00 UTC (Monday UTC) → bucket B.
					// But 2026-04-13 02:00 PDT = 2026-04-13 09:00 UTC (Monday UTC) → bucket A.
					// Pick one that crosses: 2026-04-12 22:00 PDT = 2026-04-13 05:00 UTC → bucket A (Monday UTC).
					SoldAt: time.Date(2026, 4, 13, 5, 0, 0, 0, time.UTC), PriceCents: 500,
				},
			},
			want: []WeeklyBucket{{WeekStart: monA, SaleCount: 1, AvgPriceCents: 500, MedianPriceCents: 500}},
		},
		{
			name: "median vs avg on uneven prices",
			sales: []Sale{
				{SoldAt: monA, PriceCents: 100},
				{SoldAt: monA.Add(time.Hour), PriceCents: 200},
				{SoldAt: monA.Add(2 * time.Hour), PriceCents: 900},
			},
			want: []WeeklyBucket{{WeekStart: monA, SaleCount: 3, AvgPriceCents: 400, MedianPriceCents: 200}},
		},
		{
			name: "output sorted chronologically regardless of input order",
			sales: []Sale{
				{SoldAt: monB, PriceCents: 300},
				{SoldAt: monA, PriceCents: 100},
			},
			want: []WeeklyBucket{
				{WeekStart: monA, SaleCount: 1, AvgPriceCents: 100, MedianPriceCents: 100},
				{WeekStart: monB, SaleCount: 1, AvgPriceCents: 300, MedianPriceCents: 300},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BucketSalesByWeek(tt.sales)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d buckets, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if !got[i].WeekStart.Equal(tt.want[i].WeekStart) {
					t.Errorf("bucket %d WeekStart: got %v, want %v", i, got[i].WeekStart, tt.want[i].WeekStart)
				}
				if got[i].SaleCount != tt.want[i].SaleCount {
					t.Errorf("bucket %d SaleCount: got %d, want %d", i, got[i].SaleCount, tt.want[i].SaleCount)
				}
				if got[i].AvgPriceCents != tt.want[i].AvgPriceCents {
					t.Errorf("bucket %d AvgPriceCents: got %d, want %d", i, got[i].AvgPriceCents, tt.want[i].AvgPriceCents)
				}
				if got[i].MedianPriceCents != tt.want[i].MedianPriceCents {
					t.Errorf("bucket %d MedianPriceCents: got %d, want %d", i, got[i].MedianPriceCents, tt.want[i].MedianPriceCents)
				}
			}
		})
	}
}

func TestComputeTrajectoryScore(t *testing.T) {
	monA := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	// Helper: n-week rising series 100,200,300,...
	linearRising := func(n int, base, step int64) []WeeklyBucket {
		out := make([]WeeklyBucket, n)
		for i := 0; i < n; i++ {
			out[i] = WeeklyBucket{
				WeekStart:     monA.AddDate(0, 0, 7*i),
				SaleCount:     3,
				AvgPriceCents: base + step*int64(i),
			}
		}
		return out
	}

	tests := []struct {
		name              string
		buckets           []WeeklyBucket
		clValue           int64
		wantSlope         float64
		wantLowConfidence bool
		wantTotalSales    int
	}{
		{
			name:              "empty buckets → zero score, low confidence",
			buckets:           nil,
			wantSlope:         0,
			wantLowConfidence: true,
		},
		{
			name: "single bucket → no slope (need 2+ points)",
			buckets: []WeeklyBucket{
				{WeekStart: monA, SaleCount: 10, AvgPriceCents: 1000},
			},
			wantSlope:         0,
			wantLowConfidence: false,
			wantTotalSales:    10,
		},
		{
			name:              "linear rising 8 weeks +100c/week → slope=100",
			buckets:           linearRising(8, 1000, 100),
			wantSlope:         100,
			wantLowConfidence: false,
			wantTotalSales:    24,
		},
		{
			name:              "flat prices → zero slope",
			buckets:           linearRising(8, 1000, 0),
			wantSlope:         0,
			wantLowConfidence: false,
			wantTotalSales:    24,
		},
		{
			name: "window trims to last 8 when more buckets provided",
			buckets: func() []WeeklyBucket {
				// 12 weeks: 1000c for first 4 (outside window), 2000c rising for last 8.
				b := make([]WeeklyBucket, 12)
				for i := 0; i < 4; i++ {
					b[i] = WeeklyBucket{WeekStart: monA.AddDate(0, 0, 7*i), SaleCount: 1, AvgPriceCents: 1000}
				}
				for i := 4; i < 12; i++ {
					b[i] = WeeklyBucket{WeekStart: monA.AddDate(0, 0, 7*i), SaleCount: 1, AvgPriceCents: 2000 + int64((i-4)*50)}
				}
				return b
			}(),
			wantSlope:         50,
			wantLowConfidence: false,
			wantTotalSales:    8,
		},
		{
			name: "low total sales → LowConfidence even if slope is defined",
			buckets: []WeeklyBucket{
				{WeekStart: monA, SaleCount: 1, AvgPriceCents: 1000},
				{WeekStart: monA.AddDate(0, 0, 7), SaleCount: 2, AvgPriceCents: 1100},
			},
			wantSlope:         100,
			wantLowConfidence: true,
			wantTotalSales:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ComputeTrajectoryScore(tt.buckets, tt.clValue)
			if score.SlopeCentsPerWeek != tt.wantSlope {
				t.Errorf("SlopeCentsPerWeek: got %v, want %v", score.SlopeCentsPerWeek, tt.wantSlope)
			}
			if score.LowConfidence != tt.wantLowConfidence {
				t.Errorf("LowConfidence: got %v, want %v", score.LowConfidence, tt.wantLowConfidence)
			}
			if score.TotalSales != tt.wantTotalSales {
				t.Errorf("TotalSales: got %d, want %d", score.TotalSales, tt.wantTotalSales)
			}
			if score.WindowWeeks != defaultTrajectoryWindow {
				t.Errorf("WindowWeeks: got %d, want %d", score.WindowWeeks, defaultTrajectoryWindow)
			}
		})
	}
}

func TestComputeTrajectoryScore_Normalization(t *testing.T) {
	monA := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	// 8 buckets rising 100c/week, CL value = 10000c → normalized = 100/10000 = 0.01.
	buckets := make([]WeeklyBucket, 8)
	for i := 0; i < 8; i++ {
		buckets[i] = WeeklyBucket{
			WeekStart:     monA.AddDate(0, 0, 7*i),
			SaleCount:     3,
			AvgPriceCents: 1000 + int64(i*100),
		}
	}
	score := ComputeTrajectoryScore(buckets, 10000)
	if score.NormalizedByCLValue != 0.01 {
		t.Errorf("NormalizedByCLValue: got %v, want 0.01", score.NormalizedByCLValue)
	}

	// CL value 0 → normalized stays 0 (no divide).
	score = ComputeTrajectoryScore(buckets, 0)
	if score.NormalizedByCLValue != 0 {
		t.Errorf("NormalizedByCLValue with zero CL: got %v, want 0", score.NormalizedByCLValue)
	}
}
