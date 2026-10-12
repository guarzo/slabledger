package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type DHLivePriceUpdaterMock struct {
	UpdateLivePriceFn func(context.Context, int, string, int) (*inventory.DHLivePriceResult, error)
}

var _ inventory.DHLivePriceUpdater = (*DHLivePriceUpdaterMock)(nil)

func (m *DHLivePriceUpdaterMock) UpdateLivePrice(ctx context.Context, target int, key string, cents int) (*inventory.DHLivePriceResult, error) {
	if m.UpdateLivePriceFn != nil {
		return m.UpdateLivePriceFn(ctx, target, key, cents)
	}
	return nil, nil
}
