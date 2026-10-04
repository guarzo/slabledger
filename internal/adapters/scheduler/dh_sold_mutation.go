package scheduler

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func WithDHSoldMutationCoordinator(c *inventory.DHMutationCoordinator, g inventory.DHMutationGuards, p inventory.DHSalePurchaseReader, sa inventory.DHSaleMutationStore) DHSoldReconcilerOption {
	return func(s *DHSoldReconcilerScheduler) {
		s.mutationRequired = true
		s.coordinator = c
		s.guards = g
		s.purchaseReader = p
		s.saleStore = sa
	}
}
func (s *DHSoldReconcilerScheduler) updateCurrentSold(ctx context.Context, id string) error {
	if !s.mutationRequired {
		return s.updater.UpdatePurchaseDHStatus(ctx, id, "sold")
	}
	if !inventory.DHDependenciesPresent(s.guards, s.purchaseReader, s.saleStore) {
		return inventory.NewReturnConflict("coordination_unavailable", "sold observation dependencies required")
	}
	at, e := s.guards.ObservationTime(ctx)
	if e != nil {
		return e
	}
	p, e := s.purchaseReader.GetPurchase(ctx, id)
	if e != nil {
		return e
	}
	sa, e := s.saleStore.GetSaleByPurchaseID(ctx, id)
	if e != nil {
		return e
	}
	if sa == nil {
		return nil
	}
	_, e = s.guards.ApplyDHSoldObservation(ctx, id, sa.ID, p.DHInventoryID, at, func(c context.Context) error { return s.updater.UpdatePurchaseDHStatus(c, id, "sold") })
	return e
}
