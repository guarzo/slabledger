package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
)

// DHMappingDeleterMock implements handlers.DHMappingDeleter with the
// Fn-field pattern. Tests override DeleteAutoMappingFn to control the return
// value. Arguments from the most recent call are captured for assertions.
type DHMappingDeleterMock struct {
	DeleteAutoMappingFn func(ctx context.Context, cardName, setName, collectorNumber, provider string) (int64, error)

	Called          bool
	CardName        string
	SetName         string
	CollectorNumber string
	Provider        string
}

func (m *DHMappingDeleterMock) DeleteAutoMapping(ctx context.Context, cardName, setName, collectorNumber, provider string) (int64, error) {
	m.Called = true
	m.CardName = cardName
	m.SetName = setName
	m.CollectorNumber = collectorNumber
	m.Provider = provider
	if m.DeleteAutoMappingFn != nil {
		return m.DeleteAutoMappingFn(ctx, cardName, setName, collectorNumber, provider)
	}
	return 0, nil
}

// DHChannelDelisterMock implements handlers.DHChannelDelister with the
// Fn-field pattern. Tests override DelistChannelsFn; arguments from the most
// recent call are captured for assertions.
type DHChannelDelisterMock struct {
	DelistChannelsFn func(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)

	Called      bool
	InventoryID int
	Channels    []string
}

func (m *DHChannelDelisterMock) DelistChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error) {
	m.Called = true
	m.InventoryID = inventoryID
	m.Channels = channels
	if m.DelistChannelsFn != nil {
		return m.DelistChannelsFn(ctx, inventoryID, channels)
	}
	return &dh.ChannelSyncResponse{}, nil
}
