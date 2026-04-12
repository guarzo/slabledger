package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// MockInventoryService is a test double for inventory.Service.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockInventoryService{
//	    ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
//	        return []inventory.Campaign{{ID: "1", Name: "Test"}}, nil
//	    },
//	}
type MockInventoryService struct {
	CreateCampaignFn            func(ctx context.Context, c *inventory.Campaign) error
	GetCampaignFn               func(ctx context.Context, id string) (*inventory.Campaign, error)
	ListCampaignsFn             func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
	UpdateCampaignFn            func(ctx context.Context, c *inventory.Campaign) error
	DeleteCampaignFn            func(ctx context.Context, id string) error
	CreatePurchaseFn            func(ctx context.Context, p *inventory.Purchase) error
	GetPurchaseFn               func(ctx context.Context, id string) (*inventory.Purchase, error)
	DeletePurchaseFn            func(ctx context.Context, id string) error
	ListPurchasesByCampaignFn   func(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Purchase, error)
	CreateSaleFn                func(ctx context.Context, s *inventory.Sale, campaign *inventory.Campaign, purchase *inventory.Purchase) error
	CreateBulkSalesFn           func(ctx context.Context, campaignID string, channel inventory.SaleChannel, saleDate string, items []inventory.BulkSaleInput) (*inventory.BulkSaleResult, error)
	ListSalesByCampaignFn       func(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error)
	DeleteSaleByPurchaseIDFn    func(ctx context.Context, purchaseID string) error
	LookupCertFn                func(ctx context.Context, certNumber string) (*inventory.CertInfo, *inventory.MarketSnapshot, error)
	QuickAddPurchaseFn          func(ctx context.Context, campaignID string, req inventory.QuickAddRequest) (*inventory.Purchase, error)
	GetCampaignPNLFn            func(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error)
	GetPNLByChannelFn           func(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error)
	GetDailySpendFn             func(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error)
	GetDaysToSellDistributionFn func(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error)
	GetInventoryAgingFn         func(ctx context.Context, campaignID string) (*inventory.InventoryResult, error)
	GetGlobalInventoryAgingFn   func(ctx context.Context) (*inventory.InventoryResult, error)
	GetFlaggedInventoryFn       func(ctx context.Context) ([]inventory.AgingItem, error)
	RefreshCrackCandidatesFn    func(ctx context.Context) error
	GenerateSellSheetFn         func(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error)
	GenerateGlobalSellSheetFn   func(ctx context.Context) (*inventory.SellSheet, error)
	GenerateSelectedSellSheetFn func(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error)
	GetCampaignTuningFn         func(ctx context.Context, campaignID string) (*inventory.TuningResponse, error)
	RefreshCLValuesGlobalFn     func(ctx context.Context, rows []inventory.CLExportRow) (*inventory.GlobalCLRefreshResult, error)
	ImportCLExportGlobalFn      func(ctx context.Context, rows []inventory.CLExportRow) (*inventory.GlobalImportResult, error)
	ImportPSAExportGlobalFn     func(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error)
	ExportCLFormatGlobalFn      func(ctx context.Context, missingCLOnly bool) ([]inventory.CLExportEntry, error)
	ExportMMFormatGlobalFn      func(ctx context.Context, missingMMOnly bool) ([]inventory.MMExportEntry, error)
	RefreshMMValuesGlobalFn     func(ctx context.Context, rows []inventory.MMRefreshRow) (*inventory.MMRefreshResult, error)
	ReassignPurchaseFn          func(ctx context.Context, purchaseID string, newCampaignID string) error

	// Capital & Invoice
	GetCapitalSummaryFn    func(ctx context.Context) (*inventory.CapitalSummary, error)
	GetCashflowConfigFn    func(ctx context.Context) (*inventory.CashflowConfig, error)
	UpdateCashflowConfigFn func(ctx context.Context, cfg *inventory.CashflowConfig) error
	ListInvoicesFn         func(ctx context.Context) ([]inventory.Invoice, error)
	UpdateInvoiceFn        func(ctx context.Context, inv *inventory.Invoice) error

	// Portfolio health
	GetPortfolioHealthFn          func(ctx context.Context) (*inventory.PortfolioHealth, error)
	GetPortfolioChannelVelocityFn func(ctx context.Context) ([]inventory.ChannelVelocity, error)

	// Portfolio insights & suggestions
	GetPortfolioInsightsFn   func(ctx context.Context) (*inventory.PortfolioInsights, error)
	GetCampaignSuggestionsFn func(ctx context.Context) (*inventory.SuggestionsResponse, error)

	// Revocation
	FlagForRevocationFn       func(ctx context.Context, segmentLabel, segmentDimension, reason string) (*inventory.RevocationFlag, error)
	ListRevocationFlagsFn     func(ctx context.Context) ([]inventory.RevocationFlag, error)
	GenerateRevocationEmailFn func(ctx context.Context, flagID string) (string, error)

	// Capital timeline
	GetCapitalTimelineFn func(ctx context.Context) (*inventory.CapitalTimeline, error)

	// Weekly review
	GetWeeklyReviewSummaryFn func(ctx context.Context) (*inventory.WeeklyReviewSummary, error)

	// Crack arbitrage
	GetCrackCandidatesFn    func(ctx context.Context, campaignID string) ([]inventory.CrackAnalysis, error)
	GetCrackOpportunitiesFn func(ctx context.Context) ([]inventory.CrackAnalysis, error)

	// Acquisition arbitrage
	GetAcquisitionTargetsFn func(ctx context.Context) ([]inventory.AcquisitionOpportunity, error)

	// Expected value
	GetExpectedValuesFn func(ctx context.Context, campaignID string) (*inventory.EVPortfolio, error)
	EvaluatePurchaseFn  func(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*inventory.ExpectedValue, error)

	// Activation checklist
	GetActivationChecklistFn func(ctx context.Context, campaignID string) (*inventory.ActivationChecklist, error)

	// Monte Carlo projection
	RunProjectionFn func(ctx context.Context, campaignID string) (*inventory.MonteCarloComparison, error)

	// Cert entry & eBay export
	ImportCertsFn         func(ctx context.Context, certNumbers []string) (*inventory.CertImportResult, error)
	ListEbayExportItemsFn func(ctx context.Context, flaggedOnly bool) (*inventory.EbayExportListResponse, error)
	GenerateEbayCSVFn     func(ctx context.Context, items []inventory.EbayExportGenerateItem) ([]byte, error)

	// Shopify price sync
	MatchShopifyPricesFn func(ctx context.Context, items []inventory.ShopifyPriceSyncItem) (*inventory.ShopifyPriceSyncResponse, error)

	// Price overrides & AI suggestions
	GetPriceOverrideStatsFn func(ctx context.Context) (*inventory.PriceOverrideStats, error)
	UpdateBuyCostFn         func(ctx context.Context, purchaseID string, buyCostCents int) error
	SetPriceOverrideFn      func(ctx context.Context, purchaseID string, priceCents int, source string) error
	SetAISuggestedPriceFn   func(ctx context.Context, purchaseID string, priceCents int) error
	AcceptAISuggestionFn    func(ctx context.Context, purchaseID string) error
	DismissAISuggestionFn   func(ctx context.Context, purchaseID string) error

	// Price review & flags
	SetReviewedPriceFn     func(ctx context.Context, purchaseID string, priceCents int, source string) error
	GetReviewStatsFn       func(ctx context.Context, campaignID string) (inventory.ReviewStats, error)
	GetGlobalReviewStatsFn func(ctx context.Context) (inventory.ReviewStats, error)
	CreatePriceFlagFn      func(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error)
	ListPriceFlagsFn       func(ctx context.Context, status string) ([]inventory.PriceFlagWithContext, error)
	ResolvePriceFlagFn     func(ctx context.Context, flagID int64, resolvedBy int64) error

	// Snapshot refresh
	RefreshPurchaseSnapshotFn func(ctx context.Context, purchaseID string, card inventory.CardIdentity, grade float64, clValueCents int) bool
	ProcessPendingSnapshotsFn func(ctx context.Context, limit int) (int, int, int)
	RetryFailedSnapshotsFn    func(ctx context.Context, limit int) (int, int, int)

	// External purchases
	EnsureExternalCampaignFn func(ctx context.Context) (*inventory.Campaign, error)
	ImportExternalCSVFn      func(ctx context.Context, rows []inventory.ShopifyExportRow) (*inventory.ExternalImportResult, error)

	// Orders sales import
	ImportOrdersSalesFn  func(ctx context.Context, rows []inventory.OrdersExportRow) (*inventory.OrdersImportResult, error)
	ConfirmOrdersSalesFn func(ctx context.Context, items []inventory.OrdersConfirmItem) (*inventory.BulkSaleResult, error)

	// Cert batch lookup
	GetPurchasesByCertNumbersFn func(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
	ScanCertFn                  func(ctx context.Context, certNumber string) (*inventory.ScanCertResult, error)
	ResolveCertFn               func(ctx context.Context, certNumber string) (*inventory.CertInfo, error)

	// DH push approve & config
	ApproveDHPushFn    func(ctx context.Context, purchaseID string) error
	GetDHPushConfigFn  func(ctx context.Context) (*inventory.DHPushConfig, error)
	SaveDHPushConfigFn func(ctx context.Context, cfg *inventory.DHPushConfig) error
}

var _ inventory.Service = (*MockInventoryService)(nil)

func (m *MockInventoryService) CreateCampaign(ctx context.Context, c *inventory.Campaign) error {
	if m.CreateCampaignFn != nil {
		return m.CreateCampaignFn(ctx, c)
	}
	return nil
}

func (m *MockInventoryService) GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error) {
	if m.GetCampaignFn != nil {
		return m.GetCampaignFn(ctx, id)
	}
	return &inventory.Campaign{ID: id, Name: "mock"}, nil
}

