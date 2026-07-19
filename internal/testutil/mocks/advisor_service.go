package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/advisor"
)

// MockAdvisorService is a test mock for advisor.Service.
type MockAdvisorService struct {
	GenerateDigestFn     func(ctx context.Context, stream func(advisor.StreamEvent)) error
	AnalyzeLiquidationFn func(ctx context.Context, stream func(advisor.StreamEvent)) error
	CollectDigestFn      func(ctx context.Context) (string, error)
	CollectLiquidationFn func(ctx context.Context) (string, error)
}

var _ advisor.Service = (*MockAdvisorService)(nil)

func (m *MockAdvisorService) GenerateDigest(ctx context.Context, stream func(advisor.StreamEvent)) error {
	if m.GenerateDigestFn != nil {
		return m.GenerateDigestFn(ctx, stream)
	}
	return nil
}

func (m *MockAdvisorService) AnalyzeLiquidation(ctx context.Context, stream func(advisor.StreamEvent)) error {
	if m.AnalyzeLiquidationFn != nil {
		return m.AnalyzeLiquidationFn(ctx, stream)
	}
	return nil
}

func (m *MockAdvisorService) CollectDigest(ctx context.Context) (string, error) {
	if m.CollectDigestFn != nil {
		return m.CollectDigestFn(ctx)
	}
	return "", nil
}

func (m *MockAdvisorService) CollectLiquidation(ctx context.Context) (string, error) {
	if m.CollectLiquidationFn != nil {
		return m.CollectLiquidationFn(ctx)
	}
	return "", nil
}
