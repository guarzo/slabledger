package advisor

import (
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/scoring"
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
	CreditUtilPct   *float64
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
		return scoring.ScoreCard{}, ErrUnsupportedType.WithContext("type", fmt.Sprintf("%T", data))
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

func purchaseFactors(d *PurchaseFactorData) ([]scoring.Factor, []scoring.DataGap) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	if d.PriceChangePct != nil {
		factors = append(factors, scoring.ComputeMarketTrend(*d.PriceChangePct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorMarketTrend, Reason: "no_market_data"})
	}
	if d.SalesPerMonth != nil {
		factors = append(factors, scoring.ComputeLiquidity(*d.SalesPerMonth, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorLiquidity, Reason: "insufficient_sales"})
	}
	if d.ROIPct != nil {
		factors = append(factors, scoring.ComputeROIPotential(*d.ROIPct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorROIPotential, Reason: "no_market_data"})
	}
	factors = append(factors, scoring.ComputePortfolioFit(d.ConcentrationRisk, 1.0, "portfolio"))
	if d.PSA10Pop != nil {
		factors = append(factors, scoring.ComputeScarcity(*d.PSA10Pop, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorScarcity, Reason: "no_population_data"})
	}
	if d.GradeROI != nil && d.CampaignAvgROI != nil {
		factors = append(factors, scoring.ComputeGradeFit(*d.GradeROI, *d.CampaignAvgROI, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorGradeFit, Reason: "insufficient_sales"})
	}
	if d.Trend30dPct != nil {
		factors = append(factors, scoring.ComputeMarketAlignment(*d.Trend30dPct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorMarketAlignment, Reason: "no_market_data"})
	}
	return factors, gaps
}

func campaignFactors(d *CampaignFactorData) ([]scoring.Factor, []scoring.DataGap) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	if d.ROIPct != nil {
		factors = append(factors, scoring.ComputeROIPotential(*d.ROIPct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorROIPotential, Reason: "no_market_data"})
	}
	if d.SellThroughPct != nil {
		factors = append(factors, scoring.ComputeSellThrough(*d.SellThroughPct, 1.0, "campaigns"))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorSellThrough, Reason: "insufficient_sales"})
	}
	if d.Trend30dPct != nil {
		factors = append(factors, scoring.ComputeMarketAlignment(*d.Trend30dPct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorMarketAlignment, Reason: "no_market_data"})
	}
	if d.SalesPerMonth != nil {
		factors = append(factors, scoring.ComputeLiquidity(*d.SalesPerMonth, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorLiquidity, Reason: "insufficient_sales"})
	}
	if d.FillRatePct != nil {
		roi := 0.0
		if d.CampaignROI != nil {
			roi = *d.CampaignROI
		}
		factors = append(factors, scoring.ComputeSpendEfficiency(*d.FillRatePct, roi, 1.0, "campaigns"))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorSpendEfficiency, Reason: "insufficient_sales"})
	}
	if d.PriceChangePct != nil {
		factors = append(factors, scoring.ComputeMarketTrend(*d.PriceChangePct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorMarketTrend, Reason: "no_market_data"})
	}
	return factors, gaps
}

func liquidationFactors(d *LiquidationFactorData) ([]scoring.Factor, []scoring.DataGap) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	factors = append(factors, scoring.ComputeCarryingCost(d.DaysHeld, 1.0, "purchase"))
	if d.CreditUtilPct != nil {
		factors = append(factors, scoring.ComputeCreditPressure(*d.CreditUtilPct, 1.0, "credit"))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorCreditPressure, Reason: "no_market_data"})
	}
	if d.PriceChangePct != nil {
		// Negate: a falling market (negative price change) increases liquidation urgency
		factors = append(factors, scoring.ComputeMarketTrend(-*d.PriceChangePct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorMarketTrend, Reason: "no_market_data"})
	}
	if d.SalesPerMonth != nil {
		factors = append(factors, scoring.ComputeLiquidity(*d.SalesPerMonth, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorLiquidity, Reason: "insufficient_sales"})
	}
	if d.CrackROI != nil && d.GradedROI != nil {
		factors = append(factors, scoring.ComputeCrackAdvantage(*d.CrackROI, *d.GradedROI, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorCrackAdvantage, Reason: "no_market_data"})
	}
	if d.ROIPct != nil {
		factors = append(factors, scoring.ComputeROIPotential(*d.ROIPct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorROIPotential, Reason: "no_market_data"})
	}
	if d.PSA10Pop != nil {
		factors = append(factors, scoring.ComputeScarcity(*d.PSA10Pop, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorScarcity, Reason: "no_population_data"})
	}
	return factors, gaps
}

func suggestionFactors(d *SuggestionFactorData) ([]scoring.Factor, []scoring.DataGap) {
	var factors []scoring.Factor
	var gaps []scoring.DataGap

	if d.ProjectedROIPct != nil {
		factors = append(factors, scoring.ComputeROIPotential(*d.ProjectedROIPct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorROIPotential, Reason: "no_market_data"})
	}
	factors = append(factors, scoring.ComputeCoverageImpact(d.FillsGap, d.OverlapCount, 1.0, "portfolio"))
	if d.Trend30dPct != nil {
		factors = append(factors, scoring.ComputeMarketAlignment(*d.Trend30dPct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorMarketAlignment, Reason: "no_market_data"})
	}
	if d.SalesPerMonth != nil {
		factors = append(factors, scoring.ComputeLiquidity(*d.SalesPerMonth, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorLiquidity, Reason: "insufficient_sales"})
	}
	if d.PriceChangePct != nil {
		factors = append(factors, scoring.ComputeMarketTrend(*d.PriceChangePct, d.PriceConfidence, d.MarketSource))
	} else {
		gaps = append(gaps, scoring.DataGap{FactorName: scoring.FactorMarketTrend, Reason: "no_market_data"})
	}
	return factors, gaps
}