func (m *MockInventoryService) ListCampaigns(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
	if m.ListCampaignsFn != nil {
		return m.ListCampaignsFn(ctx, activeOnly)
	}
	return []inventory.Campaign{}, nil
}

func (m *MockInventoryService) UpdateCampaign(ctx context.Context, c *inventory.Campaign) error {
	if m.UpdateCampaignFn != nil {
		return m.UpdateCampaignFn(ctx, c)
	}
	return nil
}

func (m *MockInventoryService) DeleteCampaign(ctx context.Context, id string) error {
	if m.DeleteCampaignFn != nil {
		return m.DeleteCampaignFn(ctx, id)
	}
	return nil
}

func (m *MockInventoryService) CreatePurchase(ctx context.Context, p *inventory.Purchase) error {
	if m.CreatePurchaseFn != nil {
		return m.CreatePurchaseFn(ctx, p)
	}
	return nil
}

func (m *MockInventoryService) GetPurchase(ctx context.Context, id string) (*inventory.Purchase, error) {
	if m.GetPurchaseFn != nil {
		return m.GetPurchaseFn(ctx, id)
	}
	return &inventory.Purchase{ID: id}, nil
}

func (m *MockInventoryService) DeletePurchase(ctx context.Context, id string) error {
	if m.DeletePurchaseFn != nil {
		return m.DeletePurchaseFn(ctx, id)
	}
	return nil
}

