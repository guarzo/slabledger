package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// MockAnalyticsService is a test double for campaigns.AnalyticsService.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockAnalyticsService{
//	    GetCampaignPNLFn: func(ctx context.Context, campaignID string) (*campaigns.CampaignPNL, error) {
//	        return &campaigns.CampaignPNL{CampaignID: campaignID}, nil
//	    },
//	}
type MockAnalyticsService struct {
	GetCampaignPNLFn              func(ctx context.Context, campaignID string) (*campaigns.CampaignPNL, error)
	GetPNLByChannelFn             func(ctx context.Context, campaignID string) ([]campaigns.ChannelPNL, error)
	GetDailySpendFn               func(ctx context.Context, campaignID string, days int) ([]campaigns.DailySpend, error)
	GetDaysToSellDistributionFn   func(ctx context.Context, campaignID string) ([]campaigns.DaysToSellBucket, error)
	GetInventoryAgingFn           func(ctx context.Context, campaignID string) (*campaigns.InventoryResult, error)
	GetGlobalInventoryAgingFn     func(ctx context.Context) (*campaigns.InventoryResult, error)
	GetFlaggedInventoryFn         func(ctx context.Context) ([]campaigns.AgingItem, error)
	GetCampaignTuningFn           func(ctx context.Context, campaignID string) (*campaigns.TuningResponse, error)
	GetPortfolioHealthFn          func(ctx context.Context) (*campaigns.PortfolioHealth, error)
	GetPortfolioChannelVelocityFn func(ctx context.Context) ([]campaigns.ChannelVelocity, error)
	GetPortfolioInsightsFn        func(ctx context.Context) (*campaigns.PortfolioInsights, error)
	GetCampaignSuggestionsFn      func(ctx context.Context) (*campaigns.SuggestionsResponse, error)
	GetCapitalTimelineFn          func(ctx context.Context) (*campaigns.CapitalTimeline, error)
	GetWeeklyReviewSummaryFn      func(ctx context.Context) (*campaigns.WeeklyReviewSummary, error)
	GetCrackCandidatesFn          func(ctx context.Context, campaignID string) ([]campaigns.CrackAnalysis, error)
	GetCrackOpportunitiesFn       func(ctx context.Context) ([]campaigns.CrackAnalysis, error)
	GetAcquisitionTargetsFn       func(ctx context.Context) ([]campaigns.AcquisitionOpportunity, error)
	GetActivationChecklistFn      func(ctx context.Context, campaignID string) (*campaigns.ActivationChecklist, error)
	GetExpectedValuesFn           func(ctx context.Context, campaignID string) (*campaigns.EVPortfolio, error)
	EvaluatePurchaseFn            func(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*campaigns.ExpectedValue, error)
	RunProjectionFn               func(ctx context.Context, campaignID string) (*campaigns.MonteCarloComparison, error)
	GenerateSellSheetFn           func(ctx context.Context, campaignID string, purchaseIDs []string) (*campaigns.SellSheet, error)
	GenerateGlobalSellSheetFn     func(ctx context.Context) (*campaigns.SellSheet, error)
	GenerateSelectedSellSheetFn   func(ctx context.Context, purchaseIDs []string) (*campaigns.SellSheet, error)
	ListEbayExportItemsFn         func(ctx context.Context, flaggedOnly bool) (*campaigns.EbayExportListResponse, error)
	GenerateEbayCSVFn             func(ctx context.Context, items []campaigns.EbayExportGenerateItem) ([]byte, error)
	MatchShopifyPricesFn          func(ctx context.Context, items []campaigns.ShopifyPriceSyncItem) (*campaigns.ShopifyPriceSyncResponse, error)
}

var _ campaigns.AnalyticsService = (*MockAnalyticsService)(nil)

func (m *MockAnalyticsService) GetCampaignPNL(ctx context.Context, campaignID string) (*campaigns.CampaignPNL, error) {
	if m.GetCampaignPNLFn != nil {
		return m.GetCampaignPNLFn(ctx, campaignID)
	}
	return &campaigns.CampaignPNL{CampaignID: campaignID}, nil
}

