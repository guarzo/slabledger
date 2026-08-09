package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/liquidation"
)

// MockLiquidationService is a test double for liquidation.Service.
type MockLiquidationService struct {
	PreviewFn func(ctx context.Context, req liquidation.PreviewRequest) (liquidation.PreviewResponse, error)
	ApplyFn   func(ctx context.Context, req liquidation.ApplyRequest) (liquidation.ApplyResult, error)
}

var _ liquidation.Service = (*MockLiquidationService)(nil)

func (m *MockLiquidationService) Preview(ctx context.Context, req liquidation.PreviewRequest) (liquidation.PreviewResponse, error) {
	if m.PreviewFn != nil {
		return m.PreviewFn(ctx, req)
	}
	return liquidation.PreviewResponse{}, nil
}

func (m *MockLiquidationService) Apply(ctx context.Context, req liquidation.ApplyRequest) (liquidation.ApplyResult, error) {
	if m.ApplyFn != nil {
		return m.ApplyFn(ctx, req)
	}
	return liquidation.ApplyResult{}, nil
}
