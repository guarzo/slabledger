package arbitrage

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

const (
	monteCarloSimulations      = 1000 // Full simulation run count
	monteCarloQuickScanSims    = 200  // Reduced count for quick parameter scans
	monteCarloSampleCap        = 100  // Max cards sampled per simulation iteration
	monteCarloMinHistory       = 10   // Minimum historical data points to run projection
	monteCarloHighConfidence   = 50   // History count threshold for "high" confidence
	monteCarloMediumConfidence = 30   // History count threshold for "medium" confidence
)

// MonteCarloResult holds simulation results for a single parameter set.
type MonteCarloResult struct {
	Label        string  `json:"label"`
	Simulations  int     `json:"simulations"`
	MedianROI    float64 `json:"medianROI"`
	P10ROI       float64 `json:"p10ROI"`
	P90ROI       float64 `json:"p90ROI"`
	MedianProfit int     `json:"medianProfitCents"`
	P10Profit    int     `json:"p10ProfitCents"`
	P90Profit    int     `json:"p90ProfitCents"`
	MedianVolume int     `json:"medianVolume"`
}

// MonteCarloComparison holds the full comparison of simulated scenarios.
type MonteCarloComparison struct {
	Current      MonteCarloResult   `json:"current"`
	Scenarios    []MonteCarloResult `json:"scenarios"`
	BestScenario int                `json:"bestScenarioIndex"`
	SampleSize   int                `json:"sampleSize"`
	Confidence   string             `json:"confidence"`
}

// SimulationParams controls a single simulation scenario.
type SimulationParams struct {
	BuyTermsCLPct float64
	GradeMin      float64
	GradeMax      float64
}

// runSimulation runs N simulated campaign outcomes and returns the result.
func runSimulation(
	label string,
	params SimulationParams,
	history []inventory.PurchaseWithSale,
	n int,
	rng *rand.Rand,
) MonteCarloResult {
	if len(history) == 0 || n == 0 {
		return MonteCarloResult{Label: label, Simulations: n}
	}

	// Build distribution from historical data
	var eligible []inventory.PurchaseWithSale
	for _, d := range history {
		if d.Purchase.GradeValue >= params.GradeMin && d.Purchase.GradeValue <= params.GradeMax {
			eligible = append(eligible, d)
		}
	}
	if len(eligible) == 0 {
		return MonteCarloResult{Label: label, Simulations: n}
	}

	// Compute buy threshold filter
	var filtered []inventory.PurchaseWithSale
	for _, d := range eligible {
		buyPct := 0.0
		if d.Purchase.CLValueCents > 0 {
			buyPct = float64(d.Purchase.BuyCostCents) / float64(d.Purchase.CLValueCents)
		}
		if buyPct <= params.BuyTermsCLPct || params.BuyTermsCLPct == 0 {
			filtered = append(filtered, d)
		}
	}
	if len(filtered) == 0 {
		return MonteCarloResult{Label: label, Simulations: n}
	}

	// Compute sell-through and margin distributions
	sold := 0
	var margins []float64
	for _, d := range filtered {
		if d.Sale != nil {
			sold++
			cost := d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
			if cost > 0 {
				margins = append(margins, float64(d.Sale.NetProfitCents)/float64(cost))
			}
		}
	}
	sellThrough := 0.0
	if len(filtered) > 0 {
		sellThrough = float64(sold) / float64(len(filtered))
	}

	avgCost := 0
	for _, d := range filtered {
		avgCost += d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
	}
	if len(filtered) > 0 {
		avgCost /= len(filtered)
	}

	// Collect per-card costs for sampling in simulation.
	cardCosts := make([]int, 0, len(filtered))
	for _, d := range filtered {
		c := d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		if c > 0 {
			cardCosts = append(cardCosts, c)
		}
	}

	// Run simulations
	rois := make([]float64, n)
	profits := make([]int, n)
	volumes := make([]int, n)

	sampleSize := len(filtered) // Each sim draws sampleSize cards
	if sampleSize > monteCarloSampleCap {
		sampleSize = monteCarloSampleCap
	}

	for i := 0; i < n; i++ {
		totalSpend := 0
		totalProfit := 0
		cardsSold := 0

		for j := 0; j < sampleSize; j++ {
			// Use per-card cost sampled from dataset; fall back to avgCost if no costs collected.
			cost := avgCost
			if len(cardCosts) > 0 {
				cost = cardCosts[rng.Intn(len(cardCosts))]
			}
			totalSpend += cost

			// Sample sell/no-sell
			if rng.Float64() < sellThrough {
				// Sample margin from distribution
				margin := 0.0
				if len(margins) > 0 {
					margin = margins[rng.Intn(len(margins))]
				}
				profit := int(float64(cost) * margin)
				totalProfit += profit
				cardsSold++
			}
		}

		roi := 0.0
		if totalSpend > 0 {
			roi = float64(totalProfit) / float64(totalSpend)
		}
		rois[i] = roi
		profits[i] = totalProfit
		volumes[i] = cardsSold
	}

	sort.Float64s(rois)
	sort.Ints(profits)
	sort.Ints(volumes)

	return MonteCarloResult{
		Label:        label,
		Simulations:  n,
		MedianROI:    mcPercentile(rois, 0.50),
		P10ROI:       mcPercentile(rois, 0.10),
		P90ROI:       mcPercentile(rois, 0.90),
		MedianProfit: mcPercentile(profits, 0.50),
		P10Profit:    mcPercentile(profits, 0.10),
		P90Profit:    mcPercentile(profits, 0.90),
		MedianVolume: mcPercentile(volumes, 0.50),
	}
}

