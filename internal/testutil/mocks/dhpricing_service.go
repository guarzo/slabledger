package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/dhpricing"
)

// MockDHPriceSyncService is a test double for dhpricing.Service.
// Each method delegates to a function field, allowing per-test configuration.
type MockDHPriceSyncService struct {
	SyncPurchasePriceFn    func(ctx context.Context, purchaseID string) dhpricing.SyncResult
	SyncDriftedPurchasesFn func(ctx context.Context) dhpricing.SyncBatchResult
}

var _ dhpricing.Service = (*MockDHPriceSyncService)(nil)

func (m *MockDHPriceSyncService) SyncPurchasePrice(ctx context.Context, purchaseID string) dhpricing.SyncResult {
	if m.SyncPurchasePriceFn != nil {
		return m.SyncPurchasePriceFn(ctx, purchaseID)
	}
	return dhpricing.SyncResult{}
}

func (m *MockDHPriceSyncService) SyncDriftedPurchases(ctx context.Context) dhpricing.SyncBatchResult {
	if m.SyncDriftedPurchasesFn != nil {
		return m.SyncDriftedPurchasesFn(ctx)
	}
	return dhpricing.SyncBatchResult{ByOutcome: map[dhpricing.Outcome]int{}}
}

// MockDHPriceSyncer is a test double for the handler-layer DHPriceSyncer
// interface (SyncPurchasePrice with no return). Lives here so tests across
// packages can share it without importing the handlers package. Structural
// typing means any struct with this method satisfies handlers.DHPriceSyncer.
type MockDHPriceSyncer struct {
	SyncPurchasePriceFn func(ctx context.Context, purchaseID string)
}

func (m *MockDHPriceSyncer) SyncPurchasePrice(ctx context.Context, purchaseID string) {
	if m.SyncPurchasePriceFn != nil {
		m.SyncPurchasePriceFn(ctx, purchaseID)
	}
}
