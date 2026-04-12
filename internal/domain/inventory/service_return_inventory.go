package inventory

import (
	"context"
	"fmt"
)

// DeleteSaleByPurchaseID removes the sale associated with a purchase,
// returning the item to unsold inventory.
func (s *service) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	if err := s.sales.DeleteSaleByPurchaseID(ctx, purchaseID); err != nil {
		return fmt.Errorf("delete sale for purchase %s: %w", purchaseID, err)
	}
	return nil
}
