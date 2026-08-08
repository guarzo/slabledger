package inventory

import "testing"

// The boundary is the whole point of this function: ComputeMarketAlignment and
// computeMarketSignal previously disagreed about which side exactly ±0.05 fell
// on (SLA-60). These cases pin the inclusive rule so the two cannot diverge
// again without a test failing.
func TestClassifyMarketDrift(t *testing.T) {
	// Smallest step that reliably moves a float64 off the threshold, so the
	// "just inside" cases cannot land back on the boundary through rounding.
	const epsilon = 1e-9

	tests := []struct {
		name  string
		delta float64
		want  MarketDrift
	}{
		{"exactly +threshold is up, not stable", MarketDriftThreshold, MarketDriftUp},
		{"exactly -threshold is down, not stable", -MarketDriftThreshold, MarketDriftDown},
		{"just inside +threshold is stable", MarketDriftThreshold - epsilon, MarketDriftStable},
		{"just inside -threshold is stable", -MarketDriftThreshold + epsilon, MarketDriftStable},
		{"just outside +threshold is up", MarketDriftThreshold + epsilon, MarketDriftUp},
		{"just outside -threshold is down", -MarketDriftThreshold - epsilon, MarketDriftDown},
		{"zero drift is stable", 0, MarketDriftStable},
		{"large positive drift is up", 1.5, MarketDriftUp},
		{"large negative drift is down", -1.5, MarketDriftDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyMarketDrift(tt.delta); got != tt.want {
				t.Errorf("ClassifyMarketDrift(%v) = %v, want %v", tt.delta, got, tt.want)
			}
		})
	}
}

// A drift computed as an exact 1/20th ratio of two ints is the only realistic
// way production reaches the boundary, so verify it classifies as Up rather
// than landing on the stable side through float division error.
func TestClassifyMarketDriftExactRatioHitsBoundary(t *testing.T) {
	const base, delta = 2000, 100 // 100/2000 == 0.05 exactly

	drift := float64(delta) / float64(base)
	if drift != MarketDriftThreshold {
		t.Fatalf("precondition: %d/%d = %v, want exactly %v", delta, base, drift, MarketDriftThreshold)
	}
	if got := ClassifyMarketDrift(drift); got != MarketDriftUp {
		t.Errorf("ClassifyMarketDrift(%v) = %v, want MarketDriftUp", drift, got)
	}
}
