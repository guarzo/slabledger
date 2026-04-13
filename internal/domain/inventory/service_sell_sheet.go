package inventory

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/errors"
)

// recommendedPrice resolves the recommended price for a purchase using the hierarchy:
// 1. User-reviewed price (if set)
// 2. CL value (if > 0)
// 3. Market median (if > 0)
func recommendedPrice(p *Purchase, snapshot *MarketSnapshot) (int, string) {
	if p.ReviewedPriceCents > 0 {
		return p.ReviewedPriceCents, "user_reviewed"
	}
	if p.CLValueCents > 0 {
		return p.CLValueCents, "card_ladder"
	}
	if snapshot != nil && snapshot.MedianCents > 0 {
		return snapshot.MedianCents, "market"
	}
	return 0, ""
}

// --- Price Review ---
// NOTE: Sell-sheet generation (GenerateSellSheet, GenerateGlobalSellSheet,
// GenerateSelectedSellSheet, MatchShopifyPrices) lives in internal/domain/export/.
// inventory.Service does not expose those methods.

func (s *service) SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if priceCents < 0 {
		return errors.NewAppError(ErrCodeCampaignValidation, "price must be non-negative")
	}
	if priceCents > 0 {
		switch ReviewSource(source) {
		case ReviewSourceManual, ReviewSourceCL, ReviewSourceMarket, ReviewSourceLastSold, ReviewSourceCostMarkup, ReviewSourceMM:
			// valid
		default:
			return errors.NewAppError(ErrCodeCampaignValidation, "invalid review source: "+source)
		}
	}
	return s.pricing.UpdateReviewedPrice(ctx, purchaseID, priceCents, source)
}

func (s *service) GetReviewStats(ctx context.Context, campaignID string) (ReviewStats, error) {
	return s.pricing.GetReviewStats(ctx, campaignID)
}

func (s *service) GetGlobalReviewStats(ctx context.Context) (ReviewStats, error) {
	return s.pricing.GetGlobalReviewStats(ctx)
}
