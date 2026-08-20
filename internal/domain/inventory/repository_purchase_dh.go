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

// PurchaseDHRepository handles the DH v2 columns on a purchase — cert
// resolution, the push pipeline's status/attempt/hold state, and the
// inventory linkage DH hands back. It is the largest of the purchase ports
// because the push pipeline needs several atomic multi-column transitions
// that cannot be composed safely from single-column updates.
//
// This is distinct from DHRepository (repository_dh.go), which persists DH
// state that is not attached to a purchase row.
type PurchaseDHRepository interface {
	UpdatePurchaseDHFields(ctx context.Context, id string, update DHFieldsUpdate) error
	GetPurchasesByDHInventoryIDs(ctx context.Context, dhIDs []int) (map[int]*Purchase, error)
	GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]Purchase, error)
	UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error
	// IncrementDHPushAttempts atomically increments the per-purchase skip-attempt
	// counter and returns the new value. Used by the DH push scheduler to cap
	// indefinite retry loops for certs DH can't match.
	IncrementDHPushAttempts(ctx context.Context, id string) (int, error)
	// UpdatePurchaseDHStatus updates only the dh_status column on a purchase.
	// This is a targeted update that does not touch any other DH fields, unlike
	// UpdatePurchaseDHFields which overwrites the full field set.
	UpdatePurchaseDHStatus(ctx context.Context, id string, status string) error
	// ListStaleDHStatusSoldPurchases returns IDs of purchases that have a linked
	// sale but whose dh_status is not 'sold'. Used by the reconciler scheduler.
	ListStaleDHStatusSoldPurchases(ctx context.Context) ([]string, error)
	// UpdatePurchaseDHCardID updates only the dh_card_id column on a purchase.
	// Targeted update that does not touch any other DH fields.
	UpdatePurchaseDHCardID(ctx context.Context, id string, cardID int) error
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
	// ResetDHFieldsForRepushDueToDelete mirrors ResetDHFieldsForRepush and
	// additionally stamps dh_unlisted_detected_at so the UI can badge the row.
	// Used when DH no longer has the item (it deleted it, or we marked it sold
	// and the local sale was later reversed) and the purchase needs to flow
	// back through the push pipeline.
	ResetDHFieldsForRepushDueToDelete(ctx context.Context, purchaseID string) error
	// UpdatePurchaseDHPriceSync updates dh_listing_price_cents and
	// dh_last_synced_at in a single targeted UPDATE. Unlike
	// UpdatePurchaseDHFields, it does not touch any other DH columns.
	// Used by the DH price re-sync path after a successful DH PATCH.
	UpdatePurchaseDHPriceSync(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error
	// UnmatchPurchaseDH atomically clears all DH tracking fields (card ID,
	// inventory ID, cert status, listing price, channels, DH status, last
	// synced timestamp) and sets dh_push_status to pushStatus in a single
	// UPDATE. dh_push_attempts is reset to 0 when pushStatus is "pending" or
	// "matched" so a fresh re-enrollment starts with a clean retry budget.
	// Used by the unmatch handler to avoid partial state between field-clear
	// and status-update.
	UnmatchPurchaseDH(ctx context.Context, purchaseID string, pushStatus string) error
	// ListDHPriceDrift returns unsold purchases where DH is known
	// (dh_inventory_id > 0), the reviewed price is positive, and the
	// reviewed price differs from the price DH currently has
	// (dh_listing_price_cents). Excludes dismissed/held push statuses.
	// Ordered oldest-synced first so stale items sync first.
	ListDHPriceDrift(ctx context.Context) ([]Purchase, error)
	// SetDHSaleConflict flags a purchase for human review after a
	// non-retryable DH sale-recording error, or an apparent success with
	// delisted == false (spec §4). A flagged purchase is excluded from
	// ListSalesNeedingDHRecord until the flag is cleared.
	SetDHSaleConflict(ctx context.Context, purchaseID, reason string) error
	// ClearDHSaleConflict clears a previously flagged conflict, re-enrolling
	// the row in the next §5b recovery pass.
	ClearDHSaleConflict(ctx context.Context, purchaseID string) error
	// ResetDHFieldsForRelistAfterVoid mirrors ResetDHFieldsForRepushDueToDelete
	// but PRESERVES dh_inventory_id (the DH row is still alive after a void)
	// and sets dh_status to in_stock. Used by the un-sell path (spec §7) to
	// route a voided sale back through the push pipeline's auto-relist branch
	// without creating a duplicate DH inventory row.
	ResetDHFieldsForRelistAfterVoid(ctx context.Context, purchaseID string) error
}
