package dhlisting

import (
	"context"
	"errors"
)

// ErrPSAKeysExhausted signals that a listing attempt failed because all
// configured PSA API keys have been rotated through without success.
//
// Contract: DHInventoryLister implementations MUST return or wrap this
// domain-level sentinel (not merely the underlying dh.ErrPSAKeysExhausted)
// so callers can detect exhaustion via errors.Is(err, ErrPSAKeysExhausted)
// without importing the dh adapter package. The underlying adapter-side
// sentinel, when relevant, should be wrapped alongside as the cause.
var ErrPSAKeysExhausted = errors.New("dh: PSA keys exhausted")

// PSAKeyRotator is implemented by lister adapters that support PSA API key
// rotation. Defined in the domain so services can type-assert without
// importing the dh adapter package.
type PSAKeyRotator interface {
	RotatePSAKey() bool
	ResetPSAKeyRotation()
}

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

// DHCertStatusMatched is the DH-side cert_status value for a cert whose card
// has been identified. Persisted on campaign_purchases after a successful
// psa_import resolution so the UI can distinguish resolved vs pending rows.
const DHCertStatusMatched = "matched"

// DefaultListingChannels are the DH sales channels enabled by default
// when listing inventory items.
var DefaultListingChannels = []string{"ebay", "shopify"}

// SourceDH is the provider key for DoubleHolo price lookups.
// Duplicated from pricing.SourceDH to avoid cross-domain import.
const SourceDH = "doubleholo"

// DHPSAImportItem carries overrides sent with a psa_import call.
// DH looks up the cert via PSA, then matches it against its catalog using
// these overrides as hints (and falls back to creating a partner_submitted
// card when the catalog has no match).
type DHPSAImportItem struct {
	CertNumber     string
	CostBasisCents int
	CardName       string
	SetName        string
	CardNumber     string
	Year           string
	Language       string // optional; inferred from set_name when empty
}

// DHPSAImportResult is the per-cert result from a psa_import call.
// Resolution is one of PSAImportStatus* constants below. For success paths
// (matched / unmatched_created / override_corrected / already_listed) both
// DHCardID and DHInventoryID are populated.
type DHPSAImportResult struct {
	CertNumber    string
	Resolution    string
	DHCardID      int
	DHInventoryID int
	DHStatus      string // "in_stock" | "listed" — DH's inventory status after the call
	Error         string
	RateLimited   bool
}

// PSAImport resolution constants mirror the adapter-side values; domain code
// switches on these without importing the dh client package.
const (
	PSAImportStatusMatched           = "matched"
	PSAImportStatusUnmatchedCreated  = "unmatched_created"
	PSAImportStatusOverrideCorrected = "override_corrected"
	PSAImportStatusAlreadyListed     = "already_listed"
	PSAImportStatusPSAError          = "psa_error"
	PSAImportStatusPartnerCardError  = "partner_card_error"
)

// IsPSAImportSuccess reports whether a psa_import resolution produced a
// usable dh_card_id + dh_inventory_id pair.
func IsPSAImportSuccess(resolution string) bool {
	switch resolution {
	case PSAImportStatusMatched,
		PSAImportStatusUnmatchedCreated,
		PSAImportStatusOverrideCorrected,
		PSAImportStatusAlreadyListed:
		return true
	}
	return false
}

// DHPSAImporter submits PSA-graded certs to DH's psa_import endpoint,
// which does PSA lookup + catalog match + inventory creation in a single call.
// This is the primary cert intake path for both the push scheduler and the
// inline "list on import" flow in the dhlisting service.
type DHPSAImporter interface {
	PSAImport(ctx context.Context, items []DHPSAImportItem) ([]DHPSAImportResult, error)
}

// DHInventoryStatusUpdate carries the fields that UpdateInventoryStatus can
// mutate on a DH inventory item. Image URLs are optional; when either is set
// on a transition to "listed", DH uses them instead of doing its own PSA
// lookup, which keeps the listing path functional when PSA is rate-limited
// or authentication is failing.
type DHInventoryStatusUpdate struct {
	Status            string
	ListingPriceCents int // 0 means omit
	CertImageURLFront string
	CertImageURLBack  string
}

// DHInventoryLister transitions DH inventory items to listed and syncs channels.
//
// UpdateInventoryStatus returns the listing_price_cents that DH has on the
// item after the update. When update.Status == "listed" and the implementation
// supports PSA key rotation, a 401 response from DH causes automatic rotation
// through configured PSA keys; on exhaustion, the error wraps
// dh.ErrPSAKeysExhausted (detectable via errors.Is).
type DHInventoryLister interface {
	UpdateInventoryStatus(ctx context.Context, inventoryID int, update DHInventoryStatusUpdate) (int, error)
	SyncChannels(ctx context.Context, inventoryID int, channels []string) error
}

// DHCardIDSaver persists DH card ID mappings for future lookups.
type DHCardIDSaver interface {
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
}