func (m *MockAnalyticsService) GetPNLByChannel(ctx context.Context, campaignID string) ([]campaigns.ChannelPNL, error) {
	if m.GetPNLByChannelFn != nil {
		return m.GetPNLByChannelFn(ctx, campaignID)
	}
	return []campaigns.ChannelPNL{}, nil
}

func (m *MockAnalyticsService) GetDailySpend(ctx context.Context, campaignID string, days int) ([]campaigns.DailySpend, error) {
	if m.GetDailySpendFn != nil {
		return m.GetDailySpendFn(ctx, campaignID, days)
	}
	return []campaigns.DailySpend{}, nil
}

func (m *MockAnalyticsService) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]campaigns.DaysToSellBucket, error) {
	if m.GetDaysToSellDistributionFn != nil {
		return m.GetDaysToSellDistributionFn(ctx, campaignID)
	}
	return []campaigns.DaysToSellBucket{}, nil
}

func (m *MockAnalyticsService) GetInventoryAging(ctx context.Context, campaignID string) (*campaigns.InventoryResult, error) {
	if m.GetInventoryAgingFn != nil {
		return m.GetInventoryAgingFn(ctx, campaignID)
	}
	return &campaigns.InventoryResult{Items: []campaigns.AgingItem{}}, nil
}

func (m *MockAnalyticsService) GetGlobalInventoryAging(ctx context.Context) (*campaigns.InventoryResult, error) {
	if m.GetGlobalInventoryAgingFn != nil {
		return m.GetGlobalInventoryAgingFn(ctx)
	}
	return &campaigns.InventoryResult{Items: []campaigns.AgingItem{}}, nil
}

func (m *MockAnalyticsService) GetFlaggedInventory(ctx context.Context) ([]campaigns.AgingItem, error) {
	if m.GetFlaggedInventoryFn != nil {
		return m.GetFlaggedInventoryFn(ctx)
	}
	return []campaigns.AgingItem{}, nil
}

func (m *MockAnalyticsService) GetCampaignTuning(ctx context.Context, campaignID string) (*campaigns.TuningResponse, error) {
	if m.GetCampaignTuningFn != nil {
		return m.GetCampaignTuningFn(ctx, campaignID)
	}
	return &campaigns.TuningResponse{CampaignID: campaignID}, nil
}

func (m *MockAnalyticsService) GetPortfolioHealth(ctx context.Context) (*campaigns.PortfolioHealth, error) {
	if m.GetPortfolioHealthFn != nil {
		return m.GetPortfolioHealthFn(ctx)
	}
	return &campaigns.PortfolioHealth{}, nil
}

func (m *MockAnalyticsService) GetPortfolioChannelVelocity(ctx context.Context) ([]campaigns.ChannelVelocity, error) {
	if m.GetPortfolioChannelVelocityFn != nil {
		return m.GetPortfolioChannelVelocityFn(ctx)
	}
	return []campaigns.ChannelVelocity{}, nil
}

func (m *MockAnalyticsService) GetPortfolioInsights(ctx context.Context) (*campaigns.PortfolioInsights, error) {
	if m.GetPortfolioInsightsFn != nil {
		return m.GetPortfolioInsightsFn(ctx)
	}
	return &campaigns.PortfolioInsights{}, nil
}

func (m *MockAnalyticsService) GetCampaignSuggestions(ctx context.Context) (*campaigns.SuggestionsResponse, error) {
	if m.GetCampaignSuggestionsFn != nil {
		return m.GetCampaignSuggestionsFn(ctx)
	}
	return &campaigns.SuggestionsResponse{}, nil
}

func (m *MockAnalyticsService) GetCapitalTimeline(ctx context.Context) (*campaigns.CapitalTimeline, error) {
	if m.GetCapitalTimelineFn != nil {
		return m.GetCapitalTimelineFn(ctx)
	}
	return &campaigns.CapitalTimeline{}, nil
}

func (m *MockAnalyticsService) GetWeeklyReviewSummary(ctx context.Context) (*campaigns.WeeklyReviewSummary, error) {
	if m.GetWeeklyReviewSummaryFn != nil {
		return m.GetWeeklyReviewSummaryFn(ctx)
	}
	return &campaigns.WeeklyReviewSummary{}, nil
}

