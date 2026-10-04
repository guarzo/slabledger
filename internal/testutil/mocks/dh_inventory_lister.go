package mocks

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type DHInventoryListerMock struct {
	UpdateInventoryStatusFn func(context.Context, int, inventory.DHInventoryStatusUpdate) (int, error)
	SyncChannelsFn          func(context.Context, int, []string) error
}

func (m *DHInventoryListerMock) UpdateInventoryStatus(c context.Context, id int, u inventory.DHInventoryStatusUpdate) (int, error) {
	if m.UpdateInventoryStatusFn != nil {
		return m.UpdateInventoryStatusFn(c, id, u)
	}
	return u.ListingPriceCents, nil
}
func (m *DHInventoryListerMock) SyncChannels(c context.Context, id int, ch []string) error {
	if m.SyncChannelsFn != nil {
		return m.SyncChannelsFn(c, id, ch)
	}
	return nil
}
