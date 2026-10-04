package dhpricing

import (
	"context"
	"encoding/json"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type Option func(*service)

// ReturnStateReader must honor the owning context so purchase and current sale
// eligibility are read without opening another transaction or connection.
type ReturnStateReader interface {
	GetReturnState(context.Context, string) (*inventory.ConfirmedReturnState, error)
}

func WithMutationCoordinator(c *inventory.DHMutationCoordinator, g inventory.DHMutationGuards, r ReturnStateReader) Option {
	return func(s *service) {
		s.mutationRequired = true
		s.coordinator = c
		s.guards = g
		s.returnStates = r
	}
}
func pricePayload(p *inventory.Purchase) string {
	b, _ := json.Marshal(struct {
		ID     int
		Status string
		Price  int
	}{p.DHInventoryID, p.DHStatus, resolveListingPrice(p)})
	return string(b)
}
func (s *service) freshPriceState(ctx context.Context, id string) (*inventory.ConfirmedReturnState, error) {
	state, err := s.returnStates.GetReturnState(ctx, id)
	if err != nil {
		return nil, err
	}
	if state == nil || state.Purchase == nil || state.Purchase.ID != id {
		return nil, inventory.NewReturnConflict("identity_conflict", "price purchase identity changed")
	}
	return state, nil
}
func (s *service) SyncPurchasePrice(ctx context.Context, id string) SyncResult {
	if !s.mutationRequired {
		return s.syncPurchasePrice(ctx, id)
	}
	result := SyncResult{PurchaseID: id, Outcome: OutcomeError}
	if !inventory.DHDependenciesPresent(s.coordinator, s.guards, s.returnStates, s.lookup, s.updater, s.writer, s.resetter) {
		result.Err = inventory.NewReturnConflict("coordination_unavailable", "price coordination is required")
		return result
	}
	sold := inventory.NewReturnConflict("current_sale_present", "sold inventory cannot be price synced")
	err := s.coordinator.Run(ctx, id, func(c context.Context) (inventory.DHMutationRequest, error) {
		state, e := s.freshPriceState(c, id)
		if e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if state.Sale != nil {
			return inventory.DHMutationRequest{}, sold
		}
		if e := s.guards.AssertMutationAllowed(c, id); e != nil {
			return inventory.DHMutationRequest{}, e
		}
		return inventory.DHMutationRequest{Kind: "price", Phase: "patch", PayloadIdentity: pricePayload(state.Purchase)}, nil
	}, func(c context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		state, e := s.freshPriceState(c, id)
		if e != nil {
			return nil, e
		}
		if state.Sale != nil {
			// This newly prepared unkeyed attempt has never dispatched. Reject
			// it locally rather than fencing the sale's required retirement.
			// Run cannot replay or settle an older open unkeyed price attempt.
			result.Err = sold
			return &inventory.DHMutationSettlement{Outcome: "rejected", Receipt: "not-dispatched:current-sale"}, nil
		}
		p := state.Purchase
		if pricePayload(p) != a.PayloadIdentity {
			return nil, inventory.NewReturnConflict("identity_conflict", "prepared price request changed")
		}
		if e := s.guards.AssertMutationAllowed(c, id); e != nil {
			return nil, e
		}
		result = s.syncPurchaseSnapshot(c, p)
		if result.Err != nil {
			return nil, result.Err
		}
		receipt := "not-dispatched:price-ineligible"
		if result.Outcome == OutcomeSynced {
			receipt = "price-patch-success"
		}
		return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: receipt}, nil
	})
	if err != nil {
		result.Outcome = OutcomeError
		result.Err = err
	}
	return result
}
