// Package dhlisting provides adapters that bridge the DH API client types
// to domain-level DHListingService interfaces.
package dhlisting

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
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
		dhItems[i] = dh.InventoryItem{
			DHCardID:         item.DHCardID,
			CertNumber:       item.CertNumber,
			GradingCompany:   dh.GraderPSA,
			Grade:            item.Grade,
			CostBasisCents:   item.CostBasisCents,
			MarketValueCents: dh.IntPtr(item.MarketValueCents),
			Status:           dh.InventoryStatusInStock,
		}
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

// --- DHInventoryLister adapter ---

// InventoryListerAdapter wraps a dh.Client to implement dhlisting.DHInventoryLister.
type InventoryListerAdapter struct {
	client interface {
		UpdateInventory(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
		SyncChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
	}
}

// NewInventoryListerAdapter creates a new InventoryListerAdapter.
func NewInventoryListerAdapter(client interface {
	UpdateInventory(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
	SyncChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
}) *InventoryListerAdapter {
	return &InventoryListerAdapter{client: client}
}

func (a *InventoryListerAdapter) UpdateInventoryStatus(ctx context.Context, inventoryID int, status string) error {
	_, err := a.client.UpdateInventory(ctx, inventoryID, dh.InventoryUpdate{Status: status})
	return err
}

func (a *InventoryListerAdapter) SyncChannels(ctx context.Context, inventoryID int, channels []string) error {
	_, err := a.client.SyncChannels(ctx, inventoryID, channels)
	return err
}

// MarkInventorySold transitions the DH inventory item to "sold" status,
// retiring it from the DH platform when a sale is recorded locally.
func (a *InventoryListerAdapter) MarkInventorySold(ctx context.Context, inventoryID int) error {
	_, err := a.client.UpdateInventory(ctx, inventoryID, dh.InventoryUpdate{Status: inventory.DHStatusSold})
	return err
}

var _ dhlisting.DHInventoryLister = (*InventoryListerAdapter)(nil)
var _ inventory.DHSoldNotifier = (*InventoryListerAdapter)(nil)
