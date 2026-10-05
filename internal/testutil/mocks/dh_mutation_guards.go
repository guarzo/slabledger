package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// DHMutationGuardsMock supports barriers around real scoped guards.
type DHMutationGuardsMock struct {
	AssertMutationAllowedFn    func(context.Context, string) error
	AuthorizeReturnedListingFn func(context.Context, string, string) error
	IsReturnedOrderFn          func(context.Context, string, string) (bool, error)
	ObservationTimeFn          func(context.Context) (time.Time, error)
	ApplyDHObservationFn       func(context.Context, string, int, time.Time, func(context.Context) error) (bool, error)
	ApplyDHSoldObservationFn   func(context.Context, string, string, int, time.Time, func(context.Context) error) (bool, error)
}

var _ inventory.DHMutationGuards = (*DHMutationGuardsMock)(nil)

func (m *DHMutationGuardsMock) AssertMutationAllowed(ctx context.Context, id string) error {
	if m.AssertMutationAllowedFn != nil {
		return m.AssertMutationAllowedFn(ctx, id)
	}
	return nil
}
func (m *DHMutationGuardsMock) AuthorizeReturnedListing(ctx context.Context, id, op string) error {
	if m.AuthorizeReturnedListingFn != nil {
		return m.AuthorizeReturnedListingFn(ctx, id, op)
	}
	return nil
}
func (m *DHMutationGuardsMock) IsReturnedOrder(ctx context.Context, id, order string) (bool, error) {
	if m.IsReturnedOrderFn != nil {
		return m.IsReturnedOrderFn(ctx, id, order)
	}
	return false, nil
}
func (m *DHMutationGuardsMock) ObservationTime(ctx context.Context) (time.Time, error) {
	if m.ObservationTimeFn != nil {
		return m.ObservationTimeFn(ctx)
	}
	return time.Time{}, nil
}
func (m *DHMutationGuardsMock) ApplyDHObservation(ctx context.Context, id string, target int, at time.Time, fn func(context.Context) error) (bool, error) {
	if m.ApplyDHObservationFn != nil {
		return m.ApplyDHObservationFn(ctx, id, target, at, fn)
	}
	return false, nil
}
func (m *DHMutationGuardsMock) ApplyDHSoldObservation(ctx context.Context, id, sale string, target int, at time.Time, fn func(context.Context) error) (bool, error) {
	if m.ApplyDHSoldObservationFn != nil {
		return m.ApplyDHSoldObservationFn(ctx, id, sale, target, at, fn)
	}
	return false, nil
}
