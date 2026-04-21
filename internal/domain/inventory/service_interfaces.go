package inventory

import "context"

// CRUDService handles campaign, purchase, and sale CRUD operations.
type CRUDService interface {
	CreateCampaign(ctx context.Context, c *Campaign) error
	GetCampaign(ctx context.Context, id string) (*Campaign, error)
	ListCampaigns(ctx context.Context, activeOnly bool) ([]Campaign, error)
	UpdateCampaign(ctx context.Context, c *Campaign) error
	DeleteCampaign(ctx context.Context, id string) error

	CreatePurchase(ctx context.Context, p *Purchase) error
	GetPurchase(ctx context.Context, id string) (*Purchase, error)
	DeletePurchase(ctx context.Context, id string) error
	ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Purchase, error)

	CreateSale(ctx context.Context, s *Sale, campaign *Campaign, purchase *Purchase) error
	CreateBulkSales(ctx context.Context, campaignID string, channel SaleChannel, saleDate string, items []BulkSaleInput) (*BulkSaleResult, error)
	ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Sale, error)
	DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error

	ReassignPurchase(ctx context.Context, purchaseID string, newCampaignID string) error
}

// AnalyticsService provides read-only analytics, portfolio, and reporting operations.
type AnalyticsService interface {
	GetCampaignPNL(ctx context.Context, campaignID string) (*CampaignPNL, error)
	GetPNLByChannel(ctx context.Context, campaignID string) ([]ChannelPNL, error)
	GetDailySpend(ctx context.Context, campaignID string, days int) ([]DailySpend, error)
	GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]DaysToSellBucket, error)
	GetInventoryAging(ctx context.Context, campaignID string) (*InventoryResult, error)
	GetGlobalInventoryAging(ctx context.Context) (*InventoryResult, error)
	GetFlaggedInventory(ctx context.Context) ([]AgingItem, error)
	// RefreshCrackCandidates refreshes the crack candidate cache. Called by the
	// crack cache refresh scheduler; also available for on-demand refresh.
	RefreshCrackCandidates(ctx context.Context) error
}

// ImportService handles CSV imports, cert entry, and external data ingestion.
type ImportService interface {
	ImportPSAExportGlobal(ctx context.Context, rows []PSAExportRow) (*PSAImportResult, error)
	ExportMMFormatGlobal(ctx context.Context, missingMMOnly bool) ([]MMExportEntry, error)
	RefreshMMValuesGlobal(ctx context.Context, rows []MMRefreshRow) (*MMRefreshResult, error)

	EnsureExternalCampaign(ctx context.Context) (*Campaign, error)
	ImportExternalCSV(ctx context.Context, rows []ShopifyExportRow) (*ExternalImportResult, error)

	ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error)
	ConfirmOrdersSales(ctx context.Context, items []OrdersConfirmItem) (*BulkSaleResult, error)

	ImportCerts(ctx context.Context, certNumbers []string) (*CertImportResult, error)
	GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*Purchase, error)
	ScanCert(ctx context.Context, certNumber string) (*ScanCertResult, error)
	ScanCerts(ctx context.Context, certNumbers []string) (*ScanCertsResult, error)
	ResolveCert(ctx context.Context, certNumber string) (*CertInfo, error)
}

// PricingService handles price overrides, AI suggestions, review, and flags.
type PricingService interface {
	UpdateBuyCost(ctx context.Context, purchaseID string, buyCostCents int) error
	SetPriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error
	SetAISuggestedPrice(ctx context.Context, purchaseID string, priceCents int) error
	AcceptAISuggestion(ctx context.Context, purchaseID string) error
	DismissAISuggestion(ctx context.Context, purchaseID string) error
	GetPriceOverrideStats(ctx context.Context) (*PriceOverrideStats, error)

	SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error
	GetReviewStats(ctx context.Context, campaignID string) (ReviewStats, error)
	GetGlobalReviewStats(ctx context.Context) (ReviewStats, error)
	CreatePriceFlag(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error)
	ListPriceFlags(ctx context.Context, status string) ([]PriceFlagWithContext, error)
	ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error
}

// DHService handles DH push approval and configuration.
type DHService interface {
	ApproveDHPush(ctx context.Context, purchaseID string) error
	GetDHPushConfig(ctx context.Context) (*DHPushConfig, error)
	SaveDHPushConfig(ctx context.Context, cfg *DHPushConfig) error
}

// CertLookupService handles certificate lookup and quick-add operations.
type CertLookupService interface {
	LookupCert(ctx context.Context, certNumber string) (*CertInfo, *MarketSnapshot, error)
	QuickAddPurchase(ctx context.Context, campaignID string, req QuickAddRequest) (*Purchase, error)
}

// SnapshotService handles background market snapshot refresh (used by schedulers).
type SnapshotService interface {
	RefreshPurchaseSnapshot(ctx context.Context, purchaseID string, card CardIdentity, grade float64, clValueCents int) bool
	ProcessPendingSnapshots(ctx context.Context, limit int) (processed, skipped, failed int, err error)
	RetryFailedSnapshots(ctx context.Context, limit int) (processed, skipped, failed int, err error)
}
