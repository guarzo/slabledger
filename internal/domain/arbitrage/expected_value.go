package arbitrage

import (
	"math"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// DefaultMarketplaceFeePct is the default eBay/TCGPlayer fee percentage (12.35%).
const DefaultMarketplaceFeePct = constants.DefaultMarketplaceFeePct

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

// EVInput holds parameters for computeExpectedValue.
// All fields have documented units to prevent positional errors.
type EVInput struct {
	CardName               string
	CertNumber             string
	Grade                  float64 // 1.0-10.0
	CostBasis              int     // cents
	SegmentSellThrough     float64 // 0.0-1.0
	SegmentMedianMarginPct float64 // fraction, e.g. 0.25 = 25%
	LiquidityFactor        float64 // multiplier; 1.0 = neutral
	TrendAdjustment        float64 // fraction; 0.05 = +5% trend
	AvgDaysUnsold          float64 // average days held before sale
	AnnualCapitalCostRate  float64 // fraction per year; e.g. 0.08 = 8%
	DataPoints             int     // number of comparable sales
	FeePct                 float64 // optional override; 0 = use DefaultMarketplaceFeePct
}

// computeExpectedValue is a pure function that computes the expected value of a card position.
func computeExpectedValue(in EVInput) *ExpectedValue {
	feePct := DefaultMarketplaceFeePct
	if in.FeePct > 0 {
		feePct = in.FeePct
	}

	// Adjust sell probability by liquidity
	pSell := in.SegmentSellThrough * math.Min(math.Max(in.LiquidityFactor, 0.5), 2.0)
	pSell = math.Min(pSell, 0.99) // Cap at 99%

	// Expected sale price: costBasis * (1 + medianMargin) * (1 + trend)
	expectedSale := float64(in.CostBasis) * (1 + in.SegmentMedianMarginPct) * (1 + in.TrendAdjustment)
	expectedSaleCents := int(expectedSale)

	// Expected fees
	expectedFees := int(expectedSale * feePct)

	// Expected profit if sold
	expectedProfit := expectedSaleCents - expectedFees - in.CostBasis

	// Carrying cost = opportunity cost of capital
	daysHeldEstimate := in.AvgDaysUnsold
	if daysHeldEstimate < 1 {
		daysHeldEstimate = 30
	}
	carryingCost := int(float64(in.CostBasis) * in.AnnualCapitalCostRate * daysHeldEstimate / 365.0)

	// E[V] = P(sell) * E[profit|sell] - (1-P(sell)) * carryingCost
	ev := int(pSell*float64(expectedProfit) - (1-pSell)*float64(carryingCost))

	evPerDollar := 0.0
	if in.CostBasis > 0 {
		evPerDollar = float64(ev) / float64(in.CostBasis)
	}

	return &ExpectedValue{
		CardName:           in.CardName,
		CertNumber:         in.CertNumber,
		Grade:              in.Grade,
		CostBasisCents:     in.CostBasis,
		SellProbability:    pSell,
		ExpectedSalePrice:  expectedSaleCents,
		ExpectedFees:       expectedFees,
		ExpectedProfit:     expectedProfit,
		CarryingCostCents:  carryingCost,
		EVCents:            ev,
		EVPerDollar:        evPerDollar,
		SegmentSellThrough: in.SegmentSellThrough,
		LiquidityFactor:    in.LiquidityFactor,
		TrendAdjustment:    in.TrendAdjustment,
		Confidence:         confidenceLabel(in.DataPoints),
	}
}
