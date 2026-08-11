package inventory

import (
	"context"
	"time"
)

// PSAUpdateFields contains the PSA-specific fields that can be updated on an existing purchase.
type PSAUpdateFields struct {
	PSAShipDate     string
	InvoiceDate     string
	WasRefunded     bool
	FrontImageURL   string
	BackImageURL    string
	PurchaseSource  string
	PSAListingTitle string
}

// PurchaseCoreRepository handles purchase lifecycle and retrieval: create,
// read, delete, and the list/count/lookup queries that return whole purchases.
type PurchaseCoreRepository interface {
	CreatePurchase(ctx context.Context, p *Purchase) error
	GetPurchase(ctx context.Context, id string) (*Purchase, error)
	DeletePurchase(ctx context.Context, id string) error

	// List and count
	ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Purchase, error)
	ListUnsoldPurchases(ctx context.Context, campaignID string) ([]Purchase, error)
	ListAllUnsoldPurchases(ctx context.Context) ([]Purchase, error)
	CountPurchasesByCampaign(ctx context.Context, campaignID string) (int, error)

	// Cert- and ID-based lookups
	GetPurchaseByCertNumber(ctx context.Context, grader string, certNumber string) (*Purchase, error)
	GetPurchasesByGraderAndCertNumbers(ctx context.Context, grader string, certNumbers []string) (map[string]*Purchase, error)
	GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*Purchase, error)
	GetPurchasesByIDs(ctx context.Context, ids []string) (map[string]*Purchase, error)
}

// PurchaseFieldRepository handles targeted column updates on an existing
// purchase: card metadata, images, cost, receipt timing, and campaign
// attribution. Every method here mutates a purchase that already exists.
type PurchaseFieldRepository interface {
	// UpdatePurchaseCLValue refreshes the current CL value and population.
	// confidence is CardLadder's confidence in this specific card, and the
	// nil/zero distinction is load-bearing: nil means CardLadder gave no answer
	// and must leave any stored value NULL, while a non-nil pointer to 0 is a
	// genuine zero-confidence answer and must be stored as 0. Implementations
	// must not collapse the two (no COALESCE(confidence, 0)).
	UpdatePurchaseCLValue(ctx context.Context, id string, clValueCents, population int, confidence *int) error
	UpdatePurchaseCLSyncedAt(ctx context.Context, id string, syncedAt string) error
	UpdatePurchaseCardMetadata(ctx context.Context, id string, cardName, cardNumber, setName string) error
	UpdatePurchaseCardYear(ctx context.Context, id string, year string) error
	UpdatePurchaseImages(ctx context.Context, id string, frontURL, backURL string) error
	UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error
	UpdateExternalPurchaseFields(ctx context.Context, id string, p *Purchase) error
	UpdatePurchaseMarketSnapshot(ctx context.Context, id string, snap MarketSnapshotData) error
	UpdatePurchaseCampaign(ctx context.Context, purchaseID string, campaignID string, sourcingFeeCents int) error
	UpdatePurchasePSAFields(ctx context.Context, id string, fields PSAUpdateFields) error
	UpdatePurchaseBuyCost(ctx context.Context, id string, buyCostCents int) error
	// SetReceivedAt stamps the receipt time that gates the DH push pipeline.
	SetReceivedAt(ctx context.Context, purchaseID string, receivedAt time.Time) error
	// ReattributePurchase moves a purchase to a PSA-authoritative campaign and
	// sets attribution_source='psa'. Returns ErrPurchaseHasSale if a linked sale
	// exists — sold rows carry frozen sale-side financials that this does not repair.
	ReattributePurchase(ctx context.Context, purchaseID string, r Reattribution) error
	// UpdatePurchaseAttributionName records PSA's campaign name and attribution
	// source without moving the campaign. Safe on sold purchases.
	UpdatePurchaseAttributionName(ctx context.Context, purchaseID, psaName, source string) error
}

// PurchasePricingRepository handles the manual price override and AI
// suggestion columns that drive the price review queue.
type PurchasePricingRepository interface {
	UpdatePurchasePriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error
	UpdatePurchaseAISuggestion(ctx context.Context, purchaseID string, priceCents int) error
	ClearPurchaseAISuggestion(ctx context.Context, purchaseID string) error
	AcceptAISuggestion(ctx context.Context, purchaseID string, priceCents int) error
	GetPriceOverrideStats(ctx context.Context) (*PriceOverrideStats, error)
}

// PurchaseEbayExportRepository handles the eBay export flag marking purchases
// queued for the next CSV export.
type PurchaseEbayExportRepository interface {
	SetEbayExportFlag(ctx context.Context, purchaseID string, flaggedAt time.Time) error
	ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error
	ListEbayFlaggedPurchases(ctx context.Context) ([]Purchase, error)
}

// PurchaseSnapshotRepository handles the market-snapshot job state the
// snapshot scheduler uses to find and advance pending work.
type PurchaseSnapshotRepository interface {
	ListSnapshotPurchasesByStatus(ctx context.Context, status SnapshotStatus, limit int) ([]Purchase, error)
	UpdatePurchaseSnapshotStatus(ctx context.Context, id string, status SnapshotStatus, retryCount int) error
}

// PurchaseRepository is the full purchase persistence port, composed from the
// focused interfaces above. Depend on the narrowest one that covers your use —
// the Postgres PurchaseStore satisfies all of them. Take this composite only
// when you genuinely span every concern, as inventory.Service does.
type PurchaseRepository interface {
	PurchaseCoreRepository
	PurchaseFieldRepository
	PurchasePricingRepository
	PurchaseEbayExportRepository
	PurchaseSnapshotRepository
	PurchaseDHRepository
}
