package mocks

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type ConfirmedReturnServiceMock struct {
	ConfirmReturnFn  func(context.Context, string, inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error)
	GetReturnStateFn func(context.Context, string) (*inventory.ConfirmedReturnState, error)
	CheckDHSaleFn    func(context.Context, string) (*inventory.DHSaleCheck, error)
}

func (m *ConfirmedReturnServiceMock) ConfirmReturn(ctx context.Context, id string, req inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error) {
	if m.ConfirmReturnFn != nil {
		return m.ConfirmReturnFn(ctx, id, req)
	}
	return &inventory.ConfirmedReturnState{}, nil
}
func (m *ConfirmedReturnServiceMock) CheckDHSale(ctx context.Context, id string) (*inventory.DHSaleCheck, error) {
	if m.CheckDHSaleFn != nil {
		return m.CheckDHSaleFn(ctx, id)
	}
	return nil, inventory.NewReturnConflict("coordination_unavailable", "DH sale check not configured")
}
func (m *ConfirmedReturnServiceMock) GetReturnState(ctx context.Context, id string) (*inventory.ConfirmedReturnState, error) {
	if m.GetReturnStateFn != nil {
		return m.GetReturnStateFn(ctx, id)
	}
	return &inventory.ConfirmedReturnState{}, nil
}
