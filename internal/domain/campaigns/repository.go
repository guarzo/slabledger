package campaigns

import (
	"context"
	"time"
)

// CampaignCRUD handles campaign lifecycle operations.
type CampaignCRUD interface {
	CreateCampaign(ctx context.Context, c *Campaign) error
	GetCampaign(ctx context.Context, id string) (*Campaign, error)
	ListCampaigns(ctx context.Context, activeOnly bool) ([]Campaign, error)
	UpdateCampaign(ctx context.Context, c *Campaign) error
	DeleteCampaign(ctx context.Context, id string) error
}

// PurchaseRepository handles purchase persistence, lookups, and field updates.
type PurchaseRepository interface {
	// CRUD
	CreatePurchase(ctx context.Context, p *Purchase) error
	GetPurchase(ctx context.Context, id string) (*Purchase, error)
	DeletePurchase(ctx context.Context, id string) error
	ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Purchase, error)
	ListUnsoldPurchases(ctx context.Context, campaignID string) ([]Purchase, error)
	ListAllUnsoldPurchases(ctx context.Context) ([]Purchase, error)
	CountPurchasesByCampaign(ctx context.Context, campaignID string) (int, error)

	// Cert-based lookups
	GetPurchaseByCertNumber(ctx context.Context, grader string, certNumber string) (*Purchase, error)
	GetPurchasesByGraderAndCertNumbers(ctx context.Context, grader string, certNumbers []string) (map[string]*Purchase, error)
	GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*Purchase, error)
	GetPurchasesByIDs(ctx context.Context, ids []string) (map[string]*Purchase, error)

	// Field updates
	UpdatePurchaseCLValue(ctx context.Context, id string, clValueCents int, population int) error
	UpdatePurchaseCardMetadata(ctx context.Context, id string, cardName, cardNumber, setName string) error
	UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error
	UpdateExternalPurchaseFields(ctx context.Context, id string, p *Purchase) error
	UpdatePurchaseMarketSnapshot(ctx context.Context, id string, snap MarketSnapshotData) error
	UpdatePurchaseCampaign(ctx context.Context, purchaseID string, campaignID string, sourcingFeeCents int) error
	UpdatePurchasePSAFields(ctx context.Context, id string, fields PSAUpdateFields) error

	// Buy cost correction
	UpdatePurchaseBuyCost(ctx context.Context, id string, buyCostCents int) error

	// Price overrides & AI suggestions
	UpdatePurchasePriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error
	UpdatePurchaseAISuggestion(ctx context.Context, purchaseID string, priceCents int) error
	ClearPurchaseAISuggestion(ctx context.Context, purchaseID string) error
	AcceptAISuggestion(ctx context.Context, purchaseID string, priceCents int) error
	GetPriceOverrideStats(ctx context.Context) (*PriceOverrideStats, error)

	// eBay export
	SetEbayExportFlag(ctx context.Context, purchaseID string, flaggedAt time.Time) error
	ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error
	ListEbayFlaggedPurchases(ctx context.Context) ([]Purchase, error)
	UpdatePurchaseCardYear(ctx context.Context, id string, year string) error

	// Snapshot status
	ListSnapshotPurchasesByStatus(ctx context.Context, status SnapshotStatus, limit int) ([]Purchase, error)
	UpdatePurchaseSnapshotStatus(ctx context.Context, id string, status SnapshotStatus, retryCount int) error

	// DH v2 fields
	UpdatePurchaseDHFields(ctx context.Context, id string, update DHFieldsUpdate) error
	GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]Purchase, error)
	UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error
	GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]Purchase, error)
}

// SaleRepository handles sale persistence.
type SaleRepository interface {
	CreateSale(ctx context.Context, s *Sale) error
	GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*Sale, error)
	GetSalesByPurchaseIDs(ctx context.Context, purchaseIDs []string) (map[string]*Sale, error)
	ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Sale, error)
	DeleteSale(ctx context.Context, saleID string) error
	DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error
}

// AnalyticsRepository handles analytics and reporting queries.
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

// FinanceRepository handles invoices, cashflow config, and credit summaries.
type FinanceRepository interface {
	CreateInvoice(ctx context.Context, inv *Invoice) error
	GetInvoice(ctx context.Context, id string) (*Invoice, error)
	ListInvoices(ctx context.Context) ([]Invoice, error)
	UpdateInvoice(ctx context.Context, inv *Invoice) error
	SumPurchaseCostByInvoiceDate(ctx context.Context, invoiceDate string) (int, error)
	GetCashflowConfig(ctx context.Context) (*CashflowConfig, error)
	UpdateCashflowConfig(ctx context.Context, cfg *CashflowConfig) error
	GetCreditSummary(ctx context.Context) (*CreditSummary, error)
}

// RevocationRepository handles revocation flag management.
type RevocationRepository interface {
	CreateRevocationFlag(ctx context.Context, flag *RevocationFlag) error
	ListRevocationFlags(ctx context.Context) ([]RevocationFlag, error)
	GetLatestRevocationFlag(ctx context.Context) (*RevocationFlag, error)
	UpdateRevocationFlagStatus(ctx context.Context, id string, status string, sentAt *time.Time) error
}

// PriceReviewRepository handles price review and flag persistence.
type PriceReviewRepository interface {
	UpdateReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error
	GetReviewStats(ctx context.Context, campaignID string) (ReviewStats, error)
	GetGlobalReviewStats(ctx context.Context) (ReviewStats, error)
	CreatePriceFlag(ctx context.Context, flag *PriceFlag) (int64, error)
	ListPriceFlags(ctx context.Context, status string) ([]PriceFlagWithContext, error)
	ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error
	HasOpenFlag(ctx context.Context, purchaseID string) (bool, error)
	OpenFlagPurchaseIDs(ctx context.Context) (map[string]bool, error)
}

// SellSheetRepository handles sell sheet item persistence.
type SellSheetRepository interface {
	GetSellSheetItems(ctx context.Context, userID int64) ([]string, error)
	AddSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error
	RemoveSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error
	ClearSellSheet(ctx context.Context, userID int64) error
}

// Repository is the composed interface for all campaign persistence.
// New code should prefer accepting specific sub-interfaces where possible.
type Repository interface {
	CampaignCRUD
	PurchaseRepository
	SaleRepository
	AnalyticsRepository
	FinanceRepository
	RevocationRepository
	PriceReviewRepository
	SellSheetRepository
}

// DHFieldsUpdate contains the DH v2 tracking fields to update on a purchase.
type DHFieldsUpdate struct {
	CardID            int
	InventoryID       int
	CertStatus        string
	ListingPriceCents int
	ChannelsJSON      string
	DHStatus          DHStatus
}

// PSAUpdateFields contains the PSA-specific fields that can be updated on an existing purchase.
type PSAUpdateFields struct {
	VaultStatus     string
	InvoiceDate     string
	WasRefunded     bool
	FrontImageURL   string
	BackImageURL    string
	PurchaseSource  string
	PSAListingTitle string
}
