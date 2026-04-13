package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// MockAnalyticsService is a test double for inventory.AnalyticsService.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockAnalyticsService{
//	    GetCampaignPNLFn: func(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error) {
//	        return &inventory.CampaignPNL{CampaignID: campaignID}, nil
//	    },
//	}
type MockAnalyticsService struct {
	GetCampaignPNLFn            func(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error)
	GetPNLByChannelFn           func(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error)
	GetDailySpendFn             func(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error)
	GetDaysToSellDistributionFn func(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error)
	GetInventoryAgingFn         func(ctx context.Context, campaignID string) (*inventory.InventoryResult, error)
	GetGlobalInventoryAgingFn   func(ctx context.Context) (*inventory.InventoryResult, error)
	GetFlaggedInventoryFn       func(ctx context.Context) ([]inventory.AgingItem, error)
	RefreshCrackCandidatesFn    func(ctx context.Context) error
}

var _ inventory.AnalyticsService = (*MockAnalyticsService)(nil)

func (m *MockAnalyticsService) GetCampaignPNL(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error) {
	if m.GetCampaignPNLFn != nil {
		return m.GetCampaignPNLFn(ctx, campaignID)
	}
	return &inventory.CampaignPNL{CampaignID: campaignID}, nil
}

func (m *MockAnalyticsService) GetPNLByChannel(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error) {
	if m.GetPNLByChannelFn != nil {
		return m.GetPNLByChannelFn(ctx, campaignID)
	}
	return []inventory.ChannelPNL{}, nil
}

func (m *MockAnalyticsService) GetDailySpend(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error) {
	if m.GetDailySpendFn != nil {
		return m.GetDailySpendFn(ctx, campaignID, days)
	}
	return []inventory.DailySpend{}, nil
}

func (m *MockAnalyticsService) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error) {
	if m.GetDaysToSellDistributionFn != nil {
		return m.GetDaysToSellDistributionFn(ctx, campaignID)
	}
	return []inventory.DaysToSellBucket{}, nil
}

func (m *MockAnalyticsService) GetInventoryAging(ctx context.Context, campaignID string) (*inventory.InventoryResult, error) {
	if m.GetInventoryAgingFn != nil {
		return m.GetInventoryAgingFn(ctx, campaignID)
	}
	return &inventory.InventoryResult{Items: []inventory.AgingItem{}}, nil
}

func (m *MockAnalyticsService) GetGlobalInventoryAging(ctx context.Context) (*inventory.InventoryResult, error) {
	if m.GetGlobalInventoryAgingFn != nil {
		return m.GetGlobalInventoryAgingFn(ctx)
	}
	return &inventory.InventoryResult{Items: []inventory.AgingItem{}}, nil
}

func (m *MockAnalyticsService) GetFlaggedInventory(ctx context.Context) ([]inventory.AgingItem, error) {
	if m.GetFlaggedInventoryFn != nil {
		return m.GetFlaggedInventoryFn(ctx)
	}
	return nil, nil
}

func (m *MockAnalyticsService) RefreshCrackCandidates(ctx context.Context) error {
	if m.RefreshCrackCandidatesFn != nil {
		return m.RefreshCrackCandidatesFn(ctx)
	}
	return nil
}
