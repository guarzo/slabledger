package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// MockCampaignService is a test double for campaigns.Service.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockCampaignService{
//	    ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]campaigns.Campaign, error) {
//	        return []campaigns.Campaign{{ID: "1", Name: "Test"}}, nil
//	    },
//	}
type MockCampaignService struct {
	CreateCampaignFn            func(ctx context.Context, c *campaigns.Campaign) error
	GetCampaignFn               func(ctx context.Context, id string) (*campaigns.Campaign, error)
	ListCampaignsFn             func(ctx context.Context, activeOnly bool) ([]campaigns.Campaign, error)
	UpdateCampaignFn            func(ctx context.Context, c *campaigns.Campaign) error
	DeleteCampaignFn            func(ctx context.Context, id string) error
	CreatePurchaseFn            func(ctx context.Context, p *campaigns.Purchase) error
	GetPurchaseFn               func(ctx context.Context, id string) (*campaigns.Purchase, error)
	DeletePurchaseFn            func(ctx context.Context, id string) error
	ListPurchasesByCampaignFn   func(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Purchase, error)
	CreateSaleFn                func(ctx context.Context, s *campaigns.Sale, campaign *campaigns.Campaign, purchase *campaigns.Purchase) error
	CreateBulkSalesFn           func(ctx context.Context, campaignID string, channel campaigns.SaleChannel, saleDate string, items []campaigns.BulkSaleInput) (*campaigns.BulkSaleResult, error)
	ListSalesByCampaignFn       func(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Sale, error)
	DeleteSaleByPurchaseIDFn    func(ctx context.Context, purchaseID string) error
	LookupCertFn                func(ctx context.Context, certNumber string) (*campaigns.CertInfo, *campaigns.MarketSnapshot, error)
	QuickAddPurchaseFn          func(ctx context.Context, campaignID string, req campaigns.QuickAddRequest) (*campaigns.Purchase, error)
	GetCampaignPNLFn            func(ctx context.Context, campaignID string) (*campaigns.CampaignPNL, error)
	GetPNLByChannelFn           func(ctx context.Context, campaignID string) ([]campaigns.ChannelPNL, error)
	GetDailySpendFn             func(ctx context.Context, campaignID string, days int) ([]campaigns.DailySpend, error)
	GetDaysToSellDistFn         func(ctx context.Context, campaignID string) ([]campaigns.DaysToSellBucket, error)
	GetInventoryAgingFn         func(ctx context.Context, campaignID string) (*campaigns.InventoryResult, error)
	GetGlobalInventoryAgingFn   func(ctx context.Context) (*campaigns.InventoryResult, error)
	GetFlaggedInventoryFn       func(ctx context.Context) ([]campaigns.AgingItem, error)
	GenerateSellSheetFn         func(ctx context.Context, campaignID string, purchaseIDs []string) (*campaigns.SellSheet, error)
	GenerateGlobalSellSheetFn   func(ctx context.Context) (*campaigns.SellSheet, error)
	GenerateSelectedSellSheetFn func(ctx context.Context, purchaseIDs []string) (*campaigns.SellSheet, error)
	GetCampaignTuningFn         func(ctx context.Context, campaignID string) (*campaigns.TuningResponse, error)
	RefreshCLValuesGlobalFn     func(ctx context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalCLRefreshResult, error)
	ImportCLExportGlobalFn      func(ctx context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalImportResult, error)
	ImportPSAExportGlobalFn     func(ctx context.Context, rows []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error)
	ExportCLFormatGlobalFn      func(ctx context.Context, missingCLOnly bool) ([]campaigns.CLExportEntry, error)
	ReassignPurchaseFn          func(ctx context.Context, purchaseID string, newCampaignID string) error

	// Capital & Invoice
	GetCapitalSummaryFn func(ctx context.Context) (*campaigns.CapitalSummary, error)
	GetCashflowConfigFn func(ctx context.Context) (*campaigns.CashflowConfig, error)
	ListInvoicesFn      func(ctx context.Context) ([]campaigns.Invoice, error)
	UpdateInvoiceFn     func(ctx context.Context, inv *campaigns.Invoice) error

	// Portfolio health
	GetPortfolioHealthFn          func(ctx context.Context) (*campaigns.PortfolioHealth, error)
	GetPortfolioChannelVelocityFn func(ctx context.Context) ([]campaigns.ChannelVelocity, error)

	// Portfolio insights & suggestions
	GetPortfolioInsightsFn   func(ctx context.Context) (*campaigns.PortfolioInsights, error)
	GetCampaignSuggestionsFn func(ctx context.Context) (*campaigns.SuggestionsResponse, error)

	// Revocation
	FlagForRevocationFn       func(ctx context.Context, segmentLabel, segmentDimension, reason string) (*campaigns.RevocationFlag, error)
	ListRevocationFlagsFn     func(ctx context.Context) ([]campaigns.RevocationFlag, error)
	GenerateRevocationEmailFn func(ctx context.Context, flagID string) (string, error)

	// Capital timeline
	GetCapitalTimelineFn func(ctx context.Context) (*campaigns.CapitalTimeline, error)

	// Weekly review
	GetWeeklyReviewSummaryFn func(ctx context.Context) (*campaigns.WeeklyReviewSummary, error)

	// Crack arbitrage
	GetCrackCandidatesFn    func(ctx context.Context, campaignID string) ([]campaigns.CrackAnalysis, error)
	GetCrackOpportunitiesFn func(ctx context.Context) ([]campaigns.CrackAnalysis, error)

	// Acquisition arbitrage
	GetAcquisitionTargetsFn func(ctx context.Context) ([]campaigns.AcquisitionOpportunity, error)

	// Expected value
	GetExpectedValuesFn func(ctx context.Context, campaignID string) (*campaigns.EVPortfolio, error)
	EvaluatePurchaseFn  func(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*campaigns.ExpectedValue, error)

	// Activation checklist
	GetActivationChecklistFn func(ctx context.Context, campaignID string) (*campaigns.ActivationChecklist, error)

	// Monte Carlo projection
	RunProjectionFn func(ctx context.Context, campaignID string) (*campaigns.MonteCarloComparison, error)

	// Cert entry & eBay export
	ImportCertsFn         func(ctx context.Context, certNumbers []string) (*campaigns.CertImportResult, error)
	ListEbayExportItemsFn func(ctx context.Context, flaggedOnly bool) (*campaigns.EbayExportListResponse, error)
	GenerateEbayCSVFn     func(ctx context.Context, items []campaigns.EbayExportGenerateItem) ([]byte, error)

	// Shopify price sync
	MatchShopifyPricesFn func(ctx context.Context, items []campaigns.ShopifyPriceSyncItem) (*campaigns.ShopifyPriceSyncResponse, error)

	// Price overrides & AI suggestions
	GetPriceOverrideStatsFn func(ctx context.Context) (*campaigns.PriceOverrideStats, error)
	UpdateBuyCostFn         func(ctx context.Context, purchaseID string, buyCostCents int) error
	SetPriceOverrideFn      func(ctx context.Context, purchaseID string, priceCents int, source string) error
	SetAISuggestedPriceFn   func(ctx context.Context, purchaseID string, priceCents int) error
	AcceptAISuggestionFn    func(ctx context.Context, purchaseID string) error
	DismissAISuggestionFn   func(ctx context.Context, purchaseID string) error

	// Price review & flags
	SetReviewedPriceFn     func(ctx context.Context, purchaseID string, priceCents int, source string) error
	GetReviewStatsFn       func(ctx context.Context, campaignID string) (campaigns.ReviewStats, error)
	GetGlobalReviewStatsFn func(ctx context.Context) (campaigns.ReviewStats, error)
	CreatePriceFlagFn      func(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error)
	ListPriceFlagsFn       func(ctx context.Context, status string) ([]campaigns.PriceFlagWithContext, error)
	ResolvePriceFlagFn     func(ctx context.Context, flagID int64, resolvedBy int64) error

	// Snapshot refresh
	RefreshPurchaseSnapshotFn func(ctx context.Context, purchaseID string, card campaigns.CardIdentity, grade float64, clValueCents int) bool
	ProcessPendingSnapshotsFn func(ctx context.Context, limit int) (int, int, int)
	RetryFailedSnapshotsFn    func(ctx context.Context, limit int) (int, int, int)

	// External purchases
	EnsureExternalCampaignFn func(ctx context.Context) (*campaigns.Campaign, error)
	ImportExternalCSVFn      func(ctx context.Context, rows []campaigns.ShopifyExportRow) (*campaigns.ExternalImportResult, error)

	// Orders sales import
	ImportOrdersSalesFn  func(ctx context.Context, rows []campaigns.OrdersExportRow) (*campaigns.OrdersImportResult, error)
	ConfirmOrdersSalesFn func(ctx context.Context, items []campaigns.OrdersConfirmItem) (*campaigns.BulkSaleResult, error)

	// Cert batch lookup
	GetPurchasesByCertNumbersFn func(ctx context.Context, certNumbers []string) (map[string]*campaigns.Purchase, error)
	ScanCertFn                  func(ctx context.Context, certNumber string) (*campaigns.ScanCertResult, error)
	ResolveCertFn               func(ctx context.Context, certNumber string) (*campaigns.CertInfo, error)
}

