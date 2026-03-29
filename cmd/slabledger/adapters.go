package main

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/psa"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// favoritesListAdapter adapts sqlite.FavoritesRepository to the scheduler.FavoritesLister interface.
type favoritesListAdapter struct {
	repo *sqlite.FavoritesRepository
}

func (a *favoritesListAdapter) ListAllDistinctCards(ctx context.Context) ([]scheduler.FavoriteCard, error) {
	cards, err := a.repo.ListAllDistinctCards(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]scheduler.FavoriteCard, len(cards))
	for i, c := range cards {
		result[i] = scheduler.FavoriteCard{
			CardName:   c.CardName,
			SetName:    c.SetName,
			CardNumber: c.CardNumber,
		}
	}
	return result, nil
}

// inventoryListAdapter adapts sqlite.CampaignsRepository to the scheduler.InventoryLister interface.
type inventoryListAdapter struct {
	repo *sqlite.CampaignsRepository
}

func (a *inventoryListAdapter) ListUnsoldInventory(ctx context.Context) ([]scheduler.InventoryPurchase, error) {
	purchases, err := a.repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]scheduler.InventoryPurchase, len(purchases))
	for i, p := range purchases {
		result[i] = scheduler.InventoryPurchase{
			ID:              p.ID,
			CardName:        p.CardName,
			CardNumber:      p.CardNumber,
			SetName:         p.SetName,
			GradeValue:      p.GradeValue,
			Grader:          p.Grader,
			BuyCostCents:    p.BuyCostCents,
			CLValueCents:    p.CLValueCents,
			PSAListingTitle: p.PSAListingTitle,
			SnapshotDate:    p.SnapshotDate,
		}
	}
	return result, nil
}

// snapshotRefreshAdapter adapts campaigns.Service to the scheduler.SnapshotRefresher interface.
type snapshotRefreshAdapter struct {
	svc campaigns.Service
}

func (a *snapshotRefreshAdapter) RefreshSnapshot(ctx context.Context, p scheduler.InventoryPurchase) bool {
	return a.svc.RefreshPurchaseSnapshot(ctx, p.ID, campaigns.CardIdentity{
		CardName: p.CardName, CardNumber: p.CardNumber, SetName: p.SetName, PSAListingTitle: p.PSAListingTitle,
	}, p.GradeValue, p.CLValueCents)
}

// campaignCardListAdapter adapts sqlite.CampaignsRepository to the scheduler.CampaignCardLister interface.
type campaignCardListAdapter struct {
	repo *sqlite.CampaignsRepository
}

func (a *campaignCardListAdapter) ListUnsoldCards(ctx context.Context) ([]scheduler.UnsoldCard, error) {
	infos, err := a.repo.ListUnsoldCards(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]scheduler.UnsoldCard, len(infos))
	for i, info := range infos {
		result[i] = scheduler.UnsoldCard{CardName: info.CardName, SetName: info.SetName, CardNumber: info.CardNumber}
	}
	return result, nil
}

// cardIDMappingListAdapter adapts sqlite.CardIDMappingRepository to the scheduler.CardIDMappingLister interface.
type cardIDMappingListAdapter struct {
	repo *sqlite.CardIDMappingRepository
}

func (a *cardIDMappingListAdapter) ListByProvider(ctx context.Context, provider string) ([]scheduler.CardIDMapping, error) {
	mappings, err := a.repo.ListByProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	result := make([]scheduler.CardIDMapping, len(mappings))
	for i, m := range mappings {
		result[i] = scheduler.CardIDMapping{
			CardName:        m.CardName,
			SetName:         m.SetName,
			CollectorNumber: m.CollectorNumber,
			ExternalID:      m.ExternalID,
		}
	}
	return result, nil
}

// --- PSA image backfill adapters ---

type psaImageListerAdapter struct {
	repo *sqlite.CampaignsRepository
}

func (a *psaImageListerAdapter) ListPurchasesMissingImages(ctx context.Context, limit int) ([]psa.PurchaseImageRow, error) {
	rows, err := a.repo.ListPurchasesMissingImages(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]psa.PurchaseImageRow, len(rows))
	for i, r := range rows {
		result[i] = psa.PurchaseImageRow{ID: r.ID, CertNumber: r.CertNumber}
	}
	return result, nil
}

type psaImageUpdaterAdapter struct {
	repo *sqlite.CampaignsRepository
}

func (a *psaImageUpdaterAdapter) UpdatePurchaseImageURLs(ctx context.Context, id, frontURL, backURL string) error {
	return a.repo.UpdatePurchaseImageURLs(ctx, id, frontURL, backURL)
}

// newCardDiscovererAdapter returns the scheduler.CardDiscoverer directly as a
// handlers.CardDiscoverer. Both interfaces now use campaigns.CardIdentity, so no
// conversion is needed — the scheduler type satisfies the handler interface.
func newCardDiscovererAdapter(d scheduler.CardDiscoverer) handlers.CardDiscoverer {
	if d == nil {
		return nil
	}
	return d
}