// ordered is a type constraint for number types that can be sorted.
type ordered interface {
	~int | ~float64
}

// mcPercentile returns the value at percentile p (0.0-1.0) from a pre-sorted slice.
// Returns the zero value of T if the slice is empty.
func mcPercentile[T ordered](sorted []T, p float64) T {
	if len(sorted) == 0 {
		var zero T
		return zero
	}
	idx := int(math.Round(p * float64(len(sorted)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// RunMonteCarloProjection orchestrates the full comparison simulation.
func RunMonteCarloProjection(campaign *inventory.Campaign, history []inventory.PurchaseWithSale) *MonteCarloComparison {
	if len(history) < monteCarloMinHistory {
		return &MonteCarloComparison{
			SampleSize: len(history),
			Confidence: "insufficient",
		}
	}

	seedGen := rand.New(rand.NewSource(42)) // Deterministic seed generator
	n := monteCarloSimulations

	// Parse grade range (supports decimals like "9.5-10")
	gradeMin, gradeMax := 1.0, 10.0
	if campaign.GradeRange != "" {
		if scanned, err := fmt.Sscanf(campaign.GradeRange, "%f-%f", &gradeMin, &gradeMax); err != nil || scanned != 2 {
			gradeMin, gradeMax = 1.0, 10.0 // fallback to full range on parse error
		}
	}

	// Scenario 1: Current parameters
	currentParams := SimulationParams{
		BuyTermsCLPct: campaign.BuyTermsCLPct,
		GradeMin:      gradeMin,
		GradeMax:      gradeMax,
	}
	current := runSimulation("Current Parameters", currentParams, history, n, rand.New(rand.NewSource(seedGen.Int63())))

	// Find empirical optimal BuyTermsCLPct
	bestPct := campaign.BuyTermsCLPct
	bestROI := current.MedianROI
	for pct := 0.40; pct <= 0.90; pct += 0.05 {
		test := runSimulation("", SimulationParams{BuyTermsCLPct: pct, GradeMin: gradeMin, GradeMax: gradeMax}, history, monteCarloQuickScanSims, rand.New(rand.NewSource(seedGen.Int63())))
		if test.MedianROI > bestROI {
			bestROI = test.MedianROI
			bestPct = pct
		}
	}

	scenarios := []MonteCarloResult{
		runSimulation(fmt.Sprintf("Optimal Buy Terms (%.0f%%)", bestPct*100),
			SimulationParams{BuyTermsCLPct: bestPct, GradeMin: gradeMin, GradeMax: gradeMax}, history, n, rand.New(rand.NewSource(seedGen.Int63()))),
		runSimulation(fmt.Sprintf("Buy Terms -5%% (%.0f%%)", (campaign.BuyTermsCLPct-0.05)*100),
			SimulationParams{BuyTermsCLPct: campaign.BuyTermsCLPct - 0.05, GradeMin: gradeMin, GradeMax: gradeMax}, history, n, rand.New(rand.NewSource(seedGen.Int63()))),
		runSimulation(fmt.Sprintf("Buy Terms +5%% (%.0f%%)", (campaign.BuyTermsCLPct+0.05)*100),
			SimulationParams{BuyTermsCLPct: campaign.BuyTermsCLPct + 0.05, GradeMin: gradeMin, GradeMax: gradeMax}, history, n, rand.New(rand.NewSource(seedGen.Int63()))),
	}

	// Narrowed grade range: drop the lowest whole-grade tier.
	// Uses +1 (not +0.5) intentionally to exclude an entire grade level
	// (e.g., 8→9 drops both PSA 8 and 8.5), since half-grade cards are
	// typically priced similarly to their floor grade.
	narrowedMin := gradeMin + 1
	if narrowedMin <= gradeMax {
		scenarios = append(scenarios, runSimulation(
			fmt.Sprintf("Narrowed Grade (PSA %.4g-%.4g)", narrowedMin, gradeMax),
			SimulationParams{BuyTermsCLPct: campaign.BuyTermsCLPct, GradeMin: narrowedMin, GradeMax: gradeMax},
			history, n, rand.New(rand.NewSource(seedGen.Int63())),
		))
	}

	// Find best scenario
	bestIdx := 0
	for i, s := range scenarios {
		if s.MedianROI > scenarios[bestIdx].MedianROI {
			bestIdx = i
		}
	}

	confidence := "low"
	switch {
	case len(history) >= monteCarloHighConfidence:
		confidence = "high"
	case len(history) >= monteCarloMediumConfidence:
		confidence = "medium"
	}

	return &MonteCarloComparison{
		Current:      current,
		Scenarios:    scenarios,
		BestScenario: bestIdx,
		SampleSize:   len(history),
		Confidence:   confidence,
	}
}