var _ campaigns.Service = (*MockCampaignService)(nil)

func (m *MockCampaignService) CreateCampaign(ctx context.Context, c *campaigns.Campaign) error {
	if m.CreateCampaignFn != nil {
		return m.CreateCampaignFn(ctx, c)
	}
	return nil
}

func (m *MockCampaignService) GetCampaign(ctx context.Context, id string) (*campaigns.Campaign, error) {
	if m.GetCampaignFn != nil {
		return m.GetCampaignFn(ctx, id)
	}
	return &campaigns.Campaign{ID: id, Name: "mock"}, nil
}

func (m *MockCampaignService) ListCampaigns(ctx context.Context, activeOnly bool) ([]campaigns.Campaign, error) {
	if m.ListCampaignsFn != nil {
		return m.ListCampaignsFn(ctx, activeOnly)
	}
	return []campaigns.Campaign{}, nil
}

func (m *MockCampaignService) UpdateCampaign(ctx context.Context, c *campaigns.Campaign) error {
	if m.UpdateCampaignFn != nil {
		return m.UpdateCampaignFn(ctx, c)
	}
	return nil
}

func (m *MockCampaignService) DeleteCampaign(ctx context.Context, id string) error {
	if m.DeleteCampaignFn != nil {
		return m.DeleteCampaignFn(ctx, id)
	}
	return nil
}

