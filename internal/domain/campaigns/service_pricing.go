package campaigns

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/errors"
)

// validOverrideSources lists allowed values for SetPriceOverride when priceCents > 0.
// OverrideSourceAIAccepted is intentionally excluded — it is only set via AcceptAISuggestion.
var validOverrideSources = map[string]bool{
	string(OverrideSourceManual):     true,
	string(OverrideSourceCostMarkup): true,
}

func (s *service) UpdateBuyCost(ctx context.Context, purchaseID string, buyCostCents int) error {
	if buyCostCents < 0 {
		return errors.NewAppError(ErrCodeCampaignValidation, "buyCostCents must be >= 0")
	}
	return s.repo.UpdatePurchaseBuyCost(ctx, purchaseID, buyCostCents)
}

func (s *service) SetPriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if priceCents < 0 {
		return errors.NewAppError(ErrCodeCampaignValidation, "priceCents must be >= 0")
	}
	// When clearing, normalize source to empty before validation.
	if priceCents == 0 {
		source = ""
	}
	if priceCents > 0 && !validOverrideSources[source] {
		return errors.NewAppError(ErrCodeCampaignValidation, fmt.Sprintf("invalid override source: %q", source))
	}
	return s.repo.UpdatePurchasePriceOverride(ctx, purchaseID, priceCents, source)
}

func (s *service) SetAISuggestedPrice(ctx context.Context, purchaseID string, priceCents int) error {
	if priceCents <= 0 {
		return errors.NewAppError(ErrCodeCampaignValidation, "ai suggested priceCents must be > 0")
	}
	return s.repo.UpdatePurchaseAISuggestion(ctx, purchaseID, priceCents)
}

func (s *service) AcceptAISuggestion(ctx context.Context, purchaseID string) error {
	p, err := s.repo.GetPurchase(ctx, purchaseID)
	if err != nil {
		return err
	}
	if p.AISuggestedPriceCents <= 0 {
		return errors.NewAppError(ErrCodeNoAISuggestion, fmt.Sprintf("no AI suggestion to accept for purchase %s", purchaseID))
	}
	// Atomically set override and clear suggestion.
	return s.repo.AcceptAISuggestion(ctx, purchaseID, p.AISuggestedPriceCents)
}

func (s *service) DismissAISuggestion(ctx context.Context, purchaseID string) error {
	return s.repo.ClearPurchaseAISuggestion(ctx, purchaseID)
}

func (s *service) GetPriceOverrideStats(ctx context.Context) (*PriceOverrideStats, error) {
	stats, err := s.repo.GetPriceOverrideStats(ctx)
	if err != nil {
		return nil, err
	}
	stats.OverrideTotalUsd = float64(stats.OverrideTotalCents) / 100.0
	stats.SuggestionTotalUsd = float64(stats.SuggestionTotalCents) / 100.0
	return stats, nil
}
