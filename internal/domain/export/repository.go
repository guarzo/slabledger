package export

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// SellSheetRepository handles sell sheet item persistence.
type SellSheetRepository interface {
	GetSellSheetItems(ctx context.Context) ([]string, error)
	AddSellSheetItems(ctx context.Context, purchaseIDs []string) error
	RemoveSellSheetItems(ctx context.Context, purchaseIDs []string) error
	ClearSellSheet(ctx context.Context) error
}

// ExportReader is the minimal repository interface for the export service.
type ExportReader interface {
	SellSheetRepository
	// Purchases (from PurchaseRepository)
	GetPurchasesByIDs(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Purchase, error)
	ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error)
	ListEbayFlaggedPurchases(ctx context.Context) ([]inventory.Purchase, error)
	ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error
	// Campaigns
	GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error)
	ListCampaigns(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
}