func (m *MockCampaignService) CreatePurchase(ctx context.Context, p *campaigns.Purchase) error {
	if m.CreatePurchaseFn != nil {
		return m.CreatePurchaseFn(ctx, p)
	}
	return nil
}

func (m *MockCampaignService) GetPurchase(ctx context.Context, id string) (*campaigns.Purchase, error) {
	if m.GetPurchaseFn != nil {
		return m.GetPurchaseFn(ctx, id)
	}
	return &campaigns.Purchase{ID: id}, nil
}

func (m *MockCampaignService) DeletePurchase(ctx context.Context, id string) error {
	if m.DeletePurchaseFn != nil {
		return m.DeletePurchaseFn(ctx, id)
	}
	return nil
}

func (m *MockCampaignService) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Purchase, error) {
	if m.ListPurchasesByCampaignFn != nil {
		return m.ListPurchasesByCampaignFn(ctx, campaignID, limit, offset)
	}
	return []campaigns.Purchase{}, nil
}

func (m *MockCampaignService) CreateSale(ctx context.Context, s *campaigns.Sale, campaign *campaigns.Campaign, purchase *campaigns.Purchase) error {
	if m.CreateSaleFn != nil {
		return m.CreateSaleFn(ctx, s, campaign, purchase)
	}
	return nil
}

func (m *MockCampaignService) CreateBulkSales(ctx context.Context, campaignID string, channel campaigns.SaleChannel, saleDate string, items []campaigns.BulkSaleInput) (*campaigns.BulkSaleResult, error) {
	if m.CreateBulkSalesFn != nil {
		return m.CreateBulkSalesFn(ctx, campaignID, channel, saleDate, items)
	}
	return &campaigns.BulkSaleResult{Created: len(items)}, nil
}

