package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- PricingRepository ---

func (m *InMemoryCampaignStore) UpdateReviewedPrice(_ context.Context, purchaseID string, priceCents int, source string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.ReviewedPriceCents = priceCents
	if priceCents > 0 {
		p.ReviewedAt = time.Now().Format(time.RFC3339)
		p.ReviewSource = inventory.ReviewSource(source)
		p.AISuggestedPriceCents = 0
		p.AISuggestedAt = ""
	} else {
		p.ReviewedAt = ""
		p.ReviewSource = ""
	}
	return nil
}

func (m *InMemoryCampaignStore) GetReviewStats(_ context.Context, _ string) (inventory.ReviewStats, error) {
	return inventory.ReviewStats{}, nil
}

func (m *InMemoryCampaignStore) GetGlobalReviewStats(_ context.Context) (inventory.ReviewStats, error) {
	return inventory.ReviewStats{}, nil
}

func (m *InMemoryCampaignStore) CreatePriceFlag(_ context.Context, _ *inventory.PriceFlag) (int64, error) {
	return 0, nil
}

func (m *InMemoryCampaignStore) ListPriceFlags(_ context.Context, _ string) ([]inventory.PriceFlagWithContext, error) {
	return []inventory.PriceFlagWithContext{}, nil
}

func (m *InMemoryCampaignStore) ResolvePriceFlag(_ context.Context, _ int64, _ int64) error {
	return nil
}

func (m *InMemoryCampaignStore) HasOpenFlag(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *InMemoryCampaignStore) OpenFlagPurchaseIDs(ctx context.Context) (map[string]int64, error) {
	if m.OpenFlagPurchaseIDsFn != nil {
		return m.OpenFlagPurchaseIDsFn(ctx)
	}
	return map[string]int64{}, nil
}
