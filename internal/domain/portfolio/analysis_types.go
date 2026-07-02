package portfolio

import "github.com/guarzo/slabledger/internal/domain/inventory"

// AnalysisResponse is the top-level response for the portfolio analysis endpoint.
type AnalysisResponse struct {
	GeneratedAt string             `json:"generatedAt"` // RFC3339
	Since       string             `json:"since,omitempty"`
	Campaigns   []CampaignAnalysis `json:"campaigns"`
	Deltas      SessionDeltas      `json:"deltas"`
}

// CampaignAnalysis holds all computed analytics for a single campaign.
type CampaignAnalysis struct {
	CampaignID     string           `json:"campaignId"`
	CampaignName   string           `json:"campaignName"`
	Phase          inventory.Phase  `json:"phase"`
	BuyTermsCLPct  float64          `json:"buyTermsCLPct"`
	BPCLAtBuy      BPCLStats        `json:"bpclAtBuy"`
	PNL            SplitPNL         `json:"pnl"`
	WeeklyFill     []WeeklyFill     `json:"weeklyFill"`
	InScopeByGrade []GradeScopeRow  `json:"inScopeByGrade"`
}

// BPCLStats summarises buy-price-as-a-fraction-of-CL-at-purchase metrics.
// Only purchases with a non-zero CLValueAtPurchaseCents snapshot contribute to
// DollarWeighted and MeanDriftPct; the rest are still counted in Total.
type BPCLStats struct {
	N              int     `json:"n"`              // purchases with a CL-at-buy snapshot
	Total          int     `json:"total"`          // all purchases in campaign
	CoveragePct    float64 `json:"coveragePct"`    // N/Total*100
	DollarWeighted float64 `json:"dollarWeighted"` // sum(buyCost)/sum(clAtBuy)
	MeanDriftPct   float64 `json:"meanDriftPct"`   // mean of (clNow-clAtBuy)/clAtBuy*100 over N
}

// SplitPNL separates realised P&L into discretionary and forced-liquidation buckets.
type SplitPNL struct {
	Discretionary PNLBlock `json:"discretionary"` // ForcedLiquidation == false
	Forced        PNLBlock `json:"forced"`        // ForcedLiquidation == true
}

// PNLBlock holds aggregated P&L metrics for one sale bucket.
type PNLBlock struct {
	SoldCount      int     `json:"soldCount"`
	RevenueCents   int     `json:"revenueCents"`
	NetProfitCents int     `json:"netProfitCents"`
	ROIPct         float64 `json:"roiPct"` // netProfit/(revenue-netProfit)*100; 0 when cost basis is 0
}

// WeeklyFill captures purchase volume for one ISO-week bucket within the trailing 8 weeks.
type WeeklyFill struct {
	WeekStart      string  `json:"weekStart"`      // Monday, YYYY-MM-DD
	Fills          int     `json:"fills"`
	SpendCents     int     `json:"spendCents"`
	CapCents       int     `json:"capCents"`       // DailySpendCapCents * 7
	UtilizationPct float64 `json:"utilizationPct"` // spend/cap*100; 0 when cap is 0
}

// GradeScopeRow holds in-scope metrics broken down by PSA grade.
type GradeScopeRow struct {
	Grade                   float64 `json:"grade"`
	N                       int     `json:"n"`
	DollarWeightedBPCLAtBuy float64 `json:"dollarWeightedBpclAtBuy"`
	SoldCount               int     `json:"soldCount"`      // discretionary only
	NetProfitCents          int     `json:"netProfitCents"` // discretionary only
}

// SessionDeltas surfaces what changed since the last analysis session.
type SessionDeltas struct {
	NewPurchases     int              `json:"newPurchases"`
	NewPurchaseCents int              `json:"newPurchaseCents"`
	NewSales         int              `json:"newSales"`
	NewSaleCents     int              `json:"newSaleCents"`
	CampaignsUpdated []string         `json:"campaignsUpdated"` // campaign names with UpdatedAt > since
	Invoices         []InvoiceSummary `json:"invoices"`         // invoices with InvoiceDate >= since
}

// InvoiceSummary is a lightweight invoice representation for the deltas block.
type InvoiceSummary struct {
	InvoiceDate string `json:"invoiceDate"`
	DueDate     string `json:"dueDate"`
	TotalCents  int    `json:"totalCents"`
	Status      string `json:"status"`
}