func (m *MockCampaignService) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Sale, error) {
	if m.ListSalesByCampaignFn != nil {
		return m.ListSalesByCampaignFn(ctx, campaignID, limit, offset)
	}
	return []campaigns.Sale{}, nil
}

func (m *MockCampaignService) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	if m.DeleteSaleByPurchaseIDFn != nil {
		return m.DeleteSaleByPurchaseIDFn(ctx, purchaseID)
	}
	return nil
}

func (m *MockCampaignService) LookupCert(ctx context.Context, certNumber string) (*campaigns.CertInfo, *campaigns.MarketSnapshot, error) {
	if m.LookupCertFn != nil {
		return m.LookupCertFn(ctx, certNumber)
	}
	return &campaigns.CertInfo{CertNumber: certNumber}, nil, nil
}

func (m *MockCampaignService) QuickAddPurchase(ctx context.Context, campaignID string, req campaigns.QuickAddRequest) (*campaigns.Purchase, error) {
	if m.QuickAddPurchaseFn != nil {
		return m.QuickAddPurchaseFn(ctx, campaignID, req)
	}
	return &campaigns.Purchase{CampaignID: campaignID}, nil
}

func (m *MockCampaignService) GetCampaignPNL(ctx context.Context, campaignID string) (*campaigns.CampaignPNL, error) {
	if m.GetCampaignPNLFn != nil {
		return m.GetCampaignPNLFn(ctx, campaignID)
	}
	return &campaigns.CampaignPNL{CampaignID: campaignID}, nil
}

func (m *MockCampaignService) GetPNLByChannel(ctx context.Context, campaignID string) ([]campaigns.ChannelPNL, error) {
	if m.GetPNLByChannelFn != nil {
		return m.GetPNLByChannelFn(ctx, campaignID)
	}
	return []campaigns.ChannelPNL{}, nil
}

func (m *MockCampaignService) GetDailySpend(ctx context.Context, campaignID string, days int) ([]campaigns.DailySpend, error) {
	if m.GetDailySpendFn != nil {
		return m.GetDailySpendFn(ctx, campaignID, days)
	}
	return []campaigns.DailySpend{}, nil
}

func (m *MockCampaignService) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]campaigns.DaysToSellBucket, error) {
	if m.GetDaysToSellDistFn != nil {
		return m.GetDaysToSellDistFn(ctx, campaignID)
	}
	return []campaigns.DaysToSellBucket{}, nil
}

func (m *MockCampaignService) GetInventoryAging(ctx context.Context, campaignID string) (*campaigns.InventoryResult, error) {
	if m.GetInventoryAgingFn != nil {
		return m.GetInventoryAgingFn(ctx, campaignID)
	}
	return &campaigns.InventoryResult{Items: []campaigns.AgingItem{}}, nil
}

func (m *MockCampaignService) GetGlobalInventoryAging(ctx context.Context) (*campaigns.InventoryResult, error) {
	if m.GetGlobalInventoryAgingFn != nil {
		return m.GetGlobalInventoryAgingFn(ctx)
	}
	return &campaigns.InventoryResult{Items: []campaigns.AgingItem{}}, nil
}

func (m *MockCampaignService) GetFlaggedInventory(ctx context.Context) ([]campaigns.AgingItem, error) {
	if m.GetFlaggedInventoryFn != nil {
		return m.GetFlaggedInventoryFn(ctx)
	}
	return []campaigns.AgingItem{}, nil
}

func (m *MockCampaignService) GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*campaigns.SellSheet, error) {
	if m.GenerateSellSheetFn != nil {
		return m.GenerateSellSheetFn(ctx, campaignID, purchaseIDs)
	}
	return &campaigns.SellSheet{}, nil
}

func (m *MockCampaignService) GenerateGlobalSellSheet(ctx context.Context) (*campaigns.SellSheet, error) {
	if m.GenerateGlobalSellSheetFn != nil {
		return m.GenerateGlobalSellSheetFn(ctx)
	}
	return &campaigns.SellSheet{}, nil
}