func (m *MockInventoryService) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Purchase, error) {
	if m.ListPurchasesByCampaignFn != nil {
		return m.ListPurchasesByCampaignFn(ctx, campaignID, limit, offset)
	}
	return []inventory.Purchase{}, nil
}

func (m *MockInventoryService) CreateSale(ctx context.Context, s *inventory.Sale, campaign *inventory.Campaign, purchase *inventory.Purchase) error {
	if m.CreateSaleFn != nil {
		return m.CreateSaleFn(ctx, s, campaign, purchase)
	}
	return nil
}

func (m *MockInventoryService) CreateBulkSales(ctx context.Context, campaignID string, channel inventory.SaleChannel, saleDate string, items []inventory.BulkSaleInput) (*inventory.BulkSaleResult, error) {
	if m.CreateBulkSalesFn != nil {
		return m.CreateBulkSalesFn(ctx, campaignID, channel, saleDate, items)
	}
	return &inventory.BulkSaleResult{Created: len(items)}, nil
}

func (m *MockInventoryService) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error) {
	if m.ListSalesByCampaignFn != nil {
		return m.ListSalesByCampaignFn(ctx, campaignID, limit, offset)
	}
	return []inventory.Sale{}, nil
}

func (m *MockInventoryService) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	if m.DeleteSaleByPurchaseIDFn != nil {
		return m.DeleteSaleByPurchaseIDFn(ctx, purchaseID)
	}
	return nil
}

