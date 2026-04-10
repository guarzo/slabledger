package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// MockPendingItemRepository is a test mock for campaigns.PendingItemRepository.
type MockPendingItemRepository struct {
	SavePendingItemsFn   func(ctx context.Context, items []campaigns.PendingItem) error
	ListPendingItemsFn   func(ctx context.Context) ([]campaigns.PendingItem, error)
	GetPendingItemByIDFn func(ctx context.Context, id string) (*campaigns.PendingItem, error)
	ResolvePendingItemFn func(ctx context.Context, id string, campaignID string) error
	DismissPendingItemFn func(ctx context.Context, id string) error
	CountPendingItemsFn  func(ctx context.Context) (int, error)
}

func (m *MockPendingItemRepository) SavePendingItems(ctx context.Context, items []campaigns.PendingItem) error {
	if m.SavePendingItemsFn != nil {
		return m.SavePendingItemsFn(ctx, items)
	}
	return nil
}

func (m *MockPendingItemRepository) ListPendingItems(ctx context.Context) ([]campaigns.PendingItem, error) {
	if m.ListPendingItemsFn != nil {
		return m.ListPendingItemsFn(ctx)
	}
	return nil, nil
}

func (m *MockPendingItemRepository) GetPendingItemByID(ctx context.Context, id string) (*campaigns.PendingItem, error) {
	if m.GetPendingItemByIDFn != nil {
		return m.GetPendingItemByIDFn(ctx, id)
	}
	return nil, campaigns.ErrPendingItemNotFound
}

func (m *MockPendingItemRepository) ResolvePendingItem(ctx context.Context, id string, campaignID string) error {
	if m.ResolvePendingItemFn != nil {
		return m.ResolvePendingItemFn(ctx, id, campaignID)
	}
	return nil
}

func (m *MockPendingItemRepository) DismissPendingItem(ctx context.Context, id string) error {
	if m.DismissPendingItemFn != nil {
		return m.DismissPendingItemFn(ctx, id)
	}
	return nil
}

func (m *MockPendingItemRepository) CountPendingItems(ctx context.Context) (int, error) {
	if m.CountPendingItemsFn != nil {
		return m.CountPendingItemsFn(ctx)
	}
	return 0, nil
}
