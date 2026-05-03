package inventory

// CampaignPNL contains overall profit/loss metrics for a campaign.
type CampaignPNL struct {
	CampaignID        string  `json:"campaignId"`
	TotalSpendCents   int     `json:"totalSpendCents"`   // Sum of buyCost + sourcingFee
	TotalRevenueCents int     `json:"totalRevenueCents"` // Sum of salePriceCents
	TotalFeesCents    int     `json:"totalFeesCents"`    // Sum of saleFeeCents
	NetProfitCents    int     `json:"netProfitCents"`    // Sum of netProfitCents
	ROI               float64 `json:"roi"`               // netProfit / totalSpend
	AvgDaysToSell     float64 `json:"avgDaysToSell"`
	TotalPurchases    int     `json:"totalPurchases"`
	TotalSold         int     `json:"totalSold"`
	TotalUnsold       int     `json:"totalUnsold"`
	SellThroughPct    float64 `json:"sellThroughPct"` // totalSold / totalPurchases
}

// ChannelPNL contains P&L broken down by sale channel.
type ChannelPNL struct {
	Channel        SaleChannel `json:"channel"`
	SaleCount      int         `json:"saleCount"`
	RevenueCents   int         `json:"revenueCents"`
	FeesCents      int         `json:"feesCents"`
	NetProfitCents int         `json:"netProfitCents"`
	AvgDaysToSell  float64     `json:"avgDaysToSell"`
}

// DailySpend holds actual spend vs cap for a given date.
type DailySpend struct {
	Date          string  `json:"date"`
	SpendCents    int     `json:"spendCents"`
	CapCents      int     `json:"capCents"`
	FillRatePct   float64 `json:"fillRatePct"` // spend / cap
	PurchaseCount int     `json:"purchaseCount"`
}

// DaysToSellBucket is a histogram bucket for days-to-sell distribution.
type DaysToSellBucket struct {
	Label string `json:"label"` // e.g. "0-7", "8-14"
	Min   int    `json:"min"`
	Max   int    `json:"max"`
	Count int    `json:"count"`
}

// AgingItem represents an unsold card with days held.
type AgingItem struct {
	Purchase              Purchase          `json:"purchase"`
	DaysHeld              int               `json:"daysHeld"`
	CampaignName          string            `json:"campaignName,omitempty"`
	Signal                *MarketSignal     `json:"signal,omitempty"`
	CurrentMarket         *MarketSnapshot   `json:"currentMarket,omitempty"`
	PriceAnomaly          bool              `json:"priceAnomaly,omitempty"`
	AnomalyReason         string            `json:"anomalyReason,omitempty"`
	HasOpenFlag           bool              `json:"hasOpenFlag,omitempty"`
	OpenFlagID            int64             `json:"openFlagId,omitempty"`
	RecommendedPriceCents int               `json:"recommendedPriceCents,omitempty"`
	RecommendedSource     string            `json:"recommendedSource,omitempty"`
	Signals               *InventorySignals `json:"signals,omitempty"`
	CompSummary           *CompSummary      `json:"compSummary,omitempty"`
}

// InventoryResult wraps an inventory listing with optional warnings
// for partial failures (e.g. flag data unavailable).
type InventoryResult struct {
	Items    []AgingItem `json:"items"`
	Warnings []string    `json:"warnings,omitempty"`
}

// InventorySignals contains procedural flags for an unsold card.
// Computed server-side from market data, aging, and profitability.
type InventorySignals struct {
	ProfitCaptureDeclining bool `json:"profitCaptureDeclining,omitempty"`
	ProfitCaptureSpike     bool `json:"profitCaptureSpike,omitempty"`
	CrackCandidate         bool `json:"crackCandidate,omitempty"`
	StaleListing           bool `json:"staleListing,omitempty"`
	DeepStale              bool `json:"deepStale,omitempty"`
	CutLoss                bool `json:"cutLoss,omitempty"`
}

// HasAnySignal returns true if any signal flag is set.
func (s *InventorySignals) HasAnySignal() bool {
	if s == nil {
		return false
	}
	return s.ProfitCaptureDeclining || s.ProfitCaptureSpike ||
		s.CrackCandidate || s.StaleListing || s.DeepStale || s.CutLoss
}

// SellSheet contains data for a printable sell sheet.
type SellSheet struct {
	GeneratedAt  string          `json:"generatedAt"`
	CampaignName string          `json:"campaignName"`
	Items        []SellSheetItem `json:"items"`
	Totals       SellSheetTotals `json:"totals"`
}

