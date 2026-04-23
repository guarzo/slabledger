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

// MockPurchaseLister is a test double for liquidation.PurchaseLister.
type MockPurchaseLister struct {
	ListUnsoldForLiquidationFn func(ctx context.Context) ([]liquidation.UnsoldPurchase, error)
}

var _ liquidation.PurchaseLister = (*MockPurchaseLister)(nil)

func (m *MockPurchaseLister) ListUnsoldForLiquidation(ctx context.Context) ([]liquidation.UnsoldPurchase, error) {
	if m.ListUnsoldForLiquidationFn != nil {
		return m.ListUnsoldForLiquidationFn(ctx)
	}
	return nil, nil
}

// MockCompReader is a test double for liquidation.CompReader.
type MockCompReader struct {
	GetSaleCompsForCardFn func(ctx context.Context, gemRateID, condition string) ([]liquidation.SaleComp, error)
}

var _ liquidation.CompReader = (*MockCompReader)(nil)

func (m *MockCompReader) GetSaleCompsForCard(ctx context.Context, gemRateID, condition string) ([]liquidation.SaleComp, error) {
	if m.GetSaleCompsForCardFn != nil {
		return m.GetSaleCompsForCardFn(ctx, gemRateID, condition)
	}
	return nil, nil
}

// MockPriceWriter is a test double for liquidation.PriceWriter.
type MockPriceWriter struct {
	SetReviewedPriceFn func(ctx context.Context, purchaseID string, priceCents int, source string) error
	Applied            map[string]int
}

var _ liquidation.PriceWriter = (*MockPriceWriter)(nil)

func (m *MockPriceWriter) SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.SetReviewedPriceFn != nil {
		return m.SetReviewedPriceFn(ctx, purchaseID, priceCents, source)
	}
	if m.Applied == nil {
		m.Applied = make(map[string]int)
	}
	m.Applied[purchaseID] = priceCents
	return nil
}
