package inventory

import (
	"context"
)

// Every interface in this file has at least one non-test consumer that depends
// on it instead of the full Service union. An interface here that loses its last
// such consumer should be inlined into Service rather than left standing as a
// split that buys no seam (SLA-36). Campaign/purchase/sale CRUD is declared
// directly on Service for exactly that reason: no consumer wants it alone.

// AnalyticsService provides read-only analytics, portfolio, and reporting operations.
type AnalyticsService interface {
	GetCampaignPNL(ctx context.Context, campaignID string) (*CampaignPNL, error)
	GetPNLByChannel(ctx context.Context, campaignID string) ([]ChannelPNL, error)
	GetDailySpend(ctx context.Context, campaignID string, days int) ([]DailySpend, error)
	GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]DaysToSellBucket, error)
	GetInventoryAging(ctx context.Context, campaignID string) (*InventoryResult, error)
	GetGlobalInventoryAging(ctx context.Context) (*InventoryResult, error)
	GetFlaggedInventory(ctx context.Context) ([]AgingItem, error)
}

// ImportService handles CSV imports, cert entry, and external data ingestion.
type ImportService interface {
	ImportPSAExportGlobal(ctx context.Context, rows []PSAExportRow) (*PSAImportResult, error)
	ReconcilePSAAttribution(ctx context.Context, rows []PSAExportRow) (ReconcileResult, error)

	EnsureExternalCampaign(ctx context.Context) (*Campaign, error)
	ImportExternalCSV(ctx context.Context, rows []ShopifyExportRow) (*ExternalImportResult, error)

	ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error)
	ImportEbayOrdersSales(ctx context.Context, rows []EbayOrderRow) (*OrdersImportResult, error)
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
