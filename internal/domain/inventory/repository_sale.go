package inventory

import "context"

// SaleRepository handles sale persistence.
type SaleRepository interface {
	CreateSale(ctx context.Context, s *Sale) error
	GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*Sale, error)
	GetSalesByPurchaseIDs(ctx context.Context, purchaseIDs []string) (map[string]*Sale, error)
	ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Sale, error)
	DeleteSale(ctx context.Context, saleID string) error
	DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error
}
