package mocks

import (
	"context"
	"sync"
)

// DHSoldNotifierMock is a test double for inventory.DHSoldNotifier. It records
// every inventory ID it is asked to retire so tests can assert that a sale
// path actually notified DH, and MarkInventorySoldFn can override the result
// to exercise the best-effort failure branches.
type DHSoldNotifierMock struct {
	MarkInventorySoldFn func(ctx context.Context, dhInventoryID int) error

	mu     sync.Mutex
	called []int
}

func (m *DHSoldNotifierMock) MarkInventorySold(ctx context.Context, dhInventoryID int) error {
	m.mu.Lock()
	m.called = append(m.called, dhInventoryID)
	m.mu.Unlock()
	if m.MarkInventorySoldFn != nil {
		return m.MarkInventorySoldFn(ctx, dhInventoryID)
	}
	return nil
}

// MarkedSold returns the inventory IDs passed to MarkInventorySold, in order.
func (m *DHSoldNotifierMock) MarkedSold() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]int(nil), m.called...)
}
