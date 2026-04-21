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

// DHInventoryPusherMock implements handlers.DHInventoryPusher with the
// Fn-field pattern. Tests override PushInventoryFn to return a synthesized
// response (or error); the call count and the items sent on the most recent
// call are captured for assertions.
type DHInventoryPusherMock struct {
	PushInventoryFn func(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)

	CallCount int
	LastItems []dh.InventoryItem
}

func (m *DHInventoryPusherMock) PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
	m.CallCount++
	m.LastItems = items
	if m.PushInventoryFn != nil {
		return m.PushInventoryFn(ctx, items)
	}
	return &dh.InventoryPushResponse{}, nil
}

// DHCardIDSaverMock implements handlers.DHCardIDSaver with the Fn-field
// pattern. Every method returns a zero value by default; tests override the
// corresponding *Fn field to customize behavior.
type DHCardIDSaverMock struct {
	GetExternalIDFn  func(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
	SaveExternalIDFn func(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
	GetMappedSetFn   func(ctx context.Context, provider string) (map[string]string, error)
}

func (m *DHCardIDSaverMock) GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error) {
	if m.GetExternalIDFn != nil {
		return m.GetExternalIDFn(ctx, cardName, setName, collectorNumber, provider)
	}
	return "", nil
}

func (m *DHCardIDSaverMock) SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error {
	if m.SaveExternalIDFn != nil {
		return m.SaveExternalIDFn(ctx, cardName, setName, collectorNumber, provider, externalID)
	}
	return nil
}

func (m *DHCardIDSaverMock) GetMappedSet(ctx context.Context, provider string) (map[string]string, error) {
	if m.GetMappedSetFn != nil {
		return m.GetMappedSetFn(ctx, provider)
	}
	return map[string]string{}, nil
}
