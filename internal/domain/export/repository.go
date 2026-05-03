package export

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// ExportReader is the minimal repository interface for the export service.
type ExportReader interface {
	// Purchases (from PurchaseRepository)
	GetPurchasesByIDs(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Purchase, error)
	ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error)
	// Campaigns
	GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error)
	ListCampaigns(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
}
