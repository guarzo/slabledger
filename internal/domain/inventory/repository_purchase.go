package inventory

import (
	"context"
	"time"
)

// DHFieldsUpdate contains the DH v2 tracking fields to update on a purchase.
type DHFieldsUpdate struct {
	CardID            int
	InventoryID       int
	CertStatus        string
	ListingPriceCents int
	ChannelsJSON      string
	DHStatus          DHStatus
	LastSyncedAt      string // RFC3339; set to time.Now() on each inventory poll
}

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

// PurchaseRepository handles purchase persistence.
type PurchaseRepository interface {
	// CRUD
	CreatePurchase(ctx context.Context, p *Purchase) error
	GetPurchase(ctx context.Context, id string) (*Purchase, error)
	DeletePurchase(ctx context.Context, id string) error

	// List and count
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
	UpdatePurchaseCLSyncedAt(ctx context.Context, id string, syncedAt string) error
	UpdatePurchaseMMValue(ctx context.Context, id string, mmValueCents int) error
	UpdatePurchaseCardMetadata(ctx context.Context, id string, cardName, cardNumber, setName string) error
	UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error
	UpdateExternalPurchaseFields(ctx context.Context, id string, p *Purchase) error
	UpdatePurchaseMarketSnapshot(ctx context.Context, id string, snap MarketSnapshotData) error
	UpdatePurchaseCampaign(ctx context.Context, purchaseID string, campaignID string, sourcingFeeCents int) error
	UpdatePurchasePSAFields(ctx context.Context, id string, fields PSAUpdateFields) error
	UpdatePurchaseBuyCost(ctx context.Context, id string, buyCostCents int) error

	// Price overrides & AI suggestions
	UpdatePurchasePriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error
	UpdatePurchaseAISuggestion(ctx context.Context, purchaseID string, priceCents int) error
	ClearPurchaseAISuggestion(ctx context.Context, purchaseID string) error
	AcceptAISuggestion(ctx context.Context, purchaseID string, priceCents int) error
	GetPriceOverrideStats(ctx context.Context) (*PriceOverrideStats, error)

	// Receipt tracking
	SetReceivedAt(ctx context.Context, purchaseID string, receivedAt time.Time) error

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
	CountUnsoldByDHPushStatus(ctx context.Context) (map[string]int, error)
	// CountDHPipelineHealth returns finer-grained counts for the DH push
	// pipeline dashboard. PendingReceived matches what /api/dh/pending actually
	// drains (dh_push_status='pending' AND received_at IS NOT NULL).
	// UnenrolledReceived counts received, unsold rows with no push-pipeline
	// state — the "black hole" bucket that was previously invisible.
	CountDHPipelineHealth(ctx context.Context) (DHPipelineHealth, error)
	UpdatePurchaseDHCandidates(ctx context.Context, id string, candidatesJSON string) error
	UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error
	// SetHeldWithReason atomically sets the push status to held and records
	// the hold reason in a single transaction, preventing any reader from
	// observing a held purchase with an empty reason.
	SetHeldWithReason(ctx context.Context, purchaseID string, reason string) error
	// ApproveHeldPurchase atomically clears the hold reason and sets the push
	// status to pending in a single transaction, preventing the scheduler from
	// observing a half-updated record.
	ApproveHeldPurchase(ctx context.Context, purchaseID string) error
	// ResetDHFieldsForRepush atomically clears the DH inventory linkage
	// (inventory ID, listing price, channels, status) and sets push status to
	// pending so the scheduler re-enrolls the purchase. Preserves dh_card_id,
	// dh_cert_status, and dh_candidates (cert resolution remains valid).
	// Used by reconciliation when DH inventory has drifted from local state.
	ResetDHFieldsForRepush(ctx context.Context, purchaseID string) error
}
