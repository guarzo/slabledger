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
	ImportPSAExportGlobalFn     func(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error)
	ExportMMFormatGlobalFn      func(ctx context.Context, missingMMOnly bool) ([]inventory.MMExportEntry, error)
	RefreshMMValuesGlobalFn     func(ctx context.Context, rows []inventory.MMRefreshRow) (*inventory.MMRefreshResult, error)
	ReassignPurchaseFn          func(ctx context.Context, purchaseID string, newCampaignID string) error

	// Capital & Invoice
	GetCapitalSummaryFn    func(ctx context.Context) (*inventory.CapitalSummary, error)
	GetCashflowConfigFn    func(ctx context.Context) (*inventory.CashflowConfig, error)
	UpdateCashflowConfigFn func(ctx context.Context, cfg *inventory.CashflowConfig) error
	ListInvoicesFn         func(ctx context.Context) ([]inventory.Invoice, error)
	UpdateInvoiceFn        func(ctx context.Context, inv *inventory.Invoice) error

	// Revocation
	FlagForRevocationFn       func(ctx context.Context, segmentLabel, segmentDimension, reason string) (*inventory.RevocationFlag, error)
	ListRevocationFlagsFn     func(ctx context.Context) ([]inventory.RevocationFlag, error)
	GenerateRevocationEmailFn func(ctx context.Context, flagID string) (string, error)

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
	ProcessPendingSnapshotsFn func(ctx context.Context, limit int) (int, int, int, error)
	RetryFailedSnapshotsFn    func(ctx context.Context, limit int) (int, int, int, error)

	// External purchases
	EnsureExternalCampaignFn func(ctx context.Context) (*inventory.Campaign, error)
	ImportExternalCSVFn      func(ctx context.Context, rows []inventory.ShopifyExportRow) (*inventory.ExternalImportResult, error)

	// Orders sales import
	ImportOrdersSalesFn  func(ctx context.Context, rows []inventory.OrdersExportRow) (*inventory.OrdersImportResult, error)
	ConfirmOrdersSalesFn func(ctx context.Context, items []inventory.OrdersConfirmItem) (*inventory.BulkSaleResult, error)

	// Cert batch lookup
	ImportCertsFn               func(ctx context.Context, certNumbers []string) (*inventory.CertImportResult, error)
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

func (m *MockInventoryService) ProcessPendingSnapshots(ctx context.Context, limit int) (int, int, int, error) {
	if m.ProcessPendingSnapshotsFn != nil {
		return m.ProcessPendingSnapshotsFn(ctx, limit)
	}
	return 0, 0, 0, nil
}

func (m *MockInventoryService) RetryFailedSnapshots(ctx context.Context, limit int) (int, int, int, error) {
	if m.RetryFailedSnapshotsFn != nil {
		return m.RetryFailedSnapshotsFn(ctx, limit)
	}
	return 0, 0, 0, nil
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

func (m *MockInventoryService) ImportCerts(ctx context.Context, certNumbers []string) (*inventory.CertImportResult, error) {
	if m.ImportCertsFn != nil {
		return m.ImportCertsFn(ctx, certNumbers)
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