func (m *MockInventoryService) LookupCert(ctx context.Context, certNumber string) (*inventory.CertInfo, *inventory.MarketSnapshot, error) {
	if m.LookupCertFn != nil {
		return m.LookupCertFn(ctx, certNumber)
	}
	return &inventory.CertInfo{CertNumber: certNumber}, nil, nil
}

func (m *MockInventoryService) QuickAddPurchase(ctx context.Context, campaignID string, req inventory.QuickAddRequest) (*inventory.Purchase, error) {
	if m.QuickAddPurchaseFn != nil {
		return m.QuickAddPurchaseFn(ctx, campaignID, req)
	}
	return &inventory.Purchase{CampaignID: campaignID}, nil
}

func (m *MockInventoryService) GetCampaignPNL(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error) {
	if m.GetCampaignPNLFn != nil {
		return m.GetCampaignPNLFn(ctx, campaignID)
	}
	return &inventory.CampaignPNL{CampaignID: campaignID}, nil
}

func (m *MockInventoryService) GetPNLByChannel(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error) {
	if m.GetPNLByChannelFn != nil {
		return m.GetPNLByChannelFn(ctx, campaignID)
	}
	return []inventory.ChannelPNL{}, nil
}

func (m *MockInventoryService) GetDailySpend(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error) {
	if m.GetDailySpendFn != nil {
		return m.GetDailySpendFn(ctx, campaignID, days)
	}
	return []inventory.DailySpend{}, nil
}

func (m *MockInventoryService) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error) {
	if m.GetDaysToSellDistributionFn != nil {
		return m.GetDaysToSellDistributionFn(ctx, campaignID)
	}
	return []inventory.DaysToSellBucket{}, nil
}

func (m *MockInventoryService) GetInventoryAging(ctx context.Context, campaignID string) (*inventory.InventoryResult, error) {
	if m.GetInventoryAgingFn != nil {
		return m.GetInventoryAgingFn(ctx, campaignID)
	}
	return &inventory.InventoryResult{Items: []inventory.AgingItem{}}, nil
}

func (m *MockInventoryService) GetGlobalInventoryAging(ctx context.Context) (*inventory.InventoryResult, error) {
	if m.GetGlobalInventoryAgingFn != nil {
		return m.GetGlobalInventoryAgingFn(ctx)
	}
	return &inventory.InventoryResult{Items: []inventory.AgingItem{}}, nil
}

func (m *MockInventoryService) GetFlaggedInventory(ctx context.Context) ([]inventory.AgingItem, error) {
	if m.GetFlaggedInventoryFn != nil {
		return m.GetFlaggedInventoryFn(ctx)
	}
	return []inventory.AgingItem{}, nil
}

func (m *MockInventoryService) RefreshCrackCandidates(ctx context.Context) error {
	if m.RefreshCrackCandidatesFn != nil {
		return m.RefreshCrackCandidatesFn(ctx)
	}
	return nil
}

func (m *MockInventoryService) GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error) {
	if m.GenerateSellSheetFn != nil {
		return m.GenerateSellSheetFn(ctx, campaignID, purchaseIDs)
	}
	return &inventory.SellSheet{}, nil
}

func (m *MockInventoryService) GenerateGlobalSellSheet(ctx context.Context) (*inventory.SellSheet, error) {
	if m.GenerateGlobalSellSheetFn != nil {
		return m.GenerateGlobalSellSheetFn(ctx)
	}
	return &inventory.SellSheet{}, nil
}

func (m *MockInventoryService) GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error) {
	if m.GenerateSelectedSellSheetFn != nil {
		return m.GenerateSelectedSellSheetFn(ctx, purchaseIDs)
	}
	return &inventory.SellSheet{}, nil
}

func (m *MockInventoryService) GetCampaignTuning(ctx context.Context, campaignID string) (*inventory.TuningResponse, error) {
	if m.GetCampaignTuningFn != nil {
		return m.GetCampaignTuningFn(ctx, campaignID)
	}
	return &inventory.TuningResponse{CampaignID: campaignID}, nil
}

func (m *MockInventoryService) RefreshCLValuesGlobal(ctx context.Context, rows []inventory.CLExportRow) (*inventory.GlobalCLRefreshResult, error) {
	if m.RefreshCLValuesGlobalFn != nil {
		return m.RefreshCLValuesGlobalFn(ctx, rows)
	}
	return &inventory.GlobalCLRefreshResult{}, nil
}

