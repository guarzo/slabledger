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
	GetCampaignPNLFn              func(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error)
	GetPNLByChannelFn             func(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error)
	GetDailySpendFn               func(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error)
	GetDaysToSellDistributionFn   func(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error)
	GetInventoryAgingFn           func(ctx context.Context, campaignID string) (*inventory.InventoryResult, error)
	GetGlobalInventoryAgingFn     func(ctx context.Context) (*inventory.InventoryResult, error)
	GetFlaggedInventoryFn         func(ctx context.Context) ([]inventory.AgingItem, error)
	RefreshCrackCandidatesFn      func(ctx context.Context) error
	GetCampaignTuningFn           func(ctx context.Context, campaignID string) (*inventory.TuningResponse, error)
	GetPortfolioHealthFn          func(ctx context.Context) (*inventory.PortfolioHealth, error)
	GetPortfolioChannelVelocityFn func(ctx context.Context) ([]inventory.ChannelVelocity, error)
	GetPortfolioInsightsFn        func(ctx context.Context) (*inventory.PortfolioInsights, error)
	GetCampaignSuggestionsFn      func(ctx context.Context) (*inventory.SuggestionsResponse, error)
	GetCapitalTimelineFn          func(ctx context.Context) (*inventory.CapitalTimeline, error)
	GetWeeklyReviewSummaryFn      func(ctx context.Context) (*inventory.WeeklyReviewSummary, error)
	GetCrackCandidatesFn          func(ctx context.Context, campaignID string) ([]inventory.CrackAnalysis, error)
	GetCrackOpportunitiesFn       func(ctx context.Context) ([]inventory.CrackAnalysis, error)
	GetAcquisitionTargetsFn       func(ctx context.Context) ([]inventory.AcquisitionOpportunity, error)
	GetActivationChecklistFn      func(ctx context.Context, campaignID string) (*inventory.ActivationChecklist, error)
	GetExpectedValuesFn           func(ctx context.Context, campaignID string) (*inventory.EVPortfolio, error)
	EvaluatePurchaseFn            func(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*inventory.ExpectedValue, error)
	RunProjectionFn               func(ctx context.Context, campaignID string) (*inventory.MonteCarloComparison, error)
	GenerateSellSheetFn           func(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error)
	GenerateGlobalSellSheetFn     func(ctx context.Context) (*inventory.SellSheet, error)
	GenerateSelectedSellSheetFn   func(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error)
	ListEbayExportItemsFn         func(ctx context.Context, flaggedOnly bool) (*inventory.EbayExportListResponse, error)
	GenerateEbayCSVFn             func(ctx context.Context, items []inventory.EbayExportGenerateItem) ([]byte, error)
	MatchShopifyPricesFn          func(ctx context.Context, items []inventory.ShopifyPriceSyncItem) (*inventory.ShopifyPriceSyncResponse, error)
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

func (m *MockAnalyticsService) GetCampaignTuning(ctx context.Context, campaignID string) (*inventory.TuningResponse, error) {
	if m.GetCampaignTuningFn != nil {
		return m.GetCampaignTuningFn(ctx, campaignID)
	}
	return &inventory.TuningResponse{CampaignID: campaignID}, nil
}

func (m *MockAnalyticsService) GetPortfolioHealth(ctx context.Context) (*inventory.PortfolioHealth, error) {
	if m.GetPortfolioHealthFn != nil {
		return m.GetPortfolioHealthFn(ctx)
	}
	return &inventory.PortfolioHealth{}, nil
}

func (m *MockAnalyticsService) GetPortfolioChannelVelocity(ctx context.Context) ([]inventory.ChannelVelocity, error) {
	if m.GetPortfolioChannelVelocityFn != nil {
		return m.GetPortfolioChannelVelocityFn(ctx)
	}
	return []inventory.ChannelVelocity{}, nil
}

func (m *MockAnalyticsService) GetPortfolioInsights(ctx context.Context) (*inventory.PortfolioInsights, error) {
	if m.GetPortfolioInsightsFn != nil {
		return m.GetPortfolioInsightsFn(ctx)
	}
	return &inventory.PortfolioInsights{}, nil
}

func (m *MockAnalyticsService) GetCampaignSuggestions(ctx context.Context) (*inventory.SuggestionsResponse, error) {
	if m.GetCampaignSuggestionsFn != nil {
		return m.GetCampaignSuggestionsFn(ctx)
	}
	return &inventory.SuggestionsResponse{}, nil
}

