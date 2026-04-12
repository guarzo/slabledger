package inventory

import "context"

// SellSheetRepository handles sell sheet item persistence (global, not per-user).
type SellSheetRepository interface {
	GetSellSheetItems(ctx context.Context) ([]string, error)
	AddSellSheetItems(ctx context.Context, purchaseIDs []string) error
	RemoveSellSheetItems(ctx context.Context, purchaseIDs []string) error
	ClearSellSheet(ctx context.Context) error
}
