package arbitrage

import "math"

// DefaultMarketplaceFeePct is the default eBay/TCGPlayer fee percentage (12.35%).
const DefaultMarketplaceFeePct = 0.1235

// confidenceLabel returns a confidence string based on data point count.
func confidenceLabel(n int) string {
	switch {
	case n >= 20:
		return "high"
	case n >= 5:
		return "medium"
	default:
		return "low"
	}
}

// ExpectedValue contains the EV computation for a single card.
type ExpectedValue struct {
	CardName           string  `json:"cardName"`
	CertNumber         string  `json:"certNumber"`
	Grade              float64 `json:"grade"`
	CostBasisCents     int     `json:"costBasisCents"`
	SellProbability    float64 `json:"sellProbability"`
	ExpectedSalePrice  int     `json:"expectedSalePriceCents"`
	ExpectedFees       int     `json:"expectedFeesCents"`
	ExpectedProfit     int     `json:"expectedProfitCents"`
	CarryingCostCents  int     `json:"carryingCostCents"`
	EVCents            int     `json:"evCents"`
	EVPerDollar        float64 `json:"evPerDollar"`
	SegmentSellThrough float64 `json:"segmentSellThrough"`
	LiquidityFactor    float64 `json:"liquidityFactor"`
	TrendAdjustment    float64 `json:"trendAdjustment"`
	Confidence         string  `json:"confidence"`
}

// EVPortfolio contains aggregate EV across a campaign.
type EVPortfolio struct {
	Items         []ExpectedValue `json:"items"`
	TotalEVCents  int             `json:"totalEvCents"`
	PositiveCount int             `json:"positiveCount"`
	NegativeCount int             `json:"negativeCount"`
	MinDataPoints int             `json:"minDataPoints"`
}

// computeExpectedValue is a pure function that computes the expected value of a card position.
func computeExpectedValue(
	cardName, certNumber string,
	grade float64,
	costBasis int,
	segmentSellThrough float64,
	segmentMedianMarginPct float64,
	liquidityFactor float64,
	trendAdjustment float64,
	avgDaysUnsold float64,
	annualCapitalCostRate float64,
	dataPoints int,
	feePctOpts ...float64,
) *ExpectedValue {
	feePct := DefaultMarketplaceFeePct
	if len(feePctOpts) > 0 && feePctOpts[0] > 0 {
		feePct = feePctOpts[0]
	}

	// Adjust sell probability by liquidity
	pSell := segmentSellThrough * math.Min(math.Max(liquidityFactor, 0.5), 2.0)
	pSell = math.Min(pSell, 0.99) // Cap at 99%

	// Expected sale price: costBasis * (1 + medianMargin) * (1 + trend)
	expectedSale := float64(costBasis) * (1 + segmentMedianMarginPct) * (1 + trendAdjustment)
	expectedSaleCents := int(expectedSale)

	// Expected fees
	expectedFees := int(expectedSale * feePct)

	// Expected profit if sold
	expectedProfit := expectedSaleCents - expectedFees - costBasis

	// Carrying cost = opportunity cost of capital
	daysHeldEstimate := avgDaysUnsold
	if daysHeldEstimate < 1 {
		daysHeldEstimate = 30
	}
	carryingCost := int(float64(costBasis) * annualCapitalCostRate * daysHeldEstimate / 365.0)

	// E[V] = P(sell) * E[profit|sell] - (1-P(sell)) * carryingCost
	ev := int(pSell*float64(expectedProfit) - (1-pSell)*float64(carryingCost))

	evPerDollar := 0.0
	if costBasis > 0 {
		evPerDollar = float64(ev) / float64(costBasis)
	}

	confidence := confidenceLabel(dataPoints)

	return &ExpectedValue{
		CardName:           cardName,
		CertNumber:         certNumber,
		Grade:              grade,
		CostBasisCents:     costBasis,
		SellProbability:    pSell,
		ExpectedSalePrice:  expectedSaleCents,
		ExpectedFees:       expectedFees,
		ExpectedProfit:     expectedProfit,
		CarryingCostCents:  carryingCost,
		EVCents:            ev,
		EVPerDollar:        evPerDollar,
		SegmentSellThrough: segmentSellThrough,
		LiquidityFactor:    liquidityFactor,
		TrendAdjustment:    trendAdjustment,
		Confidence:         confidence,
	}
}
