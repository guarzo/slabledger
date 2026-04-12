package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Compile-time interface check
var _ inventory.SellSheetRepository = (*MockSellSheetRepository)(nil)

// MockSellSheetRepository implements inventory.SellSheetRepository for testing.
// Each method delegates to a function field; nil fields return zero values.
type MockSellSheetRepository struct {
	GetSellSheetItemsFn    func(ctx context.Context) ([]string, error)
	AddSellSheetItemsFn    func(ctx context.Context, purchaseIDs []string) error
	RemoveSellSheetItemsFn func(ctx context.Context, purchaseIDs []string) error
	ClearSellSheetFn       func(ctx context.Context) error
}

// NewMockSellSheetRepository creates a new MockSellSheetRepository.
func NewMockSellSheetRepository() *MockSellSheetRepository {
	return &MockSellSheetRepository{}
}

func (m *MockSellSheetRepository) GetSellSheetItems(ctx context.Context) ([]string, error) {
	if m.GetSellSheetItemsFn != nil {
		return m.GetSellSheetItemsFn(ctx)
	}
	return []string{}, nil
}

func (m *MockSellSheetRepository) AddSellSheetItems(ctx context.Context, purchaseIDs []string) error {
	if m.AddSellSheetItemsFn != nil {
		return m.AddSellSheetItemsFn(ctx, purchaseIDs)
	}
	return nil
}

func (m *MockSellSheetRepository) RemoveSellSheetItems(ctx context.Context, purchaseIDs []string) error {
	if m.RemoveSellSheetItemsFn != nil {
		return m.RemoveSellSheetItemsFn(ctx, purchaseIDs)
	}
	return nil
}

func (m *MockSellSheetRepository) ClearSellSheet(ctx context.Context) error {
	if m.ClearSellSheetFn != nil {
		return m.ClearSellSheetFn(ctx)
	}
	return nil
}
