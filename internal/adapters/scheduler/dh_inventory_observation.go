package scheduler

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"time"
)

type DHInventoryPollOption func(*DHInventoryPollScheduler)

func WithDHInventoryObservationGuards(g inventory.DHMutationGuards) DHInventoryPollOption {
	return func(s *DHInventoryPollScheduler) { s.observationRequired = true; s.guards = g }
}

type returnStateReader interface {
	GetReturnState(context.Context, string) (*inventory.ConfirmedReturnState, error)
}

func (s *DHInventoryPollScheduler) applyInventoryObservation(ctx context.Context, id string, target int, at time.Time, u inventory.DHFieldsUpdate) (bool, error) {
	if !s.observationRequired {
		return true, s.updater.UpdatePurchaseDHFields(ctx, id, u)
	}
	reader, ok := s.guards.(returnStateReader)
	if !ok {
		return false, inventory.NewReturnConflict("coordination_unavailable", "inventory observation state reader required")
	}
	state, e := reader.GetReturnState(ctx, id)
	if e != nil {
		return false, e
	}
	if state.Sale != nil || (state.Purchase.DHInventoryID != 0 && state.Purchase.DHInventoryID != target) {
		return false, nil
	}
	return s.guards.ApplyDHObservation(ctx, id, state.Purchase.DHInventoryID, at, func(c context.Context) error {
		fresh, e := reader.GetReturnState(c, id)
		if e != nil {
			return e
		}
		if fresh.Sale != nil {
			return nil
		}
		return s.updater.UpdatePurchaseDHFields(c, id, u)
	})
}
