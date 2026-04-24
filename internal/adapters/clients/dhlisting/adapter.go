// Package dhlisting provides adapters that bridge the DH API client types
// to domain-level DHListingService interfaces.
package dhlisting

import (
	"context"
	"errors"
	"fmt"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- DHPSAImporter adapter ---

// PSAImporterAdapter wraps a dh.Client (or any object with the same method
// signature) to implement dhlisting.DHPSAImporter. It translates between the
// domain's DHPSAImportItem/Result types and the adapter-side dh.PSAImport*
// types. It also exposes the client's PSAKeyRotator so the listing service
// can rotate on rate-limit without importing the dh package.
type PSAImporterAdapter struct {
	client interface {
		PSAImport(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error)
	}
	rotator dh.PSAKeyRotator
}

// NewPSAImporterAdapter creates a new PSAImporterAdapter. If the client
// implements dh.PSAKeyRotator, rotation is wired through.
func NewPSAImporterAdapter(client interface {
	PSAImport(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error)
}) *PSAImporterAdapter {
	a := &PSAImporterAdapter{client: client}
	if r, ok := client.(dh.PSAKeyRotator); ok {
		a.rotator = r
	}
	return a
}

func (a *PSAImporterAdapter) PSAImport(ctx context.Context, items []dhlisting.DHPSAImportItem) ([]dhlisting.DHPSAImportResult, error) {
	dhItems := make([]dh.PSAImportItem, len(items))
	for i, item := range items {
		overrides := &dh.PSAImportOverrides{
			Name:       item.CardName,
			SetName:    item.SetName,
			CardNumber: item.CardNumber,
			Year:       item.Year,
			Language:   item.Language,
		}
		dhItems[i] = dh.PSAImportItem{
			CertNumber:     item.CertNumber,
			CostBasisCents: item.CostBasisCents,
			Status:         dh.InventoryStatusInStock,
			Overrides:      overrides,
		}
	}

	resp, err := a.client.PSAImport(ctx, dhItems)
	if err != nil {
		return nil, err
	}

	// Batch-level rejection — DH returns {success:false, error:"..."} on 422
	// for invalid batches (>50 items, missing vendor profile, blank
	// cert_number). Surface as an error so the domain caller logs the real
	// reason instead of "no results".
	if !resp.Success {
		if resp.Error != "" {
			return nil, fmt.Errorf("psa_import batch rejected: %s", resp.Error)
		}
		return nil, fmt.Errorf("psa_import batch rejected (no reason given)")
	}

	results := make([]dhlisting.DHPSAImportResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		results = append(results, dhlisting.DHPSAImportResult{
			CertNumber:    r.CertNumber,
			Resolution:    r.Resolution,
			DHCardID:      r.DHCardID,
			DHInventoryID: r.DHInventoryID,
			DHStatus:      r.Status,
			Error:         r.Error,
			RateLimited:   r.RateLimited,
		})
	}
	return results, nil
}

// RotatePSAKey delegates to the underlying client when available.
func (a *PSAImporterAdapter) RotatePSAKey() bool {
	if a.rotator == nil {
		return false
	}
	return a.rotator.RotatePSAKey()
}

// ResetPSAKeyRotation delegates to the underlying client when available.
func (a *PSAImporterAdapter) ResetPSAKeyRotation() {
	if a.rotator == nil {
		return
	}
	a.rotator.ResetPSAKeyRotation()
}

var _ dhlisting.DHPSAImporter = (*PSAImporterAdapter)(nil)
var _ dhlisting.PSAKeyRotator = (*PSAImporterAdapter)(nil)

// --- DHInventoryLister / DHSoldNotifier adapter ---

// InventoryAdapter wraps a dh.Client to implement dhlisting.DHInventoryLister
// and inventory.DHSoldNotifier. It handles both read/list operations and
// inventory status mutations (listing updates and sold transitions).
//
// When transitioning to "listed" and the underlying client supports
// dh.PSAKeyRotator, UpdateInventoryStatus will rotate PSA keys on 401/422
// via dh.UpdateInventoryWithRotation.
type InventoryAdapter struct {
	client interface {
		UpdateInventory(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
		SyncChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
	}
	rotator dh.PSAKeyRotator     // optional; inferred from client if it implements the interface
	logger  observability.Logger // optional; defaults to noop
}

// NewInventoryAdapter creates a new InventoryAdapter. If the client implements
// dh.PSAKeyRotator, rotation is wired up automatically.
func NewInventoryAdapter(client interface {
	UpdateInventory(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
	SyncChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
}) *InventoryAdapter {
	a := &InventoryAdapter{client: client, logger: observability.NewNoopLogger()}
	if r, ok := client.(dh.PSAKeyRotator); ok {
		a.rotator = r
	}
	return a
}

// WithLogger sets the logger used for rotation diagnostics.
func (a *InventoryAdapter) WithLogger(l observability.Logger) *InventoryAdapter {
	if l != nil {
		a.logger = l
	}
	return a
}

// RotatePSAKey delegates to the underlying client's key rotator when available.
// Returns false when no rotator is configured so callers treat it as exhausted.
// Required so *InventoryAdapter satisfies dh.PSAKeyRotator (and the mirror
// domain interface dhlisting.PSAKeyRotator), which dhListingService uses to
// reset rotation state at the top of each ListPurchases call.
func (a *InventoryAdapter) RotatePSAKey() bool {
	if a.rotator == nil {
		return false
	}
	return a.rotator.RotatePSAKey()
}

// ResetPSAKeyRotation delegates to the underlying client when available; no-op
// otherwise. See RotatePSAKey for why InventoryAdapter satisfies the rotator
// interface directly rather than relying on the embedded field.
func (a *InventoryAdapter) ResetPSAKeyRotation() {
	if a.rotator == nil {
		return
	}
	a.rotator.ResetPSAKeyRotation()
}

// UpdateInventoryStatus PATCHes /inventory/:id with the new status, listing
// price, and (when set) cert image URLs. When update.Status == "listed" and
// a rotator is configured, PSA auth/rate-limit errors trigger key rotation.
// On exhaustion, the returned error wraps dh.ErrPSAKeysExhausted.
func (a *InventoryAdapter) UpdateInventoryStatus(ctx context.Context, inventoryID int, update dhlisting.DHInventoryStatusUpdate) (int, error) {
	dhUpdate := dh.InventoryUpdate{Status: update.Status}
	if update.ListingPriceCents > 0 {
		dhUpdate.ListingPriceCents = dh.IntPtr(update.ListingPriceCents)
	}
	if update.CertImageURLFront != "" {
		dhUpdate.CertImageURLFront = update.CertImageURLFront
	}
	if update.CertImageURLBack != "" {
		dhUpdate.CertImageURLBack = update.CertImageURLBack
	}

	var (
		resp *dh.InventoryResult
		err  error
	)
	if update.Status == inventory.DHStatusListed && a.rotator != nil {
		resp, err = dh.UpdateInventoryWithRotation(
			ctx, inventoryID, dhUpdate,
			a.client.UpdateInventory,
			a.rotator.RotatePSAKey,
			a.logger,
			"dh listing",
		)
	} else {
		resp, err = a.client.UpdateInventory(ctx, inventoryID, dhUpdate)
	}
	if err != nil {
		if errors.Is(err, dh.ErrPSAKeysExhausted) {
			return 0, fmt.Errorf("%w: %w", dhlisting.ErrPSAKeysExhausted, err)
		}
		return 0, err
	}
	if resp == nil {
		return 0, nil
	}
	return resp.ListingPriceCents, nil
}

func (a *InventoryAdapter) SyncChannels(ctx context.Context, inventoryID int, channels []string) error {
	_, err := a.client.SyncChannels(ctx, inventoryID, channels)
	return err
}

// MarkInventorySold transitions the DH inventory item to "sold" status,
// retiring it from the DH platform when a sale is recorded locally.
func (a *InventoryAdapter) MarkInventorySold(ctx context.Context, inventoryID int) error {
	_, err := a.client.UpdateInventory(ctx, inventoryID, dh.InventoryUpdate{Status: inventory.DHStatusSold})
	return err
}

var _ dhlisting.DHInventoryLister = (*InventoryAdapter)(nil)
var _ inventory.DHSoldNotifier = (*InventoryAdapter)(nil)
var _ dh.PSAKeyRotator = (*InventoryAdapter)(nil)
var _ dhlisting.PSAKeyRotator = (*InventoryAdapter)(nil)

// --- DHInventorySnapshotFetcher adapter ---

// snapshotPageSize matches the inventory poll scheduler; DH accepts up to 100/page.
const snapshotPageSize = 100

// maxSnapshotPages prevents unbounded pagination if DH misreports totals.
const maxSnapshotPages = 500

// InventorySnapshotAdapter wraps a dh.Client to implement
// dhlisting.DHInventorySnapshotFetcher by paginating ListInventory with no
// updated_since filter to collect the full authoritative set of inventory IDs.
type InventorySnapshotAdapter struct {
	client interface {
		ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
	}
}

// NewInventorySnapshotAdapter creates a new InventorySnapshotAdapter.
func NewInventorySnapshotAdapter(client interface {
	ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
}) *InventorySnapshotAdapter {
	return &InventorySnapshotAdapter{client: client}
}

// FetchAllInventoryIDs paginates GET /inventory with no filters and returns
// the set of DH inventory IDs. Any page error fails the whole call so the
// reconciler never acts on a partial snapshot. Pagination stops on the first
// short or empty page rather than trusting Meta.TotalCount — an underreported
// total would cause us to treat a partial snapshot as complete and
// incorrectly reset healthy items.
func (a *InventorySnapshotAdapter) FetchAllInventoryIDs(ctx context.Context) (map[int]struct{}, error) {
	ids := make(map[int]struct{})
	for page := 1; page <= maxSnapshotPages; page++ {
		resp, err := a.client.ListInventory(ctx, dh.InventoryFilters{
			Page:    page,
			PerPage: snapshotPageSize,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range resp.Items {
			if item.DHInventoryID != 0 {
				ids[item.DHInventoryID] = struct{}{}
			}
		}
		if len(resp.Items) < snapshotPageSize {
			return ids, nil
		}
	}
	return nil, fmt.Errorf("DH snapshot: exceeded max pages (%d), possible API miscount", maxSnapshotPages)
}

var _ dhlisting.DHInventorySnapshotFetcher = (*InventorySnapshotAdapter)(nil)
