package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// ConfirmedReturnRepositoryMock exposes the durable core ports without an
// adapter dependency in domain boundary tests.
type ConfirmedReturnRepositoryMock struct {
	AssertDHPreparationAllowedFn func(context.Context) error
	PrepareDHMutationFn          func(context.Context, string, inventory.DHMutationRequest) (*inventory.DHMutationAttempt, error)
	OwnDHMutationFn              func(context.Context, string, *inventory.DHMutationAttempt) error
	SettleDHMutationFn           func(context.Context, string, *inventory.DHMutationAttempt, inventory.DHMutationSettlement) error
	GetReturnStateFn             func(context.Context, string) (*inventory.ConfirmedReturnState, error)
	ResolveReturnFn              func(context.Context, string, inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error)
	AssertMutationAllowedFn      func(context.Context, string) error
	DeleteLocalConfirmedSaleFn   func(context.Context, string, string) error
	PrepareReturnFn              func(context.Context, string, inventory.ConfirmReturnRequest, string, string) (*inventory.ConfirmedReturnState, error)
	CompleteReturnFn             func(context.Context, string, *inventory.ConfirmedReturnEpisode, *inventory.DHReturnResult) error
	RecordReturnFailureFn        func(context.Context, string, string, inventory.ReturnFailure, *inventory.DHReturnResult, bool) error
}

var _ inventory.ConfirmedReturnRepository = (*ConfirmedReturnRepositoryMock)(nil)

func (m *ConfirmedReturnRepositoryMock) AssertDHPreparationAllowed(ctx context.Context) error {
	if m.AssertDHPreparationAllowedFn != nil {
		return m.AssertDHPreparationAllowedFn(ctx)
	}
	return nil
}
func (m *ConfirmedReturnRepositoryMock) PrepareDHMutation(ctx context.Context, id string, r inventory.DHMutationRequest) (*inventory.DHMutationAttempt, error) {
	if m.PrepareDHMutationFn != nil {
		return m.PrepareDHMutationFn(ctx, id, r)
	}
	return nil, inventory.NewReturnConflict("missing_mock_attempt", "no mutation attempt configured")
}
func (m *ConfirmedReturnRepositoryMock) OwnDHMutation(ctx context.Context, id string, a *inventory.DHMutationAttempt) error {
	if m.OwnDHMutationFn != nil {
		return m.OwnDHMutationFn(ctx, id, a)
	}
	return nil
}
func (m *ConfirmedReturnRepositoryMock) SettleDHMutation(ctx context.Context, id string, a *inventory.DHMutationAttempt, r inventory.DHMutationSettlement) error {
	if m.SettleDHMutationFn != nil {
		return m.SettleDHMutationFn(ctx, id, a, r)
	}
	return nil
}
func (m *ConfirmedReturnRepositoryMock) GetReturnState(ctx context.Context, id string) (*inventory.ConfirmedReturnState, error) {
	if m.GetReturnStateFn != nil {
		return m.GetReturnStateFn(ctx, id)
	}
	return nil, inventory.ErrPurchaseNotFound
}
func (m *ConfirmedReturnRepositoryMock) ResolveReturn(ctx context.Context, id string, r inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error) {
	if m.ResolveReturnFn != nil {
		return m.ResolveReturnFn(ctx, id, r)
	}
	return nil, inventory.ErrPurchaseNotFound
}
func (m *ConfirmedReturnRepositoryMock) AssertMutationAllowed(ctx context.Context, id string) error {
	if m.AssertMutationAllowedFn != nil {
		return m.AssertMutationAllowedFn(ctx, id)
	}
	return nil
}
func (m *ConfirmedReturnRepositoryMock) DeleteLocalConfirmedSale(ctx context.Context, id, sale string) error {
	if m.DeleteLocalConfirmedSaleFn != nil {
		return m.DeleteLocalConfirmedSaleFn(ctx, id, sale)
	}
	return nil
}
func (m *ConfirmedReturnRepositoryMock) PrepareReturn(ctx context.Context, id string, r inventory.ConfirmReturnRequest, operation, key string) (*inventory.ConfirmedReturnState, error) {
	if m.PrepareReturnFn != nil {
		return m.PrepareReturnFn(ctx, id, r, operation, key)
	}
	return nil, inventory.NewReturnConflict("missing_mock_operation", "no return operation configured")
}
func (m *ConfirmedReturnRepositoryMock) CompleteReturn(ctx context.Context, id string, e *inventory.ConfirmedReturnEpisode, r *inventory.DHReturnResult) error {
	if m.CompleteReturnFn != nil {
		return m.CompleteReturnFn(ctx, id, e, r)
	}
	return nil
}
func (m *ConfirmedReturnRepositoryMock) RecordReturnFailure(ctx context.Context, id, operation string, f inventory.ReturnFailure, r *inventory.DHReturnResult, conflicted bool) error {
	if m.RecordReturnFailureFn != nil {
		return m.RecordReturnFailureFn(ctx, id, operation, f, r, conflicted)
	}
	return nil
}
