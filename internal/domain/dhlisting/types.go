package dhlisting

import "context"

// DHListingService orchestrates the multi-step DH listing workflow:
// cert resolution → card ID saving → market value resolution → inventory push
// → field persistence → status update → channel sync (with rollback).
//
// This is a separate interface from Service to keep the main Service from
// growing further (ISP). Implementations coordinate multiple external
// dependencies (cert resolver, pusher, lister, persistence) that are
// injected via functional options.
type Service interface {
	// ListPurchases resolves the given cert numbers to purchases, performs
	// inline match-and-push for any that are pending, then transitions them
	// to listed status with channel sync. It runs synchronously; the caller
	// is responsible for launching it in a background goroutine if desired.
	//
	// Returns a summary of the listing operation.
	ListPurchases(ctx context.Context, certNumbers []string) DHListingResult
}

// DHListingResult summarises a batch listing operation.
type DHListingResult struct {
	Listed  int   // items successfully set to listed + synced
	Synced  int   // items with channels synced
	Skipped int   // items skipped (unenrolled, inline push failed, update/sync error, etc)
	Total   int   // total purchases found
	Error   error // set when a fatal error prevented listing (e.g. DB lookup failure)
}

// --- Domain-level abstractions for DH external operations ---

// DHCertResolveRequest contains the fields needed to resolve a cert against DH.
type DHCertResolveRequest struct {
	CertNumber string
	CardName   string
	SetName    string
	CardNumber string
	Year       string
	Variant    string
}

// DHCertCandidate is one possible match for an ambiguous cert resolution.
type DHCertCandidate struct {
	DHCardID   int
	CardName   string
	SetName    string
	CardNumber string
}

// DHCertResolution is the result of resolving a single cert.
type DHCertResolution struct {
	Status     string // "matched", "ambiguous", "not_found"
	DHCardID   int
	Candidates []DHCertCandidate
}

// Domain-level cert resolution status constants.
const (
	DHCertStatusMatched   = "matched"
	DHCertStatusAmbiguous = "ambiguous"
	DHCertStatusNotFound  = "not_found"
)

// DefaultListingChannels are the DH sales channels enabled by default
// when listing inventory items.
var DefaultListingChannels = []string{"ebay", "shopify"}

// SourceDH is the provider key for DoubleHolo price lookups.
// Duplicated from pricing.SourceDH to avoid cross-domain import.
const SourceDH = "doubleholo"

// DHCertResolver resolves PSA cert numbers to DH card IDs.
// This is the domain-level interface; adapter implementations wrap the
// external DH API client.
type DHCertResolver interface {
	ResolveCert(ctx context.Context, req DHCertResolveRequest) (*DHCertResolution, error)
}

// DHInventoryPushItem is a single item to push to DH inventory.
type DHInventoryPushItem struct {
	DHCardID          int
	CertNumber        string
	Grade             float64
	CostBasisCents    int
	ListingPriceCents int // 0 means omit — DH uses catalog fallback
}

// DHInventoryPushResultItem is the per-item response from an inventory push.
type DHInventoryPushResultItem struct {
	DHInventoryID      int
	Status             string // "in_stock", "listed", "failed"
	AssignedPriceCents int
	ChannelsJSON       string // serialized channel statuses
}

// DHInventoryPushResult is the response from pushing inventory to DH.
type DHInventoryPushResult struct {
	Results []DHInventoryPushResultItem
}

// DHInventoryPusher pushes inventory items to DH.
type DHInventoryPusher interface {
	PushInventory(ctx context.Context, items []DHInventoryPushItem) (*DHInventoryPushResult, error)
}

// DHInventoryLister transitions DH inventory items to listed and syncs channels.
//
// UpdateInventoryStatus may carry an optional listingPriceCents to set/update
// the listing price in the same PATCH call. Pass 0 to omit. On success,
// returns the listing_price_cents that DH has on the item after the update.
type DHInventoryLister interface {
	UpdateInventoryStatus(ctx context.Context, inventoryID int, status string, listingPriceCents int) (int, error)
	SyncChannels(ctx context.Context, inventoryID int, channels []string) error
}

// DHCardIDSaver persists DH card ID mappings for future lookups.
type DHCardIDSaver interface {
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
}