func (m *MockInventoryService) ImportCLExportGlobal(ctx context.Context, rows []inventory.CLExportRow) (*inventory.GlobalImportResult, error) {
	if m.ImportCLExportGlobalFn != nil {
		return m.ImportCLExportGlobalFn(ctx, rows)
	}
	return &inventory.GlobalImportResult{}, nil
}

func (m *MockInventoryService) ExportCLFormatGlobal(ctx context.Context, missingCLOnly bool) ([]inventory.CLExportEntry, error) {
	if m.ExportCLFormatGlobalFn != nil {
		return m.ExportCLFormatGlobalFn(ctx, missingCLOnly)
	}
	return []inventory.CLExportEntry{}, nil
}

func (m *MockInventoryService) ExportMMFormatGlobal(ctx context.Context, missingMMOnly bool) ([]inventory.MMExportEntry, error) {
	if m.ExportMMFormatGlobalFn != nil {
		return m.ExportMMFormatGlobalFn(ctx, missingMMOnly)
	}
	return []inventory.MMExportEntry{}, nil
}

func (m *MockInventoryService) RefreshMMValuesGlobal(ctx context.Context, rows []inventory.MMRefreshRow) (*inventory.MMRefreshResult, error) {
	if m.RefreshMMValuesGlobalFn != nil {
		return m.RefreshMMValuesGlobalFn(ctx, rows)
	}
	return &inventory.MMRefreshResult{}, nil
}

func (m *MockInventoryService) ImportPSAExportGlobal(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
	if m.ImportPSAExportGlobalFn != nil {
		return m.ImportPSAExportGlobalFn(ctx, rows)
	}
	return &inventory.PSAImportResult{}, nil
}

func (m *MockInventoryService) ReassignPurchase(ctx context.Context, purchaseID string, newCampaignID string) error {
	if m.ReassignPurchaseFn != nil {
		return m.ReassignPurchaseFn(ctx, purchaseID, newCampaignID)
	}
	return nil
}

func (m *MockInventoryService) GetCapitalSummary(ctx context.Context) (*inventory.CapitalSummary, error) {
	if m.GetCapitalSummaryFn != nil {
		return m.GetCapitalSummaryFn(ctx)
	}
	return &inventory.CapitalSummary{OutstandingCents: 0, WeeksToCover: inventory.WeeksToCoverNoData, AlertLevel: inventory.AlertOK}, nil
}

func (m *MockInventoryService) GetCashflowConfig(ctx context.Context) (*inventory.CashflowConfig, error) {
	if m.GetCashflowConfigFn != nil {
		return m.GetCashflowConfigFn(ctx)
	}
	return &inventory.CashflowConfig{CapitalBudgetCents: 5000000, CashBufferCents: 1000000}, nil
}

func (m *MockInventoryService) UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error {
	if m.UpdateCashflowConfigFn != nil {
		return m.UpdateCashflowConfigFn(ctx, cfg)
	}
	return nil
}

func (m *MockInventoryService) ListInvoices(ctx context.Context) ([]inventory.Invoice, error) {
	if m.ListInvoicesFn != nil {
		return m.ListInvoicesFn(ctx)
	}
	return []inventory.Invoice{}, nil
}

func (m *MockInventoryService) UpdateInvoice(ctx context.Context, inv *inventory.Invoice) error {
	if m.UpdateInvoiceFn != nil {
		return m.UpdateInvoiceFn(ctx, inv)
	}
	return nil
}

func (m *MockInventoryService) GetPortfolioHealth(ctx context.Context) (*inventory.PortfolioHealth, error) {
	if m.GetPortfolioHealthFn != nil {
		return m.GetPortfolioHealthFn(ctx)
	}
	return &inventory.PortfolioHealth{}, nil
}

func (m *MockInventoryService) GetPortfolioChannelVelocity(ctx context.Context) ([]inventory.ChannelVelocity, error) {
	if m.GetPortfolioChannelVelocityFn != nil {
		return m.GetPortfolioChannelVelocityFn(ctx)
	}
	return []inventory.ChannelVelocity{}, nil
}

