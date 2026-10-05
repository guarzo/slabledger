package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// PurchaseMutationScopeMock supplies deterministic barriers around a real scope
// without replacing its transactional behavior in storage/service tests.
type PurchaseMutationScopeMock struct {
	WithPurchaseMutationFn func(context.Context, string, func(context.Context) error) error
}

func (m *PurchaseMutationScopeMock) WithPurchaseMutation(ctx context.Context, id string, fn func(context.Context) error) error {
	if m.WithPurchaseMutationFn != nil {
		return m.WithPurchaseMutationFn(ctx, id, fn)
	}
	return inventory.NewReturnConflict("missing_mock_scope", "no mutation scope configured")
}

type DHReturnerMock struct {
	ReturnInventoryToStockFn   func(context.Context, int, string) (*inventory.DHReturnResult, error)
	GetReturnInventoryStatusFn func(context.Context, int, string) (string, error)
}

func (m *DHReturnerMock) ReturnInventoryToStock(ctx context.Context, id int, key string) (*inventory.DHReturnResult, error) {
	if m.ReturnInventoryToStockFn != nil {
		return m.ReturnInventoryToStockFn(ctx, id, key)
	}
	return nil, inventory.NewReturnConflict("missing_mock_receipt", "no return response configured")
}
func (m *DHReturnerMock) GetReturnInventoryStatus(ctx context.Context, id int, cert string) (string, error) {
	if m.GetReturnInventoryStatusFn != nil {
		return m.GetReturnInventoryStatusFn(ctx, id, cert)
	}
	return "", inventory.NewReturnConflict("missing_mock_status", "no inventory status configured")
}
