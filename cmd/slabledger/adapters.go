package main

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/psa"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

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
