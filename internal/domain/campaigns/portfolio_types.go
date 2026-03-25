package campaigns

// SegmentPerformance holds aggregated P&L for a segment across campaigns.
type SegmentPerformance struct {
	Label             string      `json:"label"`
	Dimension         string      `json:"dimension"`
	PurchaseCount     int         `json:"purchaseCount"`
	SoldCount         int         `json:"soldCount"`
	SellThroughPct    float64     `json:"sellThroughPct"`
	AvgDaysToSell     float64     `json:"avgDaysToSell"`
	TotalSpendCents   int         `json:"totalSpendCents"`
	TotalRevenueCents int         `json:"totalRevenueCents"`
	TotalFeesCents    int         `json:"totalFeesCents"`
	NetProfitCents    int         `json:"netProfitCents"`
	ROI               float64     `json:"roi"`
	AvgBuyPctOfCL     float64     `json:"avgBuyPctOfCL"`
	AvgMarginPct      float64     `json:"avgMarginPct"`
	BestChannel       SaleChannel `json:"bestChannel,omitempty"`
	CampaignCount     int         `json:"campaignCount"`
	LatestSaleDate    string      `json:"latestSaleDate,omitempty"`
}

// CoverageGap identifies a profitable segment not covered by active campaigns.
type CoverageGap struct {
	Segment     SegmentPerformance `json:"segment"`
	Reason      string             `json:"reason"`
	Opportunity string             `json:"opportunity"`
}

// CampaignPNLBrief holds per-campaign ROI computed from cross-campaign data.
type CampaignPNLBrief struct {
	CampaignID    string  `json:"campaignId"`
	ROI           float64 `json:"roi"`
	SpendCents    int     `json:"spendCents"`
	ProfitCents   int     `json:"profitCents"`
	SoldCount     int     `json:"soldCount"`
	PurchaseCount int     `json:"purchaseCount"`
}

// PortfolioInsights is the top-level cross-campaign analytics response.
type PortfolioInsights struct {
	ByCharacter      []SegmentPerformance `json:"byCharacter"`
	ByGrade          []SegmentPerformance `json:"byGrade"`
	ByEra            []SegmentPerformance `json:"byEra"`
	ByPriceTier      []SegmentPerformance `json:"byPriceTier"`
	ByChannel        []ChannelPNL         `json:"byChannel"`
	ByCharacterGrade []SegmentPerformance `json:"byCharacterGrade"`
	CoverageGaps     []CoverageGap        `json:"coverageGaps"`
	CampaignMetrics  []CampaignPNLBrief   `json:"campaignMetrics,omitempty"`
	DataSummary      InsightsDataSummary  `json:"dataSummary"`
}

// InsightsDataSummary metadata about the cross-campaign dataset.
type InsightsDataSummary struct {
	TotalPurchases    int     `json:"totalPurchases"`
	TotalSales        int     `json:"totalSales"`
	CampaignsAnalyzed int     `json:"campaignsAnalyzed"`
	DateRange         string  `json:"dateRange"`
	OverallROI        float64 `json:"overallROI"`
}
