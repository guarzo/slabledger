package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// PricingRepositoryMock implements inventory.PricingRepository with Fn-field pattern.
type PricingRepositoryMock struct {
	UpdateReviewedPriceFn  func(ctx context.Context, purchaseID string, priceCents int, source string) error
	GetReviewStatsFn       func(ctx context.Context, campaignID string) (inventory.ReviewStats, error)
	GetGlobalReviewStatsFn func(ctx context.Context) (inventory.ReviewStats, error)
	CreatePriceFlagFn      func(ctx context.Context, flag *inventory.PriceFlag) (int64, error)
	ListPriceFlagsFn       func(ctx context.Context, status string) ([]inventory.PriceFlagWithContext, error)
	ResolvePriceFlagFn     func(ctx context.Context, flagID int64, resolvedBy int64) error
	HasOpenFlagFn          func(ctx context.Context, purchaseID string) (bool, error)
	OpenFlagPurchaseIDsFn  func(ctx context.Context) (map[string]int64, error)
}

var _ inventory.PricingRepository = (*PricingRepositoryMock)(nil)

func (m *PricingRepositoryMock) UpdateReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.UpdateReviewedPriceFn != nil {
		return m.UpdateReviewedPriceFn(ctx, purchaseID, priceCents, source)
	}
	return nil
}

func (m *PricingRepositoryMock) GetReviewStats(ctx context.Context, campaignID string) (inventory.ReviewStats, error) {
	if m.GetReviewStatsFn != nil {
		return m.GetReviewStatsFn(ctx, campaignID)
	}
	return inventory.ReviewStats{}, nil
}

func (m *PricingRepositoryMock) GetGlobalReviewStats(ctx context.Context) (inventory.ReviewStats, error) {
	if m.GetGlobalReviewStatsFn != nil {
		return m.GetGlobalReviewStatsFn(ctx)
	}
	return inventory.ReviewStats{}, nil
}

func (m *PricingRepositoryMock) CreatePriceFlag(ctx context.Context, flag *inventory.PriceFlag) (int64, error) {
	if m.CreatePriceFlagFn != nil {
		return m.CreatePriceFlagFn(ctx, flag)
	}
	return 0, nil
}

func (m *PricingRepositoryMock) ListPriceFlags(ctx context.Context, status string) ([]inventory.PriceFlagWithContext, error) {
	if m.ListPriceFlagsFn != nil {
		return m.ListPriceFlagsFn(ctx, status)
	}
	return []inventory.PriceFlagWithContext{}, nil
}

func (m *PricingRepositoryMock) ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error {
	if m.ResolvePriceFlagFn != nil {
		return m.ResolvePriceFlagFn(ctx, flagID, resolvedBy)
	}
	return nil
}

func (m *PricingRepositoryMock) HasOpenFlag(ctx context.Context, purchaseID string) (bool, error) {
	if m.HasOpenFlagFn != nil {
		return m.HasOpenFlagFn(ctx, purchaseID)
	}
	return false, nil
}

func (m *PricingRepositoryMock) OpenFlagPurchaseIDs(ctx context.Context) (map[string]int64, error) {
	if m.OpenFlagPurchaseIDsFn != nil {
		return m.OpenFlagPurchaseIDsFn(ctx)
	}
	return map[string]int64{}, nil
}
