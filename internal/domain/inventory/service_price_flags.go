package inventory

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/errors"
)

// --- Price Flags ---

func (s *service) CreatePriceFlag(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error) {
	if !PriceFlagReason(reason).Valid() {
		return 0, errors.NewAppError(ErrCodeCampaignValidation, "invalid flag reason: "+reason)
	}
	// Verify purchase exists
	if _, err := s.purchases.GetPurchase(ctx, purchaseID); err != nil {
		return 0, err
	}
	flag := &PriceFlag{
		PurchaseID: purchaseID,
		FlaggedBy:  userID,
		FlaggedAt:  time.Now(),
		Reason:     PriceFlagReason(reason),
	}
	return s.pricing.CreatePriceFlag(ctx, flag)
}

func (s *service) ListPriceFlags(ctx context.Context, status string) ([]PriceFlagWithContext, error) {
	return s.pricing.ListPriceFlags(ctx, status)
}

func (s *service) ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error {
	return s.pricing.ResolvePriceFlag(ctx, flagID, resolvedBy)
}