func (m *MockInventoryService) GetPortfolioInsights(ctx context.Context) (*inventory.PortfolioInsights, error) {
	if m.GetPortfolioInsightsFn != nil {
		return m.GetPortfolioInsightsFn(ctx)
	}
	return &inventory.PortfolioInsights{}, nil
}

func (m *MockInventoryService) GetCampaignSuggestions(ctx context.Context) (*inventory.SuggestionsResponse, error) {
	if m.GetCampaignSuggestionsFn != nil {
		return m.GetCampaignSuggestionsFn(ctx)
	}
	return &inventory.SuggestionsResponse{}, nil
}

// Revocation

func (m *MockInventoryService) FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*inventory.RevocationFlag, error) {
	if m.FlagForRevocationFn != nil {
		return m.FlagForRevocationFn(ctx, segmentLabel, segmentDimension, reason)
	}
	return &inventory.RevocationFlag{}, nil
}

func (m *MockInventoryService) ListRevocationFlags(ctx context.Context) ([]inventory.RevocationFlag, error) {
	if m.ListRevocationFlagsFn != nil {
		return m.ListRevocationFlagsFn(ctx)
	}
	return []inventory.RevocationFlag{}, nil
}

func (m *MockInventoryService) GenerateRevocationEmail(ctx context.Context, flagID string) (string, error) {
	if m.GenerateRevocationEmailFn != nil {
		return m.GenerateRevocationEmailFn(ctx, flagID)
	}
	return "", nil
}

// Capital timeline

func (m *MockInventoryService) GetCapitalTimeline(ctx context.Context) (*inventory.CapitalTimeline, error) {
	if m.GetCapitalTimelineFn != nil {
		return m.GetCapitalTimelineFn(ctx)
	}
	return &inventory.CapitalTimeline{}, nil
}

// Weekly review

func (m *MockInventoryService) GetWeeklyReviewSummary(ctx context.Context) (*inventory.WeeklyReviewSummary, error) {
	if m.GetWeeklyReviewSummaryFn != nil {
		return m.GetWeeklyReviewSummaryFn(ctx)
	}
	return &inventory.WeeklyReviewSummary{}, nil
}

// Crack arbitrage

func (m *MockInventoryService) GetCrackCandidates(ctx context.Context, campaignID string) ([]inventory.CrackAnalysis, error) {
	if m.GetCrackCandidatesFn != nil {
		return m.GetCrackCandidatesFn(ctx, campaignID)
	}
	return []inventory.CrackAnalysis{}, nil
}

func (m *MockInventoryService) GetCrackOpportunities(ctx context.Context) ([]inventory.CrackAnalysis, error) {
	if m.GetCrackOpportunitiesFn != nil {
		return m.GetCrackOpportunitiesFn(ctx)
	}
	return []inventory.CrackAnalysis{}, nil
}

func (m *MockInventoryService) GetAcquisitionTargets(ctx context.Context) ([]inventory.AcquisitionOpportunity, error) {
	if m.GetAcquisitionTargetsFn != nil {
		return m.GetAcquisitionTargetsFn(ctx)
	}
	return []inventory.AcquisitionOpportunity{}, nil
}

// Expected value

func (m *MockInventoryService) GetExpectedValues(ctx context.Context, campaignID string) (*inventory.EVPortfolio, error) {
	if m.GetExpectedValuesFn != nil {
		return m.GetExpectedValuesFn(ctx, campaignID)
	}
	return &inventory.EVPortfolio{}, nil
}

func (m *MockInventoryService) EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*inventory.ExpectedValue, error) {
	if m.EvaluatePurchaseFn != nil {
		return m.EvaluatePurchaseFn(ctx, campaignID, cardName, grade, buyCostCents)
	}
	return &inventory.ExpectedValue{}, nil
}

// Activation checklist

func (m *MockInventoryService) GetActivationChecklist(ctx context.Context, campaignID string) (*inventory.ActivationChecklist, error) {
	if m.GetActivationChecklistFn != nil {
		return m.GetActivationChecklistFn(ctx, campaignID)
	}
	return &inventory.ActivationChecklist{CampaignID: campaignID, AllPassed: true}, nil
}

// Monte Carlo projection

func (m *MockInventoryService) RunProjection(ctx context.Context, campaignID string) (*inventory.MonteCarloComparison, error) {
	if m.RunProjectionFn != nil {
		return m.RunProjectionFn(ctx, campaignID)
	}
	return &inventory.MonteCarloComparison{Confidence: "insufficient"}, nil
}

