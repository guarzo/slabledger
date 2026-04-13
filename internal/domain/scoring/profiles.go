package scoring

// Factor name constants used across profiles.
const (
	FactorMarketTrend     = "market_trend"
	FactorLiquidity       = "liquidity"
	FactorROIPotential    = "roi_potential"
	FactorScarcity        = "scarcity"
	FactorMarketAlignment = "market_alignment"
	FactorPortfolioFit    = "portfolio_fit"
	FactorGradeFit        = "grade_fit"
	FactorCapitalPressure = "capital_pressure"
	FactorCarryingCost    = "carrying_cost"
	FactorCrackAdvantage  = "crack_advantage"
	FactorSellThrough     = "sell_through"
	FactorSpendEfficiency = "spend_efficiency"
	FactorCoverageImpact  = "coverage_impact"
)

var PurchaseAssessmentProfile = WeightProfile{
	Name: "purchase_assessment",
	Weights: []FactorWeight{
		{Name: FactorROIPotential, Weight: 0.25},
		{Name: FactorMarketTrend, Weight: 0.20},
		{Name: FactorLiquidity, Weight: 0.20},
		{Name: FactorPortfolioFit, Weight: 0.15},
		{Name: FactorGradeFit, Weight: 0.10},
		{Name: FactorScarcity, Weight: 0.05},
		{Name: FactorMarketAlignment, Weight: 0.05},
	},
}

var CampaignAnalysisProfile = WeightProfile{
	Name: "campaign_analysis",
	Weights: []FactorWeight{
		{Name: FactorROIPotential, Weight: 0.25},
		{Name: FactorSellThrough, Weight: 0.25},
		{Name: FactorMarketAlignment, Weight: 0.20},
		{Name: FactorLiquidity, Weight: 0.10},
		{Name: FactorSpendEfficiency, Weight: 0.10},
		{Name: FactorMarketTrend, Weight: 0.10},
	},
}

var LiquidationProfile = WeightProfile{
	Name: "liquidation",
	Weights: []FactorWeight{
		{Name: FactorCarryingCost, Weight: 0.25},
		{Name: FactorCapitalPressure, Weight: 0.20},
		{Name: FactorMarketTrend, Weight: 0.20},
		{Name: FactorLiquidity, Weight: 0.15},
		{Name: FactorCrackAdvantage, Weight: 0.10},
		{Name: FactorROIPotential, Weight: 0.05},
		{Name: FactorScarcity, Weight: 0.05},
	},
}

var CampaignSuggestionsProfile = WeightProfile{
	Name: "campaign_suggestions",
	Weights: []FactorWeight{
		{Name: FactorROIPotential, Weight: 0.30},
		{Name: FactorCoverageImpact, Weight: 0.25},
		{Name: FactorMarketAlignment, Weight: 0.20},
		{Name: FactorLiquidity, Weight: 0.15},
		{Name: FactorMarketTrend, Weight: 0.10},
	},
}
