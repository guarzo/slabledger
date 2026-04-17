// Package dhpricing re-syncs DH listing prices when a user's reviewed price
// changes. Keeps DH's listing_price_cents aligned with ReviewedPriceCents
// for purchases already pushed to or listed on DH. Sibling of dhlisting;
// does not import it — the tiny price-resolution helper is inlined here.
package dhpricing

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Outcome classifies the result of one sync attempt.
type Outcome string

const (
	OutcomeSynced              Outcome = "synced"
	OutcomeSkippedNoInventory  Outcome = "skipped_no_inventory"
	OutcomeSkippedZeroReviewed Outcome = "skipped_zero_reviewed"
	OutcomeSkippedNoDrift      Outcome = "skipped_no_drift"
	OutcomeStaleInventoryID    Outcome = "stale_inventory_id"
	OutcomeError               Outcome = "error"
)

// SyncResult reports the result of SyncPurchasePrice for one purchase.
type SyncResult struct {
	PurchaseID      string
	Outcome         Outcome
	OldListingCents int
	NewListingCents int
	Err             error
}

// SyncBatchResult aggregates a batch run of SyncDriftedPurchases.
type SyncBatchResult struct {
	Total     int
	ByOutcome map[Outcome]int
}

// PurchaseLookup reads purchases for the price-sync flow.
type PurchaseLookup interface {
	GetPurchase(ctx context.Context, purchaseID string) (*inventory.Purchase, error)
	ListDHPriceDrift(ctx context.Context) ([]inventory.Purchase, error)
}

// DHPriceUpdater patches a DH inventory item's status + listing price.
// Returns the listing_price_cents DH has on the item after the update.
// Satisfied by *clients/dhlisting.InventoryAdapter.
type DHPriceUpdater interface {
	UpdateInventoryStatus(ctx context.Context, inventoryID int, status string, listingPriceCents int) (int, error)
}

// DHPriceWriter persists DH price + sync timestamp locally after a
// successful PATCH. Satisfied by *sqlite.PurchaseStore.
type DHPriceWriter interface {
	UpdatePurchaseDHPriceSync(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error
}

// DHReconcileResetter resets stale DH linkage when DH reports an inventory
// ID it no longer recognizes (ERR_PROV_NOT_FOUND). Satisfied by
// *sqlite.PurchaseStore. Declared here (rather than imported from
// dhlisting) to preserve the flat-siblings invariant.
type DHReconcileResetter interface {
	ResetDHFieldsForRepush(ctx context.Context, purchaseID string) error
}

// Service is the domain entry point for DH price re-sync.
type Service interface {
	SyncPurchasePrice(ctx context.Context, purchaseID string) SyncResult
	SyncDriftedPurchases(ctx context.Context) SyncBatchResult
}
