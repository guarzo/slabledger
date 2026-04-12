package inventory

import "context"

// AnalyticsRepository handles analytics query operations.
type AnalyticsRepository interface {
	GetCampaignPNL(ctx context.Context, campaignID string) (*CampaignPNL, error)
	GetPNLByChannel(ctx context.Context, campaignID string) ([]ChannelPNL, error)
	GetDailySpend(ctx context.Context, campaignID string, days int) ([]DailySpend, error)
	GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]DaysToSellBucket, error)
	GetPerformanceByGrade(ctx context.Context, campaignID string) ([]GradePerformance, error)
	GetPurchasesWithSales(ctx context.Context, campaignID string) ([]PurchaseWithSale, error)
	GetAllPurchasesWithSales(ctx context.Context, opts ...PurchaseFilterOpt) ([]PurchaseWithSale, error)
	GetPortfolioChannelVelocity(ctx context.Context) ([]ChannelVelocity, error)
	GetGlobalPNLByChannel(ctx context.Context) ([]ChannelPNL, error)
	GetDailyCapitalTimeSeries(ctx context.Context) ([]DailyCapitalPoint, error)
}
