package inventory

// GradePerformance contains P&L metrics for a specific PSA grade within a campaign.
type GradePerformance struct {
	Grade             float64 `json:"grade"`
	PurchaseCount     int     `json:"purchaseCount"`
	SoldCount         int     `json:"soldCount"`
	SellThroughPct    float64 `json:"sellThroughPct"`
	AvgDaysToSell     float64 `json:"avgDaysToSell"`
	TotalSpendCents   int     `json:"totalSpendCents"`
	TotalRevenueCents int     `json:"totalRevenueCents"`
	TotalFeesCents    int     `json:"totalFeesCents"`
	NetProfitCents    int     `json:"netProfitCents"`
	ROI               float64 `json:"roi"`
	AvgBuyPctOfCL     float64 `json:"avgBuyPctOfCL"`
	RoiStddev         float64 `json:"roiStddev"` // population stddev of per-sale ROIs; 0 if < 2 sales
	CV                float64 `json:"cv"`        // coefficient of variation = RoiStddev / |ROI|; 0 if ROI==0
}

// PriceTierPerformance contains P&L metrics for a cost basis price tier.
type PriceTierPerformance struct {
	TierLabel         string  `json:"tierLabel"`
	TierMinCents      int     `json:"tierMinCents"`
	TierMaxCents      int     `json:"tierMaxCents"`
	PurchaseCount     int     `json:"purchaseCount"`
	SoldCount         int     `json:"soldCount"`
	SellThroughPct    float64 `json:"sellThroughPct"`
	AvgDaysToSell     float64 `json:"avgDaysToSell"`
	TotalSpendCents   int     `json:"totalSpendCents"`
	TotalRevenueCents int     `json:"totalRevenueCents"`
	TotalFeesCents    int     `json:"totalFeesCents"`
	NetProfitCents    int     `json:"netProfitCents"`
	ROI               float64 `json:"roi"`
	AvgBuyPctOfCL     float64 `json:"avgBuyPctOfCL"`
	RoiStddev         float64 `json:"roiStddev"` // population stddev of per-sale ROIs; 0 if < 2 sales
	CV                float64 `json:"cv"`        // coefficient of variation = RoiStddev / |ROI|; 0 if ROI==0
}

// CardPerformance contains performance data for a single purchase with current market context.
type CardPerformance struct {
	Purchase      Purchase        `json:"purchase"`
	BuyPctOfCL    float64         `json:"buyPctOfCL"`
	Sale          *Sale           `json:"sale,omitempty"`
	CurrentMarket *MarketSnapshot `json:"currentMarket,omitempty"`
	RealizedPnL   int             `json:"realizedPnL"`
	UnrealizedPnL int             `json:"unrealizedPnL"`
}

// BuyThresholdDataPoint is a single observation for the empirical optimal buy threshold chart.
type BuyThresholdDataPoint struct {
	PurchaseID  string  `json:"purchaseId"`
	BuyPctOfCL  float64 `json:"buyPctOfCL"`
	ROI         float64 `json:"roi"`
	Sold        bool    `json:"sold"`
	CostBasis   int     `json:"costBasisCents"`
	ProfitCents int     `json:"profitCents"`
}

// ThresholdBucket aggregates ROI for purchases within a CL% range.
type ThresholdBucket struct {
	RangeLabel  string  `json:"rangeLabel"`
	RangeMinPct float64 `json:"rangeMinPct"`
	RangeMaxPct float64 `json:"rangeMaxPct"`
	Count       int     `json:"count"`
	AvgROI      float64 `json:"avgROI"`
	MedianROI   float64 `json:"medianROI"`
	TotalProfit int     `json:"totalProfitCents"`
}

// BuyThresholdAnalysis contains the empirical optimal buy threshold computation.
type BuyThresholdAnalysis struct {
	DataPoints  []BuyThresholdDataPoint `json:"dataPoints"`
	OptimalPct  float64                 `json:"optimalPct"`
	CurrentPct  float64                 `json:"currentPct"`
	BucketedROI []ThresholdBucket       `json:"bucketedROI"`
	SampleSize  int                     `json:"sampleSize"`
	Confidence  int                     `json:"confidence"`
}

// MarketAlignment assesses how the campaign's target segment is performing in the current market.
type MarketAlignment struct {
	AvgTrend30d       float64 `json:"avgTrend30d"`
	AvgTrend90d       float64 `json:"avgTrend90d"`
	AvgVolatility     float64 `json:"avgVolatility"`
	AvgSalesLast30d   float64 `json:"avgSalesLast30d"`
	AvgSnapshotDrift  float64 `json:"avgSnapshotDrift"`
	AppreciatingCount int     `json:"appreciatingCount"`
	DepreciatingCount int     `json:"depreciatingCount"`
	StableCount       int     `json:"stableCount"`
	Signal            string  `json:"signal"`
	SignalReason      string  `json:"signalReason"`
	SampleSize        int     `json:"sampleSize"`
}

// TuningRecommendation is a specific, actionable suggestion for adjusting a campaign parameter.
type TuningRecommendation struct {
	Parameter    string `json:"parameter"`
	CurrentVal   string `json:"currentVal"`
	SuggestedVal string `json:"suggestedVal"`
	Reasoning    string `json:"reasoning"`
	Impact       string `json:"impact"`
	Confidence   int    `json:"confidence"` // Data point count driving this recommendation
	DataPoints   int    `json:"dataPoints"`
}

// TuningResponse is the top-level API response for the tuning endpoint.
type TuningResponse struct {
	CampaignID       string                 `json:"campaignId"`
	CampaignName     string                 `json:"campaignName"`
	ByGrade          []GradePerformance     `json:"byGrade"`
	ByFixedTier      []PriceTierPerformance `json:"byFixedTier"`
	ByRelativeTier   []PriceTierPerformance `json:"byRelativeTier"`
	TopPerformers    []CardPerformance      `json:"topPerformers"`
	BottomPerformers []CardPerformance      `json:"bottomPerformers"`
	BuyThreshold     *BuyThresholdAnalysis  `json:"buyThreshold,omitempty"`
	MarketAlignment  *MarketAlignment       `json:"marketAlignment,omitempty"`
	Recommendations  []TuningRecommendation `json:"recommendations"`
}

// PurchaseWithSale joins a purchase with its optional sale for tuning analysis.
type PurchaseWithSale struct {
	Purchase Purchase
	Sale     *Sale
}
