package dhlisting

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"time"
)

func WithReconcileMutationGuards(g inventory.DHMutationGuards, r ReturnStateReader) ReconcilerOption {
	return func(s *reconcileService) { s.observationRequired = true; s.guards = g; s.returnStates = r }
}
func (s *reconcileService) applyReconcileObservation(ctx context.Context, p inventory.Purchase, at time.Time, apply func(context.Context) error) (bool, error) {
	if !s.observationRequired {
		return true, apply(ctx)
	}
	valid := false
	applied, e := s.guards.ApplyDHObservation(ctx, p.ID, p.DHInventoryID, at, func(c context.Context) error {
		state, e := s.returnStates.GetReturnState(c, p.ID)
		if e != nil {
			return e
		}
		if state.Sale != nil {
			return nil
		}
		valid = true
		return apply(c)
	})
	return applied && valid, e
}
