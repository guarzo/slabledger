package mocks

import (
	"context"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
)

type DHInventoryMutationClientMock struct {
	UpdateInventoryFn     func(context.Context, int, dh.InventoryUpdate) (*dh.InventoryResult, error)
	SyncChannelsFn        func(context.Context, int, []string) (*dh.ChannelSyncResponse, error)
	RecordInventorySaleFn func(context.Context, int, string, dh.InventorySaleRequest) (*dh.InventorySaleResponse, error)
	VoidInventorySaleFn   func(context.Context, string, dh.VoidSaleRequest) (*dh.VoidSaleResponse, error)
}

func (m *DHInventoryMutationClientMock) UpdateInventory(c context.Context, id int, u dh.InventoryUpdate) (*dh.InventoryResult, error) {
	if m.UpdateInventoryFn != nil {
		return m.UpdateInventoryFn(c, id, u)
	}
	return nil, nil
}
func (m *DHInventoryMutationClientMock) SyncChannels(c context.Context, id int, ch []string) (*dh.ChannelSyncResponse, error) {
	if m.SyncChannelsFn != nil {
		return m.SyncChannelsFn(c, id, ch)
	}
	return nil, nil
}
func (m *DHInventoryMutationClientMock) RecordInventorySale(c context.Context, id int, k string, r dh.InventorySaleRequest) (*dh.InventorySaleResponse, error) {
	if m.RecordInventorySaleFn != nil {
		return m.RecordInventorySaleFn(c, id, k, r)
	}
	return nil, nil
}
func (m *DHInventoryMutationClientMock) VoidInventorySale(c context.Context, id string, r dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) {
	if m.VoidInventorySaleFn != nil {
		return m.VoidInventorySaleFn(c, id, r)
	}
	return nil, nil
}
