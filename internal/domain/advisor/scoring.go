package advisor

import (
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/scoring"
)

// Gap reason constants used when optional market data is missing.
const (
	gapNoMarketData      = "no_market_data"
	gapInsufficientSales = "insufficient_sales"
	gapNoPopulationData  = "no_population_data"
)

// PurchaseFactorData contains raw inputs for purchase assessment factor computers.
type PurchaseFactorData struct {
	PriceChangePct    *float64
	SalesPerMonth     *float64
	ROIPct            *float64
	PSA10Pop          *int
	Trend30dPct       *float64
	ConcentrationRisk string
	GradeROI          *float64
	CampaignAvgROI    *float64
	PriceConfidence   float64
	MarketSource      string
}

// CampaignFactorData contains raw inputs for campaign analysis factor computers.
type CampaignFactorData struct {
	ROIPct          *float64
	SellThroughPct  *float64
	Trend30dPct     *float64
	SalesPerMonth   *float64
	FillRatePct     *float64
	PriceChangePct  *float64
	CampaignROI     *float64
	PriceConfidence float64
	MarketSource    string
}

// LiquidationFactorData contains raw inputs for liquidation factor computers.
type LiquidationFactorData struct {
	DaysHeld        int
	WeeksToCover    *float64
	PriceChangePct  *float64
	SalesPerMonth   *float64
	CrackROI        *float64
	GradedROI       *float64
	ROIPct          *float64
	PSA10Pop        *int
	PriceConfidence float64
	MarketSource    string
}

// SuggestionFactorData contains raw inputs for suggestion factor computers.
type SuggestionFactorData struct {
	ProjectedROIPct *float64
	FillsGap        bool
	OverlapCount    int
	Trend30dPct     *float64
	SalesPerMonth   *float64
	PriceChangePct  *float64
	PriceConfidence float64
	MarketSource    string
}

// BuildScoreCard computes a ScoreCard from raw factor data.
func BuildScoreCard(entityID, entityType string, data any, profile scoring.WeightProfile) (scoring.ScoreCard, error) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	switch d := data.(type) {
	case *PurchaseFactorData:
		factors, gaps = purchaseFactors(d)
	case *CampaignFactorData:
		factors, gaps = campaignFactors(d)
	case *LiquidationFactorData:
		factors, gaps = liquidationFactors(d)
	case *SuggestionFactorData:
		factors, gaps = suggestionFactors(d)
	default:
		return scoring.ScoreCard{}, errors.NewAppError(ErrCodeUnsupportedType, "unsupported factor data type").
			WithContext("type", fmt.Sprintf("%T", data))
	}

	req := scoring.ScoreRequest{
		EntityID:   entityID,
		EntityType: entityType,
		Factors:    factors,
		DataGaps:   gaps,
	}

	sc, err := scoring.Score(req, profile)
	if err != nil {
		return sc, err
	}
	return scoring.ApplySafetyFilters(sc), nil
}

// addOrGap appends a computed factor when present is true, otherwise records a data gap.
// The compute closure is only called when present is true, making nil-pointer dereferences safe.
func addOrGap(factors *[]scoring.Factor, gaps *[]scoring.DataGap, present bool, compute func() scoring.Factor, name, reason string) {
	if present {
		*factors = append(*factors, compute())
	} else {
		*gaps = append(*gaps, scoring.DataGap{FactorName: name, Reason: reason})
	}
}

