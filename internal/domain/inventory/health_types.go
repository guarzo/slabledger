package inventory

// ChannelVelocity holds cross-campaign recovery velocity stats for a sale channel.
type ChannelVelocity struct {
	Channel       SaleChannel `json:"channel"`
	SaleCount     int         `json:"saleCount"`
	AvgDaysToSell float64     `json:"avgDaysToSell"`
	RevenueCents  int         `json:"revenueCents"`
}

// PortfolioHealth represents cross-campaign health assessment.
type PortfolioHealth struct {
	Campaigns      []CampaignHealth `json:"campaigns"`
	TotalDeployed  int              `json:"totalDeployedCents"`
	TotalRecovered int              `json:"totalRecoveredCents"`
	TotalAtRisk    int              `json:"totalAtRiskCents"`
	OverallROI     float64          `json:"overallROI"`
	RealizedROI    float64          `json:"realizedROI"`
}

// CampaignHealth represents health status for a single campaign.
type CampaignHealth struct {
	CampaignID     string  `json:"campaignId"`
	CampaignName   string  `json:"campaignName"`
	Phase          Phase   `json:"phase"`
	ROI            float64 `json:"roi"`
	SellThroughPct float64 `json:"sellThroughPct"`
	AvgDaysToSell  float64 `json:"avgDaysToSell"`
	TotalPurchases int     `json:"totalPurchases"`
	TotalUnsold    int     `json:"totalUnsold"`
	CapitalAtRisk  int     `json:"capitalAtRiskCents"`
	HealthStatus   string  `json:"healthStatus"` // "healthy", "warning", "critical"
	HealthReason   string  `json:"healthReason"`

	// Liquidation awareness — distinguishes "marketplace margin broken" from
	// "we forced cards into low-margin inperson/cardshow sales to cover an invoice".
	LiquidationLossCents int     `json:"liquidationLossCents"` // sum of negative net profit on inperson+cardshow sales; always ≤ 0
	LiquidationSaleCount int     `json:"liquidationSaleCount"` // count of sales contributing to the loss
	EbayChannelMarginPct float64 `json:"ebayChannelMarginPct"` // net profit / revenue on eBay + TCGPlayer sales combined; 0 if no marketplace sales. JSON field name retained for frontend compatibility.

	// In-hand vs in-transit breakdown (received_at IS NOT NULL = in hand)
	InHandUnsoldCount     int `json:"inHandUnsoldCount"`     // unsold cards physically received
	InHandCapitalCents    int `json:"inHandCapitalCents"`    // cost of in-hand unsold cards
	InTransitUnsoldCount  int `json:"inTransitUnsoldCount"`  // unsold cards still at PSA
	InTransitCapitalCents int `json:"inTransitCapitalCents"` // cost of in-transit unsold cards
}
