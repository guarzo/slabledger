// Package dhlisting provides adapters that bridge the DH API client types
// to domain-level DHListingService interfaces.
package dhlisting

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- DHCertResolver adapter ---

// CertResolverAdapter wraps a dh.Client to implement dhlisting.DHCertResolver.
type CertResolverAdapter struct {
	client interface {
		ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
	}
}

// NewCertResolverAdapter creates a new CertResolverAdapter.
func NewCertResolverAdapter(client interface {
	ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
}) *CertResolverAdapter {
	return &CertResolverAdapter{client: client}
}

func (a *CertResolverAdapter) ResolveCert(ctx context.Context, req dhlisting.DHCertResolveRequest) (*dhlisting.DHCertResolution, error) {
	resp, err := a.client.ResolveCert(ctx, dh.CertResolveRequest{
		CertNumber: req.CertNumber,
		CardName:   req.CardName,
		SetName:    req.SetName,
		CardNumber: req.CardNumber,
		Year:       req.Year,
		Variant:    req.Variant,
	})
	if err != nil {
		return nil, err
	}
	result := &dhlisting.DHCertResolution{
		Status:   resp.Status,
		DHCardID: resp.DHCardID,
	}
	for _, c := range resp.Candidates {
		result.Candidates = append(result.Candidates, dhlisting.DHCertCandidate{
			DHCardID:   c.DHCardID,
			CardName:   c.CardName,
			SetName:    c.SetName,
			CardNumber: c.CardNumber,
		})
	}
	return result, nil
}

var _ dhlisting.DHCertResolver = (*CertResolverAdapter)(nil)

// --- DHInventoryPusher adapter ---

// InventoryPusherAdapter wraps a dh.Client to implement dhlisting.DHInventoryPusher.
type InventoryPusherAdapter struct {
	client interface {
		PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
	}
}

// NewInventoryPusherAdapter creates a new InventoryPusherAdapter.
func NewInventoryPusherAdapter(client interface {
	PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}) *InventoryPusherAdapter {
	return &InventoryPusherAdapter{client: client}
}

func (a *InventoryPusherAdapter) PushInventory(ctx context.Context, items []dhlisting.DHInventoryPushItem) (*dhlisting.DHInventoryPushResult, error) {
	dhItems := make([]dh.InventoryItem, len(items))
	for i, item := range items {
		dhItem := dh.NewInStockItem(item.DHCardID, item.CertNumber, item.Grade, item.CostBasisCents, item.ListingPriceCents)
		if item.CertImageURLFront != "" {
			dhItem.CertImageURLFront = item.CertImageURLFront
		}
		if item.CertImageURLBack != "" {
			dhItem.CertImageURLBack = item.CertImageURLBack
		}
		dhItems[i] = dhItem
	}

	resp, err := a.client.PushInventory(ctx, dhItems)
	if err != nil {
		return nil, err
	}

	result := &dhlisting.DHInventoryPushResult{}
	for _, r := range resp.Results {
		result.Results = append(result.Results, dhlisting.DHInventoryPushResultItem{
			DHInventoryID:      r.DHInventoryID,
			Status:             r.Status,
			AssignedPriceCents: r.AssignedPriceCents,
			ChannelsJSON:       dh.MarshalChannels(r.Channels),
		})
	}
	return result, nil
}

var _ dhlisting.DHInventoryPusher = (*InventoryPusherAdapter)(nil)

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
	if update.Status == "listed" && a.rotator != nil {
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