func purchaseFactors(d *PurchaseFactorData) ([]scoring.Factor, []scoring.DataGap) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	addOrGap(&factors, &gaps, d.PriceChangePct != nil, func() scoring.Factor {
		return scoring.ComputeMarketTrend(*d.PriceChangePct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorMarketTrend, gapNoMarketData)

	addOrGap(&factors, &gaps, d.SalesPerMonth != nil, func() scoring.Factor {
		return scoring.ComputeLiquidity(*d.SalesPerMonth, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorLiquidity, gapInsufficientSales)

	addOrGap(&factors, &gaps, d.ROIPct != nil, func() scoring.Factor {
		return scoring.ComputeROIPotential(*d.ROIPct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorROIPotential, gapNoMarketData)

	factors = append(factors, scoring.ComputePortfolioFit(d.ConcentrationRisk, 1.0, "portfolio"))

	addOrGap(&factors, &gaps, d.PSA10Pop != nil, func() scoring.Factor {
		return scoring.ComputeScarcity(*d.PSA10Pop, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorScarcity, gapNoPopulationData)

	addOrGap(&factors, &gaps, d.GradeROI != nil && d.CampaignAvgROI != nil, func() scoring.Factor {
		return scoring.ComputeGradeFit(*d.GradeROI, *d.CampaignAvgROI, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorGradeFit, gapInsufficientSales)

	addOrGap(&factors, &gaps, d.Trend30dPct != nil, func() scoring.Factor {
		return scoring.ComputeMarketAlignment(*d.Trend30dPct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorMarketAlignment, gapNoMarketData)

	return factors, gaps
}

func campaignFactors(d *CampaignFactorData) ([]scoring.Factor, []scoring.DataGap) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	addOrGap(&factors, &gaps, d.ROIPct != nil, func() scoring.Factor {
		return scoring.ComputeROIPotential(*d.ROIPct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorROIPotential, gapNoMarketData)

	addOrGap(&factors, &gaps, d.SellThroughPct != nil, func() scoring.Factor {
		return scoring.ComputeSellThrough(*d.SellThroughPct, 1.0, "campaigns")
	}, scoring.FactorSellThrough, gapInsufficientSales)

	addOrGap(&factors, &gaps, d.Trend30dPct != nil, func() scoring.Factor {
		return scoring.ComputeMarketAlignment(*d.Trend30dPct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorMarketAlignment, gapNoMarketData)

	addOrGap(&factors, &gaps, d.SalesPerMonth != nil, func() scoring.Factor {
		return scoring.ComputeLiquidity(*d.SalesPerMonth, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorLiquidity, gapInsufficientSales)

	addOrGap(&factors, &gaps, d.FillRatePct != nil, func() scoring.Factor {
		roi := 0.0
		if d.CampaignROI != nil {
			roi = *d.CampaignROI
		}
		return scoring.ComputeSpendEfficiency(*d.FillRatePct, roi, 1.0, "campaigns")
	}, scoring.FactorSpendEfficiency, gapInsufficientSales)

	addOrGap(&factors, &gaps, d.PriceChangePct != nil, func() scoring.Factor {
		return scoring.ComputeMarketTrend(*d.PriceChangePct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorMarketTrend, gapNoMarketData)

	return factors, gaps
}

func liquidationFactors(d *LiquidationFactorData) ([]scoring.Factor, []scoring.DataGap) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	factors = append(factors, scoring.ComputeCarryingCost(d.DaysHeld, 1.0, "purchase"))

	addOrGap(&factors, &gaps, d.WeeksToCover != nil, func() scoring.Factor {
		return scoring.ComputeCapitalPressure(*d.WeeksToCover, 1.0, "capital")
	}, scoring.FactorCapitalPressure, gapNoMarketData)

	addOrGap(&factors, &gaps, d.PriceChangePct != nil, func() scoring.Factor {
		// Negate: a falling market (negative price change) increases liquidation urgency
		return scoring.ComputeMarketTrend(-*d.PriceChangePct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorMarketTrend, gapNoMarketData)

	addOrGap(&factors, &gaps, d.SalesPerMonth != nil, func() scoring.Factor {
		return scoring.ComputeLiquidity(*d.SalesPerMonth, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorLiquidity, gapInsufficientSales)

	addOrGap(&factors, &gaps, d.CrackROI != nil && d.GradedROI != nil, func() scoring.Factor {
		return scoring.ComputeCrackAdvantage(*d.CrackROI, *d.GradedROI, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorCrackAdvantage, gapNoMarketData)

	addOrGap(&factors, &gaps, d.ROIPct != nil, func() scoring.Factor {
		return scoring.ComputeROIPotential(*d.ROIPct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorROIPotential, gapNoMarketData)

	addOrGap(&factors, &gaps, d.PSA10Pop != nil, func() scoring.Factor {
		return scoring.ComputeScarcity(*d.PSA10Pop, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorScarcity, gapNoPopulationData)

	return factors, gaps
}

func suggestionFactors(d *SuggestionFactorData) ([]scoring.Factor, []scoring.DataGap) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	addOrGap(&factors, &gaps, d.ProjectedROIPct != nil, func() scoring.Factor {
		return scoring.ComputeROIPotential(*d.ProjectedROIPct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorROIPotential, gapNoMarketData)

	factors = append(factors, scoring.ComputeCoverageImpact(d.FillsGap, d.OverlapCount, 1.0, "portfolio"))

	addOrGap(&factors, &gaps, d.Trend30dPct != nil, func() scoring.Factor {
		return scoring.ComputeMarketAlignment(*d.Trend30dPct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorMarketAlignment, gapNoMarketData)

	addOrGap(&factors, &gaps, d.SalesPerMonth != nil, func() scoring.Factor {
		return scoring.ComputeLiquidity(*d.SalesPerMonth, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorLiquidity, gapInsufficientSales)

	addOrGap(&factors, &gaps, d.PriceChangePct != nil, func() scoring.Factor {
		return scoring.ComputeMarketTrend(*d.PriceChangePct, d.PriceConfidence, d.MarketSource)
	}, scoring.FactorMarketTrend, gapNoMarketData)

	return factors, gaps
}