func (m *MockCampaignService) GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*campaigns.SellSheet, error) {
	if m.GenerateSelectedSellSheetFn != nil {
		return m.GenerateSelectedSellSheetFn(ctx, purchaseIDs)
	}
	return &campaigns.SellSheet{}, nil
}

func (m *MockCampaignService) GetCampaignTuning(ctx context.Context, campaignID string) (*campaigns.TuningResponse, error) {
	if m.GetCampaignTuningFn != nil {
		return m.GetCampaignTuningFn(ctx, campaignID)
	}
	return &campaigns.TuningResponse{CampaignID: campaignID}, nil
}

func (m *MockCampaignService) RefreshCLValuesGlobal(ctx context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalCLRefreshResult, error) {
	if m.RefreshCLValuesGlobalFn != nil {
		return m.RefreshCLValuesGlobalFn(ctx, rows)
	}
	return &campaigns.GlobalCLRefreshResult{}, nil
}

func (m *MockCampaignService) ImportCLExportGlobal(ctx context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalImportResult, error) {
	if m.ImportCLExportGlobalFn != nil {
		return m.ImportCLExportGlobalFn(ctx, rows)
	}
	return &campaigns.GlobalImportResult{}, nil
}

func (m *MockCampaignService) ExportCLFormatGlobal(ctx context.Context, missingCLOnly bool) ([]campaigns.CLExportEntry, error) {
	if m.ExportCLFormatGlobalFn != nil {
		return m.ExportCLFormatGlobalFn(ctx, missingCLOnly)
	}
	return []campaigns.CLExportEntry{}, nil
}

func (m *MockCampaignService) ImportPSAExportGlobal(ctx context.Context, rows []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error) {
	if m.ImportPSAExportGlobalFn != nil {
		return m.ImportPSAExportGlobalFn(ctx, rows)
	}
	return &campaigns.PSAImportResult{}, nil
}

func (m *MockCampaignService) ReassignPurchase(ctx context.Context, purchaseID string, newCampaignID string) error {
	if m.ReassignPurchaseFn != nil {
		return m.ReassignPurchaseFn(ctx, purchaseID, newCampaignID)
	}
	return nil
}

func (m *MockCampaignService) GetCapitalSummary(ctx context.Context) (*campaigns.CapitalSummary, error) {
	if m.GetCapitalSummaryFn != nil {
		return m.GetCapitalSummaryFn(ctx)
	}
	return &campaigns.CapitalSummary{OutstandingCents: 0, WeeksToCover: campaigns.WeeksToCoverNoData, AlertLevel: campaigns.AlertOK}, nil
}

func (m *MockCampaignService) GetCashflowConfig(ctx context.Context) (*campaigns.CashflowConfig, error) {
	if m.GetCashflowConfigFn != nil {
		return m.GetCashflowConfigFn(ctx)
	}
	return &campaigns.CashflowConfig{CapitalBudgetCents: 5000000, CashBufferCents: 1000000}, nil
}

func (m *MockCampaignService) ListInvoices(ctx context.Context) ([]campaigns.Invoice, error) {
	if m.ListInvoicesFn != nil {
		return m.ListInvoicesFn(ctx)
	}
	return []campaigns.Invoice{}, nil
}

func (m *MockCampaignService) UpdateInvoice(ctx context.Context, inv *campaigns.Invoice) error {
	if m.UpdateInvoiceFn != nil {
		return m.UpdateInvoiceFn(ctx, inv)
	}
	return nil
}

func (m *MockCampaignService) GetPortfolioHealth(ctx context.Context) (*campaigns.PortfolioHealth, error) {
	if m.GetPortfolioHealthFn != nil {
		return m.GetPortfolioHealthFn(ctx)
	}
	return &campaigns.PortfolioHealth{}, nil
}

func (m *MockCampaignService) GetPortfolioChannelVelocity(ctx context.Context) ([]campaigns.ChannelVelocity, error) {
	if m.GetPortfolioChannelVelocityFn != nil {
		return m.GetPortfolioChannelVelocityFn(ctx)
	}
	return []campaigns.ChannelVelocity{}, nil
}