// Cert entry & eBay export

func (m *MockInventoryService) ImportCerts(ctx context.Context, certNumbers []string) (*inventory.CertImportResult, error) {
	if m.ImportCertsFn != nil {
		return m.ImportCertsFn(ctx, certNumbers)
	}
	return nil, nil
}

func (m *MockInventoryService) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*inventory.EbayExportListResponse, error) {
	if m.ListEbayExportItemsFn != nil {
		return m.ListEbayExportItemsFn(ctx, flaggedOnly)
	}
	return nil, nil
}

func (m *MockInventoryService) GenerateEbayCSV(ctx context.Context, items []inventory.EbayExportGenerateItem) ([]byte, error) {
	if m.GenerateEbayCSVFn != nil {
		return m.GenerateEbayCSVFn(ctx, items)
	}
	return nil, nil
}

// Shopify price sync

func (m *MockInventoryService) MatchShopifyPrices(ctx context.Context, items []inventory.ShopifyPriceSyncItem) (*inventory.ShopifyPriceSyncResponse, error) {
	if m.MatchShopifyPricesFn != nil {
		return m.MatchShopifyPricesFn(ctx, items)
	}
	return &inventory.ShopifyPriceSyncResponse{Matched: []inventory.ShopifyPriceSyncMatch{}, Unmatched: []string{}}, nil
}

// External purchases

func (m *MockInventoryService) EnsureExternalCampaign(ctx context.Context) (*inventory.Campaign, error) {
	if m.EnsureExternalCampaignFn != nil {
		return m.EnsureExternalCampaignFn(ctx)
	}
	return &inventory.Campaign{ID: inventory.ExternalCampaignID, Name: inventory.ExternalCampaignName}, nil
}

func (m *MockInventoryService) ImportExternalCSV(ctx context.Context, rows []inventory.ShopifyExportRow) (*inventory.ExternalImportResult, error) {
	if m.ImportExternalCSVFn != nil {
		return m.ImportExternalCSVFn(ctx, rows)
	}
	return &inventory.ExternalImportResult{}, nil
}

func (m *MockInventoryService) ImportOrdersSales(ctx context.Context, rows []inventory.OrdersExportRow) (*inventory.OrdersImportResult, error) {
	if m.ImportOrdersSalesFn != nil {
		return m.ImportOrdersSalesFn(ctx, rows)
	}
	return &inventory.OrdersImportResult{}, nil
}

func (m *MockInventoryService) ConfirmOrdersSales(ctx context.Context, items []inventory.OrdersConfirmItem) (*inventory.BulkSaleResult, error) {
	if m.ConfirmOrdersSalesFn != nil {
		return m.ConfirmOrdersSalesFn(ctx, items)
	}
	return &inventory.BulkSaleResult{}, nil
}

// Price overrides & AI suggestions

func (m *MockInventoryService) GetPriceOverrideStats(ctx context.Context) (*inventory.PriceOverrideStats, error) {
	if m.GetPriceOverrideStatsFn != nil {
		return m.GetPriceOverrideStatsFn(ctx)
	}
	return &inventory.PriceOverrideStats{}, nil
}

func (m *MockInventoryService) UpdateBuyCost(ctx context.Context, purchaseID string, buyCostCents int) error {
	if m.UpdateBuyCostFn != nil {
		return m.UpdateBuyCostFn(ctx, purchaseID, buyCostCents)
	}
	return nil
}

func (m *MockInventoryService) SetPriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.SetPriceOverrideFn != nil {
		return m.SetPriceOverrideFn(ctx, purchaseID, priceCents, source)
	}
	return nil
}

func (m *MockInventoryService) SetAISuggestedPrice(ctx context.Context, purchaseID string, priceCents int) error {
	if m.SetAISuggestedPriceFn != nil {
		return m.SetAISuggestedPriceFn(ctx, purchaseID, priceCents)
	}
	return nil
}

func (m *MockInventoryService) AcceptAISuggestion(ctx context.Context, purchaseID string) error {
	if m.AcceptAISuggestionFn != nil {
		return m.AcceptAISuggestionFn(ctx, purchaseID)
	}
	return nil
}

