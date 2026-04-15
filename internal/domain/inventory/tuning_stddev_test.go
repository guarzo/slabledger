package inventory

import (
	"math"
	"testing"
)

func TestComputeROIStats(t *testing.T) {
	const eps = 1e-9

	tests := []struct {
		name       string
		rois       []float64
		wantStddev float64
		wantCV     float64
	}{
		{
			name:       "empty",
			rois:       nil,
			wantStddev: 0,
			wantCV:     0,
		},
		{
			name:       "single",
			rois:       []float64{0.5},
			wantStddev: 0,
			wantCV:     0,
		},
		{
			name:       "two equal",
			rois:       []float64{0.2, 0.2},
			wantStddev: 0,
			wantCV:     0,
		},
		{
			name:       "two values",
			rois:       []float64{0.0, 0.2},
			wantStddev: 0.1,
			wantCV:     1.0,
		},
		{
			name:       "three values",
			rois:       []float64{0.10, 0.20, 0.30},
			wantStddev: math.Sqrt(2.0/3.0) / 10.0, // ≈ 0.08164965809
			wantCV:     (math.Sqrt(2.0/3.0) / 10.0) / 0.20,
		},
		{
			name:       "zero mean nonzero stddev",
			rois:       []float64{-0.1, 0.1},
			wantStddev: 0.1,
			wantCV:     0, // ROI==0 → CV must be 0
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStddev, gotCV := ComputeROIStats(tc.rois)
			if math.Abs(gotStddev-tc.wantStddev) > eps {
				t.Errorf("stddev: got %v, want %v", gotStddev, tc.wantStddev)
			}
			if math.Abs(gotCV-tc.wantCV) > eps {
				t.Errorf("cv: got %v, want %v", gotCV, tc.wantCV)
			}
		})
	}
}

func TestEnrichPriceTierStddev(t *testing.T) {
	const eps = 1e-9

	// Tiers:
	//  [0, 1000)           — "low"
	//  [1000, 5000)        — "mid"
	//  [5000, math.MaxInt) — "high"
	tiers := []PriceTierPerformance{
		{TierLabel: "low", TierMinCents: 0, TierMaxCents: 1000},
		{TierLabel: "mid", TierMinCents: 1000, TierMaxCents: 5000},
		{TierLabel: "high", TierMinCents: 5000, TierMaxCents: math.MaxInt},
	}

	// Tier matching uses bucketCost = BuyCostCents only.
	// ROI uses roiCost = BuyCostCents + PSASourcingFeeCents.
	// Data:
	//  "low" tier: 1 sale (bucketCost=400) → <2 sales → stddev=0
	//  "mid" tier: 2 sales (bucketCost=1500, 2500) → >=2 sales → non-zero stddev
	//  "high" tier: no sales → stddev=0
	//  Zero-roiCost purchase: skipped.
	//  Unsold (Sale == nil): skipped.
	data := []PurchaseWithSale{
		{
			Purchase: Purchase{BuyCostCents: 400, PSASourcingFeeCents: 100}, // bucketCost=400 (low), roiCost=500
			Sale:     &Sale{NetProfitCents: 50},                             // roi = 50/500 = 0.1
		},
		{
			Purchase: Purchase{BuyCostCents: 1500, PSASourcingFeeCents: 500}, // bucketCost=1500 (mid), roiCost=2000
			Sale:     &Sale{NetProfitCents: 200},                             // roi = 200/2000 = 0.1
		},
		{
			Purchase: Purchase{BuyCostCents: 2500, PSASourcingFeeCents: 500}, // bucketCost=2500 (mid), roiCost=3000
			Sale:     &Sale{NetProfitCents: 900},                             // roi = 900/3000 = 0.3
		},
		{
			// Zero cost — should be skipped entirely.
			Purchase: Purchase{BuyCostCents: 0, PSASourcingFeeCents: 0},
			Sale:     &Sale{NetProfitCents: 100},
		},
		{
			// Unsold — should be skipped.
			Purchase: Purchase{BuyCostCents: 2000, PSASourcingFeeCents: 0},
			Sale:     nil,
		},
	}

	EnrichPriceTierStddev(tiers, data)

	// low tier: only 1 sale → stddev=0, cv=0
	if tiers[0].RoiStddev != 0 {
		t.Errorf("low tier stddev: got %v, want 0", tiers[0].RoiStddev)
	}
	if tiers[0].CV != 0 {
		t.Errorf("low tier cv: got %v, want 0", tiers[0].CV)
	}

	// mid tier: rois=[0.1, 0.3], mean=0.2, variance=(0.01+0.01)/2=0.01, stddev=0.1, cv=0.5
	wantMidStddev := 0.1
	wantMidCV := 0.5
	if math.Abs(tiers[1].RoiStddev-wantMidStddev) > eps {
		t.Errorf("mid tier stddev: got %v, want %v", tiers[1].RoiStddev, wantMidStddev)
	}
	if math.Abs(tiers[1].CV-wantMidCV) > eps {
		t.Errorf("mid tier cv: got %v, want %v", tiers[1].CV, wantMidCV)
	}

	// high tier: no sales → stddev=0, cv=0
	if tiers[2].RoiStddev != 0 {
		t.Errorf("high tier stddev: got %v, want 0", tiers[2].RoiStddev)
	}
	if tiers[2].CV != 0 {
		t.Errorf("high tier cv: got %v, want 0", tiers[2].CV)
	}
}

func TestEnrichPriceTierStddev_EmptyTiers(t *testing.T) {
	// Should not panic.
	EnrichPriceTierStddev(nil, []PurchaseWithSale{
		{Purchase: Purchase{BuyCostCents: 1000}, Sale: &Sale{NetProfitCents: 100}},
	})
}
