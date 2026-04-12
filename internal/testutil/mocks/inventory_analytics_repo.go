package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// AnalyticsRepositoryMock implements inventory.AnalyticsRepository with Fn-field pattern.
type AnalyticsRepositoryMock struct {
	GetCampaignPNLFn              func(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error)
	GetPNLByChannelFn             func(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error)
	GetDailySpendFn               func(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error)
	GetDaysToSellDistributionFn   func(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error)
	GetPerformanceByGradeFn       func(ctx context.Context, campaignID string) ([]inventory.GradePerformance, error)
	GetPurchasesWithSalesFn       func(ctx context.Context, campaignID string) ([]inventory.PurchaseWithSale, error)
	GetAllPurchasesWithSalesFn    func(ctx context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error)
	GetPortfolioChannelVelocityFn func(ctx context.Context) ([]inventory.ChannelVelocity, error)
	GetGlobalPNLByChannelFn       func(ctx context.Context) ([]inventory.ChannelPNL, error)
	GetDailyCapitalTimeSeriesFn   func(ctx context.Context) ([]inventory.DailyCapitalPoint, error)
}

var _ inventory.AnalyticsRepository = (*AnalyticsRepositoryMock)(nil)

func (m *AnalyticsRepositoryMock) GetCampaignPNL(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error) {
	if m.GetCampaignPNLFn != nil {
		return m.GetCampaignPNLFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *AnalyticsRepositoryMock) GetPNLByChannel(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error) {
	if m.GetPNLByChannelFn != nil {
		return m.GetPNLByChannelFn(ctx, campaignID)
	}
	return []inventory.ChannelPNL{}, nil
}

func (m *AnalyticsRepositoryMock) GetDailySpend(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error) {
	if m.GetDailySpendFn != nil {
		return m.GetDailySpendFn(ctx, campaignID, days)
	}
	return []inventory.DailySpend{}, nil
}

func (m *AnalyticsRepositoryMock) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error) {
	if m.GetDaysToSellDistributionFn != nil {
		return m.GetDaysToSellDistributionFn(ctx, campaignID)
	}
	return []inventory.DaysToSellBucket{}, nil
}

func (m *AnalyticsRepositoryMock) GetPerformanceByGrade(ctx context.Context, campaignID string) ([]inventory.GradePerformance, error) {
	if m.GetPerformanceByGradeFn != nil {
		return m.GetPerformanceByGradeFn(ctx, campaignID)
	}
	return []inventory.GradePerformance{}, nil
}

func (m *AnalyticsRepositoryMock) GetPurchasesWithSales(ctx context.Context, campaignID string) ([]inventory.PurchaseWithSale, error) {
	if m.GetPurchasesWithSalesFn != nil {
		return m.GetPurchasesWithSalesFn(ctx, campaignID)
	}
	return []inventory.PurchaseWithSale{}, nil
}

func (m *AnalyticsRepositoryMock) GetAllPurchasesWithSales(ctx context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
	if m.GetAllPurchasesWithSalesFn != nil {
		return m.GetAllPurchasesWithSalesFn(ctx, opts...)
	}
	return []inventory.PurchaseWithSale{}, nil
}

func (m *AnalyticsRepositoryMock) GetPortfolioChannelVelocity(ctx context.Context) ([]inventory.ChannelVelocity, error) {
	if m.GetPortfolioChannelVelocityFn != nil {
		return m.GetPortfolioChannelVelocityFn(ctx)
	}
	return []inventory.ChannelVelocity{}, nil
}

func (m *AnalyticsRepositoryMock) GetGlobalPNLByChannel(ctx context.Context) ([]inventory.ChannelPNL, error) {
	if m.GetGlobalPNLByChannelFn != nil {
		return m.GetGlobalPNLByChannelFn(ctx)
	}
	return []inventory.ChannelPNL{}, nil
}

func (m *AnalyticsRepositoryMock) GetDailyCapitalTimeSeries(ctx context.Context) ([]inventory.DailyCapitalPoint, error) {
	if m.GetDailyCapitalTimeSeriesFn != nil {
		return m.GetDailyCapitalTimeSeriesFn(ctx)
	}
	return []inventory.DailyCapitalPoint{}, nil
}