func (m *MockCampaignService) GetPortfolioInsights(ctx context.Context) (*campaigns.PortfolioInsights, error) {
	if m.GetPortfolioInsightsFn != nil {
		return m.GetPortfolioInsightsFn(ctx)
	}
	return &campaigns.PortfolioInsights{}, nil
}

func (m *MockCampaignService) GetCampaignSuggestions(ctx context.Context) (*campaigns.SuggestionsResponse, error) {
	if m.GetCampaignSuggestionsFn != nil {
		return m.GetCampaignSuggestionsFn(ctx)
	}
	return &campaigns.SuggestionsResponse{}, nil
}

// Revocation

func (m *MockCampaignService) FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*campaigns.RevocationFlag, error) {
	if m.FlagForRevocationFn != nil {
		return m.FlagForRevocationFn(ctx, segmentLabel, segmentDimension, reason)
	}
	return &campaigns.RevocationFlag{}, nil
}

func (m *MockCampaignService) ListRevocationFlags(ctx context.Context) ([]campaigns.RevocationFlag, error) {
	if m.ListRevocationFlagsFn != nil {
		return m.ListRevocationFlagsFn(ctx)
	}
	return []campaigns.RevocationFlag{}, nil
}

func (m *MockCampaignService) GenerateRevocationEmail(ctx context.Context, flagID string) (string, error) {
	if m.GenerateRevocationEmailFn != nil {
		return m.GenerateRevocationEmailFn(ctx, flagID)
	}
	return "", nil
}

// Capital timeline

func (m *MockCampaignService) GetCapitalTimeline(ctx context.Context) (*campaigns.CapitalTimeline, error) {
	if m.GetCapitalTimelineFn != nil {
		return m.GetCapitalTimelineFn(ctx)
	}
	return &campaigns.CapitalTimeline{}, nil
}

// Weekly review

func (m *MockCampaignService) GetWeeklyReviewSummary(ctx context.Context) (*campaigns.WeeklyReviewSummary, error) {
	if m.GetWeeklyReviewSummaryFn != nil {
		return m.GetWeeklyReviewSummaryFn(ctx)
	}
	return &campaigns.WeeklyReviewSummary{}, nil
}

// Crack arbitrage

func (m *MockCampaignService) GetCrackCandidates(ctx context.Context, campaignID string) ([]campaigns.CrackAnalysis, error) {
	if m.GetCrackCandidatesFn != nil {
		return m.GetCrackCandidatesFn(ctx, campaignID)
	}
	return []campaigns.CrackAnalysis{}, nil
}

func (m *MockCampaignService) GetCrackOpportunities(ctx context.Context) ([]campaigns.CrackAnalysis, error) {
	if m.GetCrackOpportunitiesFn != nil {
		return m.GetCrackOpportunitiesFn(ctx)
	}
	return []campaigns.CrackAnalysis{}, nil
}

func (m *MockCampaignService) GetAcquisitionTargets(ctx context.Context) ([]campaigns.AcquisitionOpportunity, error) {
	if m.GetAcquisitionTargetsFn != nil {
		return m.GetAcquisitionTargetsFn(ctx)
	}
	return []campaigns.AcquisitionOpportunity{}, nil
}

// Expected value

func (m *MockCampaignService) GetExpectedValues(ctx context.Context, campaignID string) (*campaigns.EVPortfolio, error) {
	if m.GetExpectedValuesFn != nil {
		return m.GetExpectedValuesFn(ctx, campaignID)
	}
	return &campaigns.EVPortfolio{}, nil
}

func (m *MockCampaignService) EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*campaigns.ExpectedValue, error) {
	if m.EvaluatePurchaseFn != nil {
		return m.EvaluatePurchaseFn(ctx, campaignID, cardName, grade, buyCostCents)
	}
	return &campaigns.ExpectedValue{}, nil
}

// Activation checklist

func (m *MockCampaignService) GetActivationChecklist(ctx context.Context, campaignID string) (*campaigns.ActivationChecklist, error) {
	if m.GetActivationChecklistFn != nil {
		return m.GetActivationChecklistFn(ctx, campaignID)
	}
	return &campaigns.ActivationChecklist{CampaignID: campaignID, AllPassed: true}, nil
}

