package arbitrage

import (
	"math/rand"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestRunSimulation_Deterministic(t *testing.T) {
	history := []inventory.PurchaseWithSale{
		{Purchase: inventory.Purchase{BuyCostCents: 8500, PSASourcingFeeCents: 300, GradeValue: 9, CLValueCents: 15000},
			Sale: &inventory.Sale{NetProfitCents: 2000, SaleFeeCents: 1200}},
		{Purchase: inventory.Purchase{BuyCostCents: 5000, PSASourcingFeeCents: 300, GradeValue: 8, CLValueCents: 8000},
			Sale: &inventory.Sale{NetProfitCents: 1000, SaleFeeCents: 800}},
		{Purchase: inventory.Purchase{BuyCostCents: 10000, PSASourcingFeeCents: 300, GradeValue: 10, CLValueCents: 20000},
			Sale: &inventory.Sale{NetProfitCents: 5000, SaleFeeCents: 2000}},
		{Purchase: inventory.Purchase{BuyCostCents: 7000, PSASourcingFeeCents: 300, GradeValue: 9.5, CLValueCents: 12000}},
		{Purchase: inventory.Purchase{BuyCostCents: 6000, PSASourcingFeeCents: 300, GradeValue: 8, CLValueCents: 10000},
			Sale: &inventory.Sale{NetProfitCents: -500, SaleFeeCents: 900}},
	}

	rng := rand.New(rand.NewSource(42))
	result := runSimulation("test", SimulationParams{BuyTermsCLPct: 0.65, GradeMin: 8, GradeMax: 10}, history, 100, rng)

	if result.Simulations != 100 {
		t.Errorf("expected 100 simulations, got %d", result.Simulations)
	}
	// With fixed seed, result should be deterministic
	rng2 := rand.New(rand.NewSource(42))
	result2 := runSimulation("test", SimulationParams{BuyTermsCLPct: 0.65, GradeMin: 8, GradeMax: 10}, history, 100, rng2)

	if result.MedianROI != result2.MedianROI {
		t.Error("expected deterministic results with same seed")
	}
}

func TestRunMonteCarloProjection_DecimalGrade(t *testing.T) {
	// Build 20 data points so we exceed the "insufficient" threshold
	var history []inventory.PurchaseWithSale
	for i := 0; i < 20; i++ {
		pw := inventory.PurchaseWithSale{
			Purchase: inventory.Purchase{
				BuyCostCents:        5000 + i*100,
				PSASourcingFeeCents: 300,
				GradeValue:          9.5,
				CLValueCents:        12000 + i*200,
			},
		}
		if i%2 == 0 {
			pw.Sale = &inventory.Sale{NetProfitCents: 1500, SaleFeeCents: 900}
		}
		history = append(history, pw)
	}

	campaign := &inventory.Campaign{BuyTermsCLPct: 0.60, GradeRange: "9.5-9.5"}
	result := RunMonteCarloProjection(campaign, history)

	if result.Confidence == "insufficient" {
		t.Fatalf("expected sufficient confidence with 20 data points, got %s", result.Confidence)
	}
	// The 9.5-grade purchases must pass the grade filter (not be rounded to int).
	// If parsing truncated "9.5-9.5" → 9-9, a 9.5-grade card would be excluded.
	if result.Current.Simulations == 0 {
		t.Error("expected simulations to run (grade filter should include 9.5)")
	}
	if result.Current.MedianVolume == 0 {
		t.Error("expected non-zero median volume (9.5-grade purchases should match 9.5-9.5 range)")
	}
}

func TestRunMonteCarloProjection_InsufficientData(t *testing.T) {
	campaign := &inventory.Campaign{BuyTermsCLPct: 0.60, GradeRange: "8-10"}
	history := []inventory.PurchaseWithSale{
		{Purchase: inventory.Purchase{BuyCostCents: 5000, GradeValue: 9}},
	}

	result := RunMonteCarloProjection(campaign, history)
	if result.Confidence != "insufficient" {
		t.Errorf("expected insufficient confidence with 1 data point, got %s", result.Confidence)
	}
}

func TestRunMonteCarloProjection_SufficientData(t *testing.T) {
	campaign := &inventory.Campaign{BuyTermsCLPct: 0.60, GradeRange: "8-10"}
	var history []inventory.PurchaseWithSale
	for i := 0; i < 50; i++ {
		pw := inventory.PurchaseWithSale{
			Purchase: inventory.Purchase{
				BuyCostCents:        5000 + i*100,
				PSASourcingFeeCents: 300,
				GradeValue:          float64(8 + i%3),
				CLValueCents:        10000 + i*200,
			},
		}
		if i%3 != 0 { // 2/3 sell
			pw.Sale = &inventory.Sale{
				NetProfitCents: 500 + i*50,
				SaleFeeCents:   800,
			}
		}
		history = append(history, pw)
	}

	result := RunMonteCarloProjection(campaign, history)
	if result.Confidence != "high" {
		t.Errorf("expected high confidence with 50 data points, got %s", result.Confidence)
	}
	if len(result.Scenarios) < 3 {
		t.Errorf("expected at least 3 scenarios, got %d", len(result.Scenarios))
	}
	if result.Current.Simulations != 1000 {
		t.Errorf("expected 1000 simulations, got %d", result.Current.Simulations)
	}
}