func (m *MockAnalyticsService) GetCrackCandidates(ctx context.Context, campaignID string) ([]campaigns.CrackAnalysis, error) {
	if m.GetCrackCandidatesFn != nil {
		return m.GetCrackCandidatesFn(ctx, campaignID)
	}
	return []campaigns.CrackAnalysis{}, nil
}

func (m *MockAnalyticsService) GetCrackOpportunities(ctx context.Context) ([]campaigns.CrackAnalysis, error) {
	if m.GetCrackOpportunitiesFn != nil {
		return m.GetCrackOpportunitiesFn(ctx)
	}
	return []campaigns.CrackAnalysis{}, nil
}

func (m *MockAnalyticsService) GetAcquisitionTargets(ctx context.Context) ([]campaigns.AcquisitionOpportunity, error) {
	if m.GetAcquisitionTargetsFn != nil {
		return m.GetAcquisitionTargetsFn(ctx)
	}
	return []campaigns.AcquisitionOpportunity{}, nil
}

func (m *MockAnalyticsService) GetActivationChecklist(ctx context.Context, campaignID string) (*campaigns.ActivationChecklist, error) {
	if m.GetActivationChecklistFn != nil {
		return m.GetActivationChecklistFn(ctx, campaignID)
	}
	return &campaigns.ActivationChecklist{}, nil
}

func (m *MockAnalyticsService) GetExpectedValues(ctx context.Context, campaignID string) (*campaigns.EVPortfolio, error) {
	if m.GetExpectedValuesFn != nil {
		return m.GetExpectedValuesFn(ctx, campaignID)
	}
	return &campaigns.EVPortfolio{}, nil
}

func (m *MockAnalyticsService) EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*campaigns.ExpectedValue, error) {
	if m.EvaluatePurchaseFn != nil {
		return m.EvaluatePurchaseFn(ctx, campaignID, cardName, grade, buyCostCents)
	}
	return &campaigns.ExpectedValue{}, nil
}

func (m *MockAnalyticsService) RunProjection(ctx context.Context, campaignID string) (*campaigns.MonteCarloComparison, error) {
	if m.RunProjectionFn != nil {
		return m.RunProjectionFn(ctx, campaignID)
	}
	return &campaigns.MonteCarloComparison{}, nil
}

func (m *MockAnalyticsService) GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*campaigns.SellSheet, error) {
	if m.GenerateSellSheetFn != nil {
		return m.GenerateSellSheetFn(ctx, campaignID, purchaseIDs)
	}
	return &campaigns.SellSheet{}, nil
}

func (m *MockAnalyticsService) GenerateGlobalSellSheet(ctx context.Context) (*campaigns.SellSheet, error) {
	if m.GenerateGlobalSellSheetFn != nil {
		return m.GenerateGlobalSellSheetFn(ctx)
	}
	return &campaigns.SellSheet{}, nil
}

func (m *MockAnalyticsService) GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*campaigns.SellSheet, error) {
	if m.GenerateSelectedSellSheetFn != nil {
		return m.GenerateSelectedSellSheetFn(ctx, purchaseIDs)
	}
	return &campaigns.SellSheet{}, nil
}

func (m *MockAnalyticsService) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*campaigns.EbayExportListResponse, error) {
	if m.ListEbayExportItemsFn != nil {
		return m.ListEbayExportItemsFn(ctx, flaggedOnly)
	}
	return &campaigns.EbayExportListResponse{}, nil
}

func (m *MockAnalyticsService) GenerateEbayCSV(ctx context.Context, items []campaigns.EbayExportGenerateItem) ([]byte, error) {
	if m.GenerateEbayCSVFn != nil {
		return m.GenerateEbayCSVFn(ctx, items)
	}
	return []byte{}, nil
}

func (m *MockAnalyticsService) MatchShopifyPrices(ctx context.Context, items []campaigns.ShopifyPriceSyncItem) (*campaigns.ShopifyPriceSyncResponse, error) {
	if m.MatchShopifyPricesFn != nil {
		return m.MatchShopifyPricesFn(ctx, items)
	}
	return &campaigns.ShopifyPriceSyncResponse{}, nil
}
