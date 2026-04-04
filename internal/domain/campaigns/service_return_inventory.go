package campaigns

import (
	"context"
	"fmt"
)

// DeleteSaleByPurchaseID removes the sale associated with a purchase,
// returning the item to unsold inventory.
func (s *service) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	sale, err := s.repo.GetSaleByPurchaseID(ctx, purchaseID)
	if err != nil {
		return fmt.Errorf("get sale for purchase %s: %w", purchaseID, err)
	}
	if err := s.repo.DeleteSale(ctx, sale.ID); err != nil {
		return fmt.Errorf("delete sale %s: %w", sale.ID, err)
	}
	return nil
}