// SellSheetItem contains sell sheet data for a single card.
type SellSheetItem struct {
	PurchaseID            string            `json:"purchaseId,omitempty"`
	CampaignName          string            `json:"campaignName,omitempty"`
	CertNumber            string            `json:"certNumber"`
	CardName              string            `json:"cardName"`
	SetName               string            `json:"setName,omitempty"`
	CardNumber            string            `json:"cardNumber,omitempty"`
	Grade                 float64           `json:"grade"`
	Grader                string            `json:"grader,omitempty"`
	CardYear              string            `json:"cardYear,omitempty"`
	Population            int               `json:"population,omitempty"`
	BuyCostCents          int               `json:"buyCostCents"`
	CostBasisCents        int               `json:"costBasisCents"`
	CLValueCents          int               `json:"clValueCents"`
	CurrentMarket         *MarketSnapshot   `json:"currentMarket,omitempty"`
	Recommendation        string            `json:"recommendation"`
	TargetSellPrice       int               `json:"targetSellPrice"`
	MinimumAcceptPrice    int               `json:"minimumAcceptPrice"`
	PriceLookupError      string            `json:"priceLookupError,omitempty"`
	RecommendedChannel    SaleChannel       `json:"recommendedChannel,omitempty"`
	ChannelLabel          string            `json:"channelLabel,omitempty"`
	OverridePriceCents    int               `json:"overridePriceCents,omitempty"`
	OverrideSource        OverrideSource    `json:"overrideSource,omitempty"`
	IsOverridden          bool              `json:"isOverridden,omitempty"`
	ComputedPriceCents    int               `json:"computedPriceCents,omitempty"`
	AISuggestedPriceCents int               `json:"aiSuggestedPriceCents,omitempty"`
	AISuggestedAt         string            `json:"aiSuggestedAt,omitempty"`
	Signals               *InventorySignals `json:"signals,omitempty"`
	PSAShipDate           string            `json:"psaShipDate,omitempty"`
}

// SellSheetTotals contains aggregate sell sheet metrics.
type SellSheetTotals struct {
	TotalCostBasis       int `json:"totalCostBasis"`
	TotalExpectedRevenue int `json:"totalExpectedRevenue"`
	TotalProjectedProfit int `json:"totalProjectedProfit"`
	ItemCount            int `json:"itemCount"`
	SkippedItems         int `json:"skippedItems"`
}

// DailyCapitalPoint represents a single day in the capital deployment timeline.
type DailyCapitalPoint struct {
	Date                    string `json:"date"`
	CumulativeSpendCents    int    `json:"cumulativeSpendCents"`
	CumulativeRecoveryCents int    `json:"cumulativeRecoveryCents"`
	OutstandingCents        int    `json:"outstandingCents"`
}

// CapitalTimeline contains the full capital deployment timeline with invoice markers.
type CapitalTimeline struct {
	DataPoints   []DailyCapitalPoint `json:"dataPoints"`
	InvoiceDates []string            `json:"invoiceDates"`
}

// WeeklyReviewSummary contains data for the Monday review cadence.
type WeeklyReviewSummary struct {
	WeekStart            string            `json:"weekStart"`
	WeekEnd              string            `json:"weekEnd"`
	PurchasesThisWeek    int               `json:"purchasesThisWeek"`
	PurchasesLastWeek    int               `json:"purchasesLastWeek"`
	SpendThisWeekCents   int               `json:"spendThisWeekCents"`
	SpendLastWeekCents   int               `json:"spendLastWeekCents"`
	SalesThisWeek        int               `json:"salesThisWeek"`
	SalesLastWeek        int               `json:"salesLastWeek"`
	RevenueThisWeekCents int               `json:"revenueThisWeekCents"`
	RevenueLastWeekCents int               `json:"revenueLastWeekCents"`
	ProfitThisWeekCents  int               `json:"profitThisWeekCents"`
	ProfitLastWeekCents  int               `json:"profitLastWeekCents"`
	ByChannel            []ChannelPNL      `json:"byChannel"`
	WeeksToCover         float64           `json:"weeksToCover"`
	DaysIntoWeek         int               `json:"daysIntoWeek"` // 0=Sunday, 1=Monday … 6=Saturday
	TopPerformers        []WeeklyPerformer `json:"topPerformers"`
	BottomPerformers     []WeeklyPerformer `json:"bottomPerformers"`
}

// WeeklyPerformer is a card that performed notably during the review period.
type WeeklyPerformer struct {
	CardName    string  `json:"cardName"`
	CertNumber  string  `json:"certNumber"`
	Grade       float64 `json:"grade"`
	ProfitCents int     `json:"profitCents"`
	Channel     string  `json:"channel"`
	DaysToSell  int     `json:"daysToSell"`
}

// PriceOverrideStats contains aggregate statistics about price overrides and AI suggestions.
type PriceOverrideStats struct {
	TotalUnsold          int     `json:"totalUnsold"`
	OverrideCount        int     `json:"overrideCount"`
	ManualCount          int     `json:"manualCount"`
	CostMarkupCount      int     `json:"costMarkupCount"`
	AIAcceptedCount      int     `json:"aiAcceptedCount"`
	PendingSuggestions   int     `json:"pendingSuggestions"`
	OverrideTotalCents   int     `json:"-"`
	SuggestionTotalCents int     `json:"-"`
	OverrideTotalUsd     float64 `json:"overrideTotalUsd"`
	SuggestionTotalUsd   float64 `json:"suggestionTotalUsd"`
}

// MarketSignal indicates whether the market is trending above or below CL valuations.
type MarketSignal struct {
	CardName       string  `json:"cardName"`
	CertNumber     string  `json:"certNumber"`
	Grade          float64 `json:"grade"`
	CLValueCents   int     `json:"clValueCents"`   // CL valuation at purchase
	LastSoldCents  int     `json:"lastSoldCents"`  // Most recent sold price
	DeltaPct       float64 `json:"deltaPct"`       // (lastSold - clValue) / clValue
	Direction      string  `json:"direction"`      // "rising", "falling", "stable"
	Recommendation string  `json:"recommendation"` // Sell-channel suggestion
}
