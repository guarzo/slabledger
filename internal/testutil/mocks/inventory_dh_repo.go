package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// DHRepositoryMock implements inventory.DHRepository with Fn-field pattern.
type DHRepositoryMock struct {
	GetDHPushConfigFn  func(ctx context.Context) (*inventory.DHPushConfig, error)
	SaveDHPushConfigFn func(ctx context.Context, cfg *inventory.DHPushConfig) error
}

var _ inventory.DHRepository = (*DHRepositoryMock)(nil)

func (m *DHRepositoryMock) GetDHPushConfig(ctx context.Context) (*inventory.DHPushConfig, error) {
	if m.GetDHPushConfigFn != nil {
		return m.GetDHPushConfigFn(ctx)
	}
	return nil, nil
}

func (m *DHRepositoryMock) SaveDHPushConfig(ctx context.Context, cfg *inventory.DHPushConfig) error {
	if m.SaveDHPushConfigFn != nil {
		return m.SaveDHPushConfigFn(ctx, cfg)
	}
	return nil
}
