package campaigns

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
	GetCampaignTuning(ctx context.Context, campaignID string) (*TuningResponse, error)

	GetPortfolioHealth(ctx context.Context) (*PortfolioHealth, error)
	GetPortfolioChannelVelocity(ctx context.Context) ([]ChannelVelocity, error)
	GetPortfolioInsights(ctx context.Context) (*PortfolioInsights, error)
	GetCampaignSuggestions(ctx context.Context) (*SuggestionsResponse, error)
	GetCapitalTimeline(ctx context.Context) (*CapitalTimeline, error)
	GetWeeklyReviewSummary(ctx context.Context) (*WeeklyReviewSummary, error)

	GetCrackCandidates(ctx context.Context, campaignID string) ([]CrackAnalysis, error)
	GetCrackOpportunities(ctx context.Context) ([]CrackAnalysis, error)
	GetAcquisitionTargets(ctx context.Context) ([]AcquisitionOpportunity, error)
	GetActivationChecklist(ctx context.Context, campaignID string) (*ActivationChecklist, error)

	GetExpectedValues(ctx context.Context, campaignID string) (*EVPortfolio, error)
	EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*ExpectedValue, error)
	RunProjection(ctx context.Context, campaignID string) (*MonteCarloComparison, error)

	GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*SellSheet, error)
	GenerateGlobalSellSheet(ctx context.Context) (*SellSheet, error)
	GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*SellSheet, error)

	ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*EbayExportListResponse, error)
	GenerateEbayCSV(ctx context.Context, items []EbayExportGenerateItem) ([]byte, error)

	MatchShopifyPrices(ctx context.Context, items []ShopifyPriceSyncItem) (*ShopifyPriceSyncResponse, error)
}

// ImportService handles CSV imports, cert entry, and external data ingestion.
type ImportService interface {
	RefreshCLValuesGlobal(ctx context.Context, rows []CLExportRow) (*GlobalCLRefreshResult, error)
	ImportCLExportGlobal(ctx context.Context, rows []CLExportRow) (*GlobalImportResult, error)
	ImportPSAExportGlobal(ctx context.Context, rows []PSAExportRow) (*PSAImportResult, error)
	ExportCLFormatGlobal(ctx context.Context, missingCLOnly bool) ([]CLExportEntry, error)

	EnsureExternalCampaign(ctx context.Context) (*Campaign, error)
	ImportExternalCSV(ctx context.Context, rows []ShopifyExportRow) (*ExternalImportResult, error)

	ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error)
	ConfirmOrdersSales(ctx context.Context, items []OrdersConfirmItem) (*BulkSaleResult, error)

	ImportCerts(ctx context.Context, certNumbers []string) (*CertImportResult, error)
	GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*Purchase, error)
	ScanCert(ctx context.Context, certNumber string) (*ScanCertResult, error)
	ResolveCert(ctx context.Context, certNumber string) (*CertInfo, error)
}

// FinanceService handles capital, invoices, and revocation operations.
type FinanceService interface {
	GetCapitalSummary(ctx context.Context) (*CapitalSummary, error)
	GetCashflowConfig(ctx context.Context) (*CashflowConfig, error)
	ListInvoices(ctx context.Context) ([]Invoice, error)
	UpdateInvoice(ctx context.Context, inv *Invoice) error

	FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*RevocationFlag, error)
	ListRevocationFlags(ctx context.Context) ([]RevocationFlag, error)
	GenerateRevocationEmail(ctx context.Context, flagID string) (string, error)
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
	ProcessPendingSnapshots(ctx context.Context, limit int) (processed, skipped, failed int)
	RetryFailedSnapshots(ctx context.Context, limit int) (processed, skipped, failed int)
}
