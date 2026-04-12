package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// MockPriceLookup is a test double for inventory.PriceLookup.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	pl := &MockPriceLookup{
//	    GetLastSoldCentsFn: func(ctx context.Context, card inventory.CardIdentity, grade float64) (int, error) {
//	        return 55000, nil
//	    },
//	    GetMarketSnapshotFn: func(ctx context.Context, card inventory.CardIdentity, grade float64) (*inventory.MarketSnapshot, error) {
//	        return &inventory.MarketSnapshot{LastSoldCents: 55000, MedianCents: 57000}, nil
//	    },
//	}
type MockPriceLookup struct {
	GetLastSoldCentsFn  func(ctx context.Context, card inventory.CardIdentity, grade float64) (int, error)
	GetMarketSnapshotFn func(ctx context.Context, card inventory.CardIdentity, grade float64) (*inventory.MarketSnapshot, error)
}

var _ inventory.PriceLookup = (*MockPriceLookup)(nil)

func (m *MockPriceLookup) GetLastSoldCents(ctx context.Context, card inventory.CardIdentity, grade float64) (int, error) {
	if m.GetLastSoldCentsFn != nil {
		return m.GetLastSoldCentsFn(ctx, card, grade)
	}
	return 0, nil
}

func (m *MockPriceLookup) GetMarketSnapshot(ctx context.Context, card inventory.CardIdentity, grade float64) (*inventory.MarketSnapshot, error) {
	if m.GetMarketSnapshotFn != nil {
		return m.GetMarketSnapshotFn(ctx, card, grade)
	}
	return nil, nil
}