// Monte Carlo projection

func (m *MockCampaignService) RunProjection(ctx context.Context, campaignID string) (*campaigns.MonteCarloComparison, error) {
	if m.RunProjectionFn != nil {
		return m.RunProjectionFn(ctx, campaignID)
	}
	return &campaigns.MonteCarloComparison{Confidence: "insufficient"}, nil
}

// Cert entry & eBay export

func (m *MockCampaignService) ImportCerts(ctx context.Context, certNumbers []string) (*campaigns.CertImportResult, error) {
	if m.ImportCertsFn != nil {
		return m.ImportCertsFn(ctx, certNumbers)
	}
	return nil, nil
}

func (m *MockCampaignService) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*campaigns.EbayExportListResponse, error) {
	if m.ListEbayExportItemsFn != nil {
		return m.ListEbayExportItemsFn(ctx, flaggedOnly)
	}
	return nil, nil
}

func (m *MockCampaignService) GenerateEbayCSV(ctx context.Context, items []campaigns.EbayExportGenerateItem) ([]byte, error) {
	if m.GenerateEbayCSVFn != nil {
		return m.GenerateEbayCSVFn(ctx, items)
	}
	return nil, nil
}

// Shopify price sync

func (m *MockCampaignService) MatchShopifyPrices(ctx context.Context, items []campaigns.ShopifyPriceSyncItem) (*campaigns.ShopifyPriceSyncResponse, error) {
	if m.MatchShopifyPricesFn != nil {
		return m.MatchShopifyPricesFn(ctx, items)
	}
	return &campaigns.ShopifyPriceSyncResponse{Matched: []campaigns.ShopifyPriceSyncMatch{}, Unmatched: []string{}}, nil
}

// External purchases

func (m *MockCampaignService) EnsureExternalCampaign(ctx context.Context) (*campaigns.Campaign, error) {
	if m.EnsureExternalCampaignFn != nil {
		return m.EnsureExternalCampaignFn(ctx)
	}
	return &campaigns.Campaign{ID: campaigns.ExternalCampaignID, Name: campaigns.ExternalCampaignName}, nil
}

func (m *MockCampaignService) ImportExternalCSV(ctx context.Context, rows []campaigns.ShopifyExportRow) (*campaigns.ExternalImportResult, error) {
	if m.ImportExternalCSVFn != nil {
		return m.ImportExternalCSVFn(ctx, rows)
	}
	return &campaigns.ExternalImportResult{}, nil
}

func (m *MockCampaignService) ImportOrdersSales(ctx context.Context, rows []campaigns.OrdersExportRow) (*campaigns.OrdersImportResult, error) {
	if m.ImportOrdersSalesFn != nil {
		return m.ImportOrdersSalesFn(ctx, rows)
	}
	return &campaigns.OrdersImportResult{}, nil
}

func (m *MockCampaignService) ConfirmOrdersSales(ctx context.Context, items []campaigns.OrdersConfirmItem) (*campaigns.BulkSaleResult, error) {
	if m.ConfirmOrdersSalesFn != nil {
		return m.ConfirmOrdersSalesFn(ctx, items)
	}
	return &campaigns.BulkSaleResult{}, nil
}

// Price overrides & AI suggestions

func (m *MockCampaignService) GetPriceOverrideStats(ctx context.Context) (*campaigns.PriceOverrideStats, error) {
	if m.GetPriceOverrideStatsFn != nil {
		return m.GetPriceOverrideStatsFn(ctx)
	}
	return &campaigns.PriceOverrideStats{}, nil
}

func (m *MockCampaignService) UpdateBuyCost(ctx context.Context, purchaseID string, buyCostCents int) error {
	if m.UpdateBuyCostFn != nil {
		return m.UpdateBuyCostFn(ctx, purchaseID, buyCostCents)
	}
	return nil
}

func (m *MockCampaignService) SetPriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.SetPriceOverrideFn != nil {
		return m.SetPriceOverrideFn(ctx, purchaseID, priceCents, source)
	}
	return nil
}

