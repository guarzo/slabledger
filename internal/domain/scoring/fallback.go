package scoring

import "fmt"

var factorDisplayNames = map[string]string{
	FactorMarketTrend:     "Market Trend",
	FactorLiquidity:       "Liquidity",
	FactorROIPotential:    "ROI Potential",
	FactorScarcity:        "Scarcity",
	FactorMarketAlignment: "Market Alignment",
	FactorPortfolioFit:    "Portfolio Fit",
	FactorGradeFit:        "Grade Fit",
	FactorCreditPressure:  "Credit Pressure",
	FactorCarryingCost:    "Carrying Cost",
	FactorCrackAdvantage:  "Crack Advantage",
	FactorSellThrough:     "Sell-Through Rate",
	FactorSpendEfficiency: "Spend Efficiency",
	FactorCoverageImpact:  "Coverage Impact",
}

func FallbackResult(sc ScoreCard) StructuredResult {
	signals := make([]Signal, 0, len(sc.Factors))
	var strongest Factor
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
		if absFloat(f.Value) > absFloat(strongest.Value) {
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

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
