package mocks

import (
	"context"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- AnalyticsRepository ---

func (m *InMemoryCampaignStore) GetCampaignPNL(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error) {
	if m.GetCampaignPNLFn != nil {
		return m.GetCampaignPNLFn(ctx, campaignID)
	}
	if pnl, ok := m.PNLData[campaignID]; ok {
		return pnl, nil
	}
	return &inventory.CampaignPNL{CampaignID: campaignID}, nil
}

func (m *InMemoryCampaignStore) GetPNLByChannel(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error) {
	if m.GetPNLByChannelFn != nil {
		return m.GetPNLByChannelFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) GetDailySpend(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error) {
	if m.GetDailySpendFn != nil {
		return m.GetDailySpendFn(ctx, campaignID, days)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error) {
	if m.GetDaysToSellDistributionFn != nil {
		return m.GetDaysToSellDistributionFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) GetPerformanceByGrade(ctx context.Context, campaignID string) ([]inventory.GradePerformance, error) {
	if m.GetPerformanceByGradeFn != nil {
		return m.GetPerformanceByGradeFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *InMemoryCampaignStore) GetPurchasesWithSales(ctx context.Context, campaignID string) ([]inventory.PurchaseWithSale, error) {
	if m.GetPurchasesWithSalesFn != nil {
		return m.GetPurchasesWithSalesFn(ctx, campaignID)
	}
	ids := make([]string, 0, len(m.Purchases))
	for id, p := range m.Purchases {
		if p.CampaignID == campaignID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var result []inventory.PurchaseWithSale
	for _, id := range ids {
		p := m.Purchases[id]
		pws := inventory.PurchaseWithSale{Purchase: *p}
		for _, s := range m.Sales {
			if s.PurchaseID == p.ID {
				pws.Sale = s
				break
			}
		}
		result = append(result, pws)
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetAllPurchasesWithSales(ctx context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
	if m.GetAllPurchasesWithSalesFn != nil {
		return m.GetAllPurchasesWithSalesFn(ctx, opts...)
	}
	var f inventory.PurchaseFilter
	for _, o := range opts {
		o(&f)
	}

	// Collect purchase IDs in sorted order for deterministic output.
	ids := make([]string, 0, len(m.Purchases))
	for id := range m.Purchases {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var result []inventory.PurchaseWithSale
	for _, id := range ids {
		p := m.Purchases[id]
		if f.SinceDate != "" && p.PurchaseDate < f.SinceDate {
			continue
		}
		if f.ExcludeArchived {
			if c, ok := m.Campaigns[p.CampaignID]; ok && c.Phase == inventory.PhaseClosed {
				continue
			}
		}
		pws := inventory.PurchaseWithSale{Purchase: *p}
		for _, s := range m.Sales {
			if s.PurchaseID == p.ID {
				pws.Sale = s
				break
			}
		}
		result = append(result, pws)
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetPortfolioChannelVelocity(_ context.Context) ([]inventory.ChannelVelocity, error) {
	if m.ChannelVelocity != nil {
		return m.ChannelVelocity, nil
	}
	return []inventory.ChannelVelocity{}, nil
}

func (m *InMemoryCampaignStore) GetGlobalPNLByChannel(ctx context.Context) ([]inventory.ChannelPNL, error) {
	if m.GetGlobalPNLByChannelFn != nil {
		return m.GetGlobalPNLByChannelFn(ctx)
	}
	return []inventory.ChannelPNL{}, nil
}

func (m *InMemoryCampaignStore) GetDailyCapitalTimeSeries(_ context.Context) ([]inventory.DailyCapitalPoint, error) {
	return []inventory.DailyCapitalPoint{}, nil
}
