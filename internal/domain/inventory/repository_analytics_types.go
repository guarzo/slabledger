package inventory

// GradePerformance contains P&L metrics for a specific PSA grade within a campaign.
//
// This stays in inventory rather than moving to internal/domain/tuning with the rest of
// the tuning cluster: it is a parameter type of AnalyticsRepository.GetPerformanceByGrade
// (repository_analytics.go), so letting tuning own it would make inventory import tuning —
// a cycle, since tuning already imports inventory.
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

// PurchaseWithSale joins a purchase with its optional sale.
//
// Stays in inventory for the same reason as GradePerformance — it is a return type of
// AnalyticsRepository — and additionally because the arbitrage and portfolio siblings
// consume it.
type PurchaseWithSale struct {
	Purchase Purchase
	Sale     *Sale
}
