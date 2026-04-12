package inventory

import "context"

// PricingRepository handles price review and flagging operations.
type PricingRepository interface {
	UpdateReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error
	GetReviewStats(ctx context.Context, campaignID string) (ReviewStats, error)
	GetGlobalReviewStats(ctx context.Context) (ReviewStats, error)
	CreatePriceFlag(ctx context.Context, flag *PriceFlag) (int64, error)
	ListPriceFlags(ctx context.Context, status string) ([]PriceFlagWithContext, error)
	ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error
	HasOpenFlag(ctx context.Context, purchaseID string) (bool, error)
	OpenFlagPurchaseIDs(ctx context.Context) (map[string]int64, error)
}
