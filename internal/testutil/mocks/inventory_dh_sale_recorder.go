package mocks

import (
	"context"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// DHSaleRecorderMock is a test double for inventory.DHSaleRecorder. It records
// every RecordInventorySale/VoidInventorySale call in full — not just the
// inventory ID — so tests can assert on the idempotency key and SoldAt that
// were actually sent. That is exactly what spec §2's pure-function-of-
// persisted-columns invariant needs: a test asserting the same key and SoldAt
// across a retry catches a regression a call-count assertion would miss.
type DHSaleRecorderMock struct {
	RecordInventorySaleFn func(ctx context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error)
	VoidInventorySaleFn   func(ctx context.Context, dhSaleID, reason string) error

	mu       sync.Mutex
	recorded []inventory.DHSaleRequest
	voided   []DHVoidCall
}

// DHVoidCall is one recorded VoidInventorySale invocation.
type DHVoidCall struct {
	DHSaleID string
	Reason   string
}

func (m *DHSaleRecorderMock) RecordInventorySale(ctx context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
	m.mu.Lock()
	m.recorded = append(m.recorded, req)
	m.mu.Unlock()
	if m.RecordInventorySaleFn != nil {
		return m.RecordInventorySaleFn(ctx, req)
	}
	return &inventory.DHSaleResult{
		DHSaleID:   "dh-sale-mock",
		Delisted:   true,
		ItemStatus: "sold",
	}, nil
}

func (m *DHSaleRecorderMock) VoidInventorySale(ctx context.Context, dhSaleID, reason string) error {
	m.mu.Lock()
	m.voided = append(m.voided, DHVoidCall{DHSaleID: dhSaleID, Reason: reason})
	m.mu.Unlock()
	if m.VoidInventorySaleFn != nil {
		return m.VoidInventorySaleFn(ctx, dhSaleID, reason)
	}
	return nil
}

// RecordedSales returns every DHSaleRequest passed to RecordInventorySale, in
// order — including the idempotency key and SoldAt, so a caller can assert
// that a retry sent a byte-identical request.
func (m *DHSaleRecorderMock) RecordedSales() []inventory.DHSaleRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]inventory.DHSaleRequest(nil), m.recorded...)
}

// VoidedSales returns every VoidInventorySale call, in order.
func (m *DHSaleRecorderMock) VoidedSales() []DHVoidCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DHVoidCall(nil), m.voided...)
}

var _ inventory.DHSaleRecorder = (*DHSaleRecorderMock)(nil)
