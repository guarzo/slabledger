package main

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// inventoryListAdapter adapts sqlite.PurchaseStore to the scheduler.InventoryLister interface.
type inventoryListAdapter struct {
	repo *sqlite.PurchaseStore
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

// snapshotRefreshAdapter adapts inventory.Service to the scheduler.SnapshotRefresher interface.
type snapshotRefreshAdapter struct {
	svc inventory.Service
}

func (a *snapshotRefreshAdapter) RefreshSnapshot(ctx context.Context, p scheduler.InventoryPurchase) bool {
	return a.svc.RefreshPurchaseSnapshot(ctx, p.ID, inventory.CardIdentity{
		CardName: p.CardName, CardNumber: p.CardNumber, SetName: p.SetName, PSAListingTitle: p.PSAListingTitle,
	}, p.GradeValue, p.CLValueCents)
}
