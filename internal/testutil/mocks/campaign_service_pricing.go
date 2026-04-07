package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// MockPricingService is a test double for campaigns.PricingService.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockPricingService{
//	    SetPriceOverrideFn: func(ctx context.Context, purchaseID string, priceCents int, source string) error {
//	        return nil
//	    },
//	}
type MockPricingService struct {
	UpdateBuyCostFn        func(ctx context.Context, purchaseID string, buyCostCents int) error
	SetPriceOverrideFn     func(ctx context.Context, purchaseID string, priceCents int, source string) error
	SetAISuggestedPriceFn  func(ctx context.Context, purchaseID string, priceCents int) error
	AcceptAISuggestionFn   func(ctx context.Context, purchaseID string) error
	DismissAISuggestionFn  func(ctx context.Context, purchaseID string) error
	GetPriceOverrideStatsFn func(ctx context.Context) (*campaigns.PriceOverrideStats, error)
	SetReviewedPriceFn     func(ctx context.Context, purchaseID string, priceCents int, source string) error
	GetReviewStatsFn       func(ctx context.Context, campaignID string) (campaigns.ReviewStats, error)
	GetGlobalReviewStatsFn func(ctx context.Context) (campaigns.ReviewStats, error)
	CreatePriceFlagFn      func(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error)
	ListPriceFlagsFn       func(ctx context.Context, status string) ([]campaigns.PriceFlagWithContext, error)
	ResolvePriceFlagFn     func(ctx context.Context, flagID int64, resolvedBy int64) error
}

var _ campaigns.PricingService = (*MockPricingService)(nil)

func (m *MockPricingService) UpdateBuyCost(ctx context.Context, purchaseID string, buyCostCents int) error {
	if m.UpdateBuyCostFn != nil {
		return m.UpdateBuyCostFn(ctx, purchaseID, buyCostCents)
	}
	return nil
}

func (m *MockPricingService) SetPriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.SetPriceOverrideFn != nil {
		return m.SetPriceOverrideFn(ctx, purchaseID, priceCents, source)
	}
	return nil
}

func (m *MockPricingService) SetAISuggestedPrice(ctx context.Context, purchaseID string, priceCents int) error {
	if m.SetAISuggestedPriceFn != nil {
		return m.SetAISuggestedPriceFn(ctx, purchaseID, priceCents)
	}
	return nil
}

func (m *MockPricingService) AcceptAISuggestion(ctx context.Context, purchaseID string) error {
	if m.AcceptAISuggestionFn != nil {
		return m.AcceptAISuggestionFn(ctx, purchaseID)
	}
	return nil
}

func (m *MockPricingService) DismissAISuggestion(ctx context.Context, purchaseID string) error {
	if m.DismissAISuggestionFn != nil {
		return m.DismissAISuggestionFn(ctx, purchaseID)
	}
	return nil
}

func (m *MockPricingService) GetPriceOverrideStats(ctx context.Context) (*campaigns.PriceOverrideStats, error) {
	if m.GetPriceOverrideStatsFn != nil {
		return m.GetPriceOverrideStatsFn(ctx)
	}
	return &campaigns.PriceOverrideStats{}, nil
}

func (m *MockPricingService) SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if m.SetReviewedPriceFn != nil {
		return m.SetReviewedPriceFn(ctx, purchaseID, priceCents, source)
	}
	return nil
}

func (m *MockPricingService) GetReviewStats(ctx context.Context, campaignID string) (campaigns.ReviewStats, error) {
	if m.GetReviewStatsFn != nil {
		return m.GetReviewStatsFn(ctx, campaignID)
	}
	return campaigns.ReviewStats{}, nil
}

func (m *MockPricingService) GetGlobalReviewStats(ctx context.Context) (campaigns.ReviewStats, error) {
	if m.GetGlobalReviewStatsFn != nil {
		return m.GetGlobalReviewStatsFn(ctx)
	}
	return campaigns.ReviewStats{}, nil
}

func (m *MockPricingService) CreatePriceFlag(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error) {
	if m.CreatePriceFlagFn != nil {
		return m.CreatePriceFlagFn(ctx, purchaseID, userID, reason)
	}
	return 0, nil
}

func (m *MockPricingService) ListPriceFlags(ctx context.Context, status string) ([]campaigns.PriceFlagWithContext, error) {
	if m.ListPriceFlagsFn != nil {
		return m.ListPriceFlagsFn(ctx, status)
	}
	return []campaigns.PriceFlagWithContext{}, nil
}

func (m *MockPricingService) ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error {
	if m.ResolvePriceFlagFn != nil {
		return m.ResolvePriceFlagFn(ctx, flagID, resolvedBy)
	}
	return nil
}
