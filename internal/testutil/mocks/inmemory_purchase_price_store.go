package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- PurchaseRepository: price overrides and AI suggestions ---

func (m *InMemoryCampaignStore) UpdatePurchasePriceOverride(_ context.Context, purchaseID string, priceCents int, source string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.OverridePriceCents = priceCents
	p.OverrideSource = inventory.OverrideSource(source)
	if priceCents > 0 {
		p.OverrideSetAt = time.Now().Format(time.RFC3339)
		p.AISuggestedPriceCents = 0
		p.AISuggestedAt = ""
	} else {
		p.OverrideSetAt = ""
	}
	return nil
}

func (m *InMemoryCampaignStore) UpdatePurchaseAISuggestion(_ context.Context, purchaseID string, priceCents int) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.AISuggestedPriceCents = priceCents
	p.AISuggestedAt = time.Now().Format(time.RFC3339)
	return nil
}

func (m *InMemoryCampaignStore) ClearPurchaseAISuggestion(_ context.Context, purchaseID string) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	p.AISuggestedPriceCents = 0
	p.AISuggestedAt = ""
	return nil
}

func (m *InMemoryCampaignStore) AcceptAISuggestion(_ context.Context, purchaseID string, priceCents int) error {
	p, ok := m.Purchases[purchaseID]
	if !ok {
		return inventory.ErrPurchaseNotFound
	}
	if p.AISuggestedPriceCents != priceCents {
		return inventory.ErrNoAISuggestion
	}
	p.OverridePriceCents = priceCents
	p.OverrideSource = inventory.OverrideSourceAIAccepted
	p.OverrideSetAt = time.Now().Format(time.RFC3339)
	p.AISuggestedPriceCents = 0
	p.AISuggestedAt = ""
	return nil
}

func (m *InMemoryCampaignStore) GetPriceOverrideStats(_ context.Context) (*inventory.PriceOverrideStats, error) {
	var stats inventory.PriceOverrideStats
	for id, p := range m.Purchases {
		if m.PurchaseSales[id] {
			continue // sold
		}
		stats.TotalUnsold++
		if p.OverridePriceCents > 0 {
			stats.OverrideCount++
			stats.OverrideTotalCents += p.OverridePriceCents
			switch p.OverrideSource {
			case inventory.OverrideSourceManual:
				stats.ManualCount++
			case inventory.OverrideSourceCostMarkup:
				stats.CostMarkupCount++
			case inventory.OverrideSourceAIAccepted:
				stats.AIAcceptedCount++
			}
		}
		if p.AISuggestedPriceCents > 0 {
			stats.PendingSuggestions++
			stats.SuggestionTotalCents += p.AISuggestedPriceCents
		}
	}
	return &stats, nil
}
