package arbitrage

import "testing"

func TestComputeExpectedValue(t *testing.T) {
	ev := computeExpectedValue(
		"Charizard", "12345", 9,
		8500, // costBasis
		0.72, // sell-through
		0.15, // 15% median margin
		1.0,  // normal liquidity
		0.0,  // no trend
		45.0, // avg 45 days unsold
		0.05, // 5% annual capital cost
		25,   // data points
	)

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
	ev := computeExpectedValue(
		"Pikachu", "67890", 8,
		10000, // high cost basis
		0.20,  // low sell-through
		-0.10, // negative margin
		0.5,   // low liquidity
		-0.05, // declining trend
		90.0,  // avg 90 days unsold
		0.05,  // 5% annual capital cost
		3,     // few data points
	)

	if ev.EVCents >= 0 {
		t.Errorf("expected negative EV for unprofitable segment, got %d", ev.EVCents)
	}
	if ev.Confidence != "low" {
		t.Errorf("expected low confidence with 3 data points, got %s", ev.Confidence)
	}
}
