package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/arbitrage"
)

// MockBatchPricer is a test double for arbitrage.BatchPricer.
//
// Usage:
//
//	mock := &MockBatchPricer{
//	    ResolveDHCardIDFn: func(ctx context.Context, cardName, setName, cardNumber string) (int, error) {
//	        return 42, nil
//	    },
//	    BatchPriceDistributionFn: func(ctx context.Context, cardIDs []int) (map[int]arbitrage.GradedDistribution, error) {
//	        return map[int]arbitrage.GradedDistribution{}, nil
//	    },
//	}
type MockBatchPricer struct {
	ResolveDHCardIDFn        func(ctx context.Context, cardName, setName, cardNumber string) (int, error)
	BatchPriceDistributionFn func(ctx context.Context, cardIDs []int) (map[int]arbitrage.GradedDistribution, error)
}

func (m *MockBatchPricer) ResolveDHCardID(ctx context.Context, cardName, setName, cardNumber string) (int, error) {
	if m.ResolveDHCardIDFn != nil {
		return m.ResolveDHCardIDFn(ctx, cardName, setName, cardNumber)
	}
	return 0, nil
}

func (m *MockBatchPricer) BatchPriceDistribution(ctx context.Context, cardIDs []int) (map[int]arbitrage.GradedDistribution, error) {
	if m.BatchPriceDistributionFn != nil {
		return m.BatchPriceDistributionFn(ctx, cardIDs)
	}
	return map[int]arbitrage.GradedDistribution{}, nil
}

var _ arbitrage.BatchPricer = (*MockBatchPricer)(nil)
