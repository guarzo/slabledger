package inventory

import (
	"context"
	"time"
)

// SaleNeedingDHRecord is a sale awaiting a DH-side sale handle (spec §5b),
// joined with the two purchase fields the recorder needs but Sale does not
// carry: the DH inventory row to record the sale against, and the purchase
// date used to derive the DH-required sold_at (see DeriveDHSoldAt).
type SaleNeedingDHRecord struct {
	Sale
	DHInventoryID int
	PurchaseDate  string
}

// SaleRepository handles sale persistence.
type SaleRepository interface {
	CreateSale(ctx context.Context, s *Sale) error
	GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*Sale, error)
	GetSalesByPurchaseIDs(ctx context.Context, purchaseIDs []string) (map[string]*Sale, error)
	ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Sale, error)
	DeleteSale(ctx context.Context, saleID string) error
	DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error
	UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string, forcedLiquidation bool) error
	// SetSaleIdempotencyKeyIfAbsent is a compare-and-set (spec §5a). It returns
	// the EFFECTIVE key: the one it just wrote, or the pre-existing one if
	// another writer won the race.
	SetSaleIdempotencyKeyIfAbsent(ctx context.Context, saleID, key string) (string, error)
	SetSaleDHSaleID(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error
	// ListSalesNeedingDHRecord returns sales scoped by our own state (missing
	// dh_sale_id, linked to a live DH inventory row, no open conflict) that
	// need recording or replaying against DH (spec §5b).
	ListSalesNeedingDHRecord(ctx context.Context, limit int) ([]SaleNeedingDHRecord, error)
}