func (m *MockAnalyticsService) GetCapitalTimeline(ctx context.Context) (*inventory.CapitalTimeline, error) {
	if m.GetCapitalTimelineFn != nil {
		return m.GetCapitalTimelineFn(ctx)
	}
	return &inventory.CapitalTimeline{}, nil
}

func (m *MockAnalyticsService) GetWeeklyReviewSummary(ctx context.Context) (*inventory.WeeklyReviewSummary, error) {
	if m.GetWeeklyReviewSummaryFn != nil {
		return m.GetWeeklyReviewSummaryFn(ctx)
	}
	return &inventory.WeeklyReviewSummary{}, nil
}

func (m *MockAnalyticsService) GetCrackCandidates(ctx context.Context, campaignID string) ([]inventory.CrackAnalysis, error) {
	if m.GetCrackCandidatesFn != nil {
		return m.GetCrackCandidatesFn(ctx, campaignID)
	}
	return []inventory.CrackAnalysis{}, nil
}

func (m *MockAnalyticsService) GetCrackOpportunities(ctx context.Context) ([]inventory.CrackAnalysis, error) {
	if m.GetCrackOpportunitiesFn != nil {
		return m.GetCrackOpportunitiesFn(ctx)
	}
	return []inventory.CrackAnalysis{}, nil
}

func (m *MockAnalyticsService) GetAcquisitionTargets(ctx context.Context) ([]inventory.AcquisitionOpportunity, error) {
	if m.GetAcquisitionTargetsFn != nil {
		return m.GetAcquisitionTargetsFn(ctx)
	}
	return []inventory.AcquisitionOpportunity{}, nil
}

func (m *MockAnalyticsService) GetActivationChecklist(ctx context.Context, campaignID string) (*inventory.ActivationChecklist, error) {
	if m.GetActivationChecklistFn != nil {
		return m.GetActivationChecklistFn(ctx, campaignID)
	}
	return &inventory.ActivationChecklist{}, nil
}

func (m *MockAnalyticsService) GetExpectedValues(ctx context.Context, campaignID string) (*inventory.EVPortfolio, error) {
	if m.GetExpectedValuesFn != nil {
		return m.GetExpectedValuesFn(ctx, campaignID)
	}
	return &inventory.EVPortfolio{}, nil
}

func (m *MockAnalyticsService) EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*inventory.ExpectedValue, error) {
	if m.EvaluatePurchaseFn != nil {
		return m.EvaluatePurchaseFn(ctx, campaignID, cardName, grade, buyCostCents)
	}
	return &inventory.ExpectedValue{}, nil
}

func (m *MockAnalyticsService) RunProjection(ctx context.Context, campaignID string) (*inventory.MonteCarloComparison, error) {
	if m.RunProjectionFn != nil {
		return m.RunProjectionFn(ctx, campaignID)
	}
	return &inventory.MonteCarloComparison{}, nil
}

func (m *MockAnalyticsService) GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error) {
	if m.GenerateSellSheetFn != nil {
		return m.GenerateSellSheetFn(ctx, campaignID, purchaseIDs)
	}
	return &inventory.SellSheet{}, nil
}

func (m *MockAnalyticsService) GenerateGlobalSellSheet(ctx context.Context) (*inventory.SellSheet, error) {
	if m.GenerateGlobalSellSheetFn != nil {
		return m.GenerateGlobalSellSheetFn(ctx)
	}
	return &inventory.SellSheet{}, nil
}

func (m *MockAnalyticsService) GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error) {
	if m.GenerateSelectedSellSheetFn != nil {
		return m.GenerateSelectedSellSheetFn(ctx, purchaseIDs)
	}
	return &inventory.SellSheet{}, nil
}

func (m *MockAnalyticsService) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*inventory.EbayExportListResponse, error) {
	if m.ListEbayExportItemsFn != nil {
		return m.ListEbayExportItemsFn(ctx, flaggedOnly)
	}
	return &inventory.EbayExportListResponse{}, nil
}

func (m *MockAnalyticsService) GenerateEbayCSV(ctx context.Context, items []inventory.EbayExportGenerateItem) ([]byte, error) {
	if m.GenerateEbayCSVFn != nil {
		return m.GenerateEbayCSVFn(ctx, items)
	}
	return []byte{}, nil
}

func (m *MockAnalyticsService) MatchShopifyPrices(ctx context.Context, items []inventory.ShopifyPriceSyncItem) (*inventory.ShopifyPriceSyncResponse, error) {
	if m.MatchShopifyPricesFn != nil {
		return m.MatchShopifyPricesFn(ctx, items)
	}
	return &inventory.ShopifyPriceSyncResponse{}, nil
}
