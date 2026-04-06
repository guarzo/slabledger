package scoring

import (
	"fmt"
	"math"
)

var factorDisplayNames = map[string]string{
	FactorMarketTrend:     "Market Trend",
	FactorLiquidity:       "Liquidity",
	FactorROIPotential:    "ROI Potential",
	FactorScarcity:        "Scarcity",
	FactorMarketAlignment: "Market Alignment",
	FactorPortfolioFit:    "Portfolio Fit",
	FactorGradeFit:        "Grade Fit",
	FactorCapitalPressure: "Capital Pressure",
	FactorCarryingCost:    "Carrying Cost",
	FactorCrackAdvantage:  "Crack Advantage",
	FactorSellThrough:     "Sell-Through Rate",
	FactorSpendEfficiency: "Spend Efficiency",
	FactorCoverageImpact:  "Coverage Impact",
}

func FallbackResult(sc ScoreCard) StructuredResult {
	if len(sc.Factors) == 0 {
		return StructuredResult{
			ScoreCard:  sc,
			Verdict:    sc.Verdict,
			KeyInsight: fmt.Sprintf("No factor data available; overall signal is %s", string(sc.Verdict)),
		}
	}
	signals := make([]Signal, 0, len(sc.Factors))
	strongest := sc.Factors[0]
	for _, f := range sc.Factors {
		dir := factorDirection(f.Value)
		title := factorDisplayNames[f.Name]
		if title == "" {
			title = f.Name
		}
		signals = append(signals, Signal{
			Factor:    f.Name,
			Direction: dir,
			Title:     title,
			Detail:    fmt.Sprintf("Score: %.2f (confidence: %.0f%%)", f.Value, f.Confidence*100),
			Metric:    fmt.Sprintf("%.2f", f.Value),
		})
		if math.Abs(f.Value) > math.Abs(strongest.Value) {
			strongest = f
		}
	}
	insight := generateInsight(strongest, sc.Verdict)
	return StructuredResult{
		ScoreCard:        sc,
		Verdict:          sc.Verdict,
		AdjustmentReason: nil,
		KeyInsight:       insight,
		Signals:          signals,
	}
}

func factorDirection(value float64) string {
	switch {
	case value > 0.1:
		return "bullish"
	case value < -0.1:
		return "bearish"
	default:
		return "neutral"
	}
}

func generateInsight(strongest Factor, verdict Verdict) string {
	name := factorDisplayNames[strongest.Name]
	if name == "" {
		name = strongest.Name
	}
	dir := factorDirection(strongest.Value)
	return fmt.Sprintf("%s is %s (%.2f), driving an overall %s signal", name, dir, strongest.Value, string(verdict))
}