func (m *MockCampaignService) SetAISuggestedPrice(ctx context.Context, purchaseID string, priceCents int) error {
	if m.SetAISuggestedPriceFn != nil {
		return m.SetAISuggestedPriceFn(ctx, purchaseID, priceCents)
	}
	return nil
}

func (m *MockCampaignService) AcceptAISuggestion(ctx context.Context, purchaseID string) error {
	if m.AcceptAISuggestionFn != nil {
		return m.AcceptAISuggestionFn(ctx, purchaseID)
	}
	return nil
}

func (m *MockCampaignService) DismissAISuggestion(ctx context.Context, purchaseID string) error {
	if m.DismissAISuggestionFn != nil {
		return m.DismissAISuggestionFn(ctx, purchaseID)
	}
	return nil
}

// Price review & flags

func (m *MockCampaignService) SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.SetReviewedPriceFn != nil {
		return m.SetReviewedPriceFn(ctx, purchaseID, priceCents, source)
	}
	return nil
}

func (m *MockCampaignService) GetReviewStats(ctx context.Context, campaignID string) (campaigns.ReviewStats, error) {
	if m.GetReviewStatsFn != nil {
		return m.GetReviewStatsFn(ctx, campaignID)
	}
	return campaigns.ReviewStats{}, nil
}

func (m *MockCampaignService) GetGlobalReviewStats(ctx context.Context) (campaigns.ReviewStats, error) {
	if m.GetGlobalReviewStatsFn != nil {
		return m.GetGlobalReviewStatsFn(ctx)
	}
	return campaigns.ReviewStats{}, nil
}

func (m *MockCampaignService) CreatePriceFlag(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error) {
	if m.CreatePriceFlagFn != nil {
		return m.CreatePriceFlagFn(ctx, purchaseID, userID, reason)
	}
	return 0, nil
}

func (m *MockCampaignService) ListPriceFlags(ctx context.Context, status string) ([]campaigns.PriceFlagWithContext, error) {
	if m.ListPriceFlagsFn != nil {
		return m.ListPriceFlagsFn(ctx, status)
	}
	return []campaigns.PriceFlagWithContext{}, nil
}

func (m *MockCampaignService) ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error {
	if m.ResolvePriceFlagFn != nil {
		return m.ResolvePriceFlagFn(ctx, flagID, resolvedBy)
	}
	return nil
}

func (m *MockCampaignService) RefreshPurchaseSnapshot(ctx context.Context, purchaseID string, card campaigns.CardIdentity, grade float64, clValueCents int) bool {
	if m.RefreshPurchaseSnapshotFn != nil {
		return m.RefreshPurchaseSnapshotFn(ctx, purchaseID, card, grade, clValueCents)
	}
	return false
}

func (m *MockCampaignService) ProcessPendingSnapshots(ctx context.Context, limit int) (int, int, int) {
	if m.ProcessPendingSnapshotsFn != nil {
		return m.ProcessPendingSnapshotsFn(ctx, limit)
	}
	return 0, 0, 0
}

func (m *MockCampaignService) RetryFailedSnapshots(ctx context.Context, limit int) (int, int, int) {
	if m.RetryFailedSnapshotsFn != nil {
		return m.RetryFailedSnapshotsFn(ctx, limit)
	}
	return 0, 0, 0
}

func (m *MockCampaignService) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*campaigns.Purchase, error) {
	if m.GetPurchasesByCertNumbersFn != nil {
		return m.GetPurchasesByCertNumbersFn(ctx, certNumbers)
	}
	return map[string]*campaigns.Purchase{}, nil
}

func (m *MockCampaignService) ScanCert(ctx context.Context, certNumber string) (*campaigns.ScanCertResult, error) {
	if m.ScanCertFn != nil {
		return m.ScanCertFn(ctx, certNumber)
	}
	return &campaigns.ScanCertResult{Status: "new"}, nil
}

func (m *MockCampaignService) ResolveCert(ctx context.Context, certNumber string) (*campaigns.CertInfo, error) {
	if m.ResolveCertFn != nil {
		return m.ResolveCertFn(ctx, certNumber)
	}
	return nil, nil
}

func (m *MockCampaignService) Close() {}
