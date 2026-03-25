package scheduler

import (
	"context"
	"sync/atomic"

	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// mockPriceRepo is a minimal pricing.PriceRepository mock that counts StorePrice calls.
// It embeds mocks.MockPriceRepository for all default method implementations and only
// overrides StorePrice to add atomic call counting and configurable error return.
type mockPriceRepo struct {
	mocks.MockPriceRepository
	storeCount atomic.Int32
	storeErr   error
}

func (m *mockPriceRepo) StorePrice(_ context.Context, _ *pricing.PriceEntry) error {
	m.storeCount.Add(1)
	return m.storeErr
}