func (m *MockInventoryService) DismissAISuggestion(ctx context.Context, purchaseID string) error {
	if m.DismissAISuggestionFn != nil {
		return m.DismissAISuggestionFn(ctx, purchaseID)
	}
	return nil
}

// Price review & flags

func (m *MockInventoryService) SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.SetReviewedPriceFn != nil {
		return m.SetReviewedPriceFn(ctx, purchaseID, priceCents, source)
	}
	return nil
}

func (m *MockInventoryService) GetReviewStats(ctx context.Context, campaignID string) (inventory.ReviewStats, error) {
	if m.GetReviewStatsFn != nil {
		return m.GetReviewStatsFn(ctx, campaignID)
	}
	return inventory.ReviewStats{}, nil
}

func (m *MockInventoryService) GetGlobalReviewStats(ctx context.Context) (inventory.ReviewStats, error) {
	if m.GetGlobalReviewStatsFn != nil {
		return m.GetGlobalReviewStatsFn(ctx)
	}
	return inventory.ReviewStats{}, nil
}

func (m *MockInventoryService) CreatePriceFlag(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error) {
	if m.CreatePriceFlagFn != nil {
		return m.CreatePriceFlagFn(ctx, purchaseID, userID, reason)
	}
	return 0, nil
}

func (m *MockInventoryService) ListPriceFlags(ctx context.Context, status string) ([]inventory.PriceFlagWithContext, error) {
	if m.ListPriceFlagsFn != nil {
		return m.ListPriceFlagsFn(ctx, status)
	}
	return []inventory.PriceFlagWithContext{}, nil
}

func (m *MockInventoryService) ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error {
	if m.ResolvePriceFlagFn != nil {
		return m.ResolvePriceFlagFn(ctx, flagID, resolvedBy)
	}
	return nil
}

func (m *MockInventoryService) RefreshPurchaseSnapshot(ctx context.Context, purchaseID string, card inventory.CardIdentity, grade float64, clValueCents int) bool {
	if m.RefreshPurchaseSnapshotFn != nil {
		return m.RefreshPurchaseSnapshotFn(ctx, purchaseID, card, grade, clValueCents)
	}
	return false
}

func (m *MockInventoryService) ProcessPendingSnapshots(ctx context.Context, limit int) (int, int, int) {
	if m.ProcessPendingSnapshotsFn != nil {
		return m.ProcessPendingSnapshotsFn(ctx, limit)
	}
	return 0, 0, 0
}

func (m *MockInventoryService) RetryFailedSnapshots(ctx context.Context, limit int) (int, int, int) {
	if m.RetryFailedSnapshotsFn != nil {
		return m.RetryFailedSnapshotsFn(ctx, limit)
	}
	return 0, 0, 0
}

func (m *MockInventoryService) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if m.GetPurchasesByCertNumbersFn != nil {
		return m.GetPurchasesByCertNumbersFn(ctx, certNumbers)
	}
	return map[string]*inventory.Purchase{}, nil
}

func (m *MockInventoryService) ScanCert(ctx context.Context, certNumber string) (*inventory.ScanCertResult, error) {
	if m.ScanCertFn != nil {
		return m.ScanCertFn(ctx, certNumber)
	}
	return &inventory.ScanCertResult{Status: "new"}, nil
}

func (m *MockInventoryService) ResolveCert(ctx context.Context, certNumber string) (*inventory.CertInfo, error) {
	if m.ResolveCertFn != nil {
		return m.ResolveCertFn(ctx, certNumber)
	}
	return nil, nil
}

// DH push approve & config

func (m *MockInventoryService) ApproveDHPush(ctx context.Context, purchaseID string) error {
	if m.ApproveDHPushFn != nil {
		return m.ApproveDHPushFn(ctx, purchaseID)
	}
	return nil
}

func (m *MockInventoryService) GetDHPushConfig(ctx context.Context) (*inventory.DHPushConfig, error) {
	if m.GetDHPushConfigFn != nil {
		return m.GetDHPushConfigFn(ctx)
	}
	return &inventory.DHPushConfig{}, nil
}

func (m *MockInventoryService) SaveDHPushConfig(ctx context.Context, cfg *inventory.DHPushConfig) error {
	if m.SaveDHPushConfigFn != nil {
		return m.SaveDHPushConfigFn(ctx, cfg)
	}
	return nil
}

func (m *MockInventoryService) Close() {}
