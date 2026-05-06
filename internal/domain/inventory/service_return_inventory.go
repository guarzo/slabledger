package inventory

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DeleteSaleByPurchaseID removes the sale associated with a purchase,
// returning the item to unsold inventory.
//
// When the sale was originally recorded (see ConfirmOrdersSales), local
// dh_status was flipped to 'sold' and DH was notified to retire the inventory
// item. Reversing only the campaign_sales row would leave the purchase with
// dh_status='sold' and a stale dh_inventory_id, which lands it in the
// "Pending DH Listing" tab but blocks the List action with a
// "Purchase is not in_stock on DH" 409. Clear the DH linkage so the push
// pipeline re-enrolls the item the way it would for any other un-pushed
// purchase.
func (s *service) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	// Snapshot the DH state before deleting so we know whether a reset is
	// needed afterwards. A missing purchase is surfaced via the sale delete.
	p, getErr := s.purchases.GetPurchase(ctx, purchaseID)

	if err := s.sales.DeleteSaleByPurchaseID(ctx, purchaseID); err != nil {
		return fmt.Errorf("delete sale for purchase %s: %w", purchaseID, err)
	}

	if getErr != nil || p == nil {
		// Sale was deleted; we just can't determine whether to reset DH state.
		// Log and return success — the user's primary action (un-sell) succeeded.
		if s.logger != nil && getErr != nil {
			s.logger.Warn(ctx, "delete sale: could not load purchase to reset DH state",
				observability.String("purchaseID", purchaseID),
				observability.Err(getErr))
		}
		return nil
	}

	// Only items that were marked sold on DH need their DH linkage cleared.
	// 'in_stock' / 'listed' / '' are all fine to leave alone.
	if p.DHStatus != DHStatusSold {
		return nil
	}

	if err := s.purchases.ResetDHFieldsForRepushDueToDelete(ctx, purchaseID); err != nil {
		// The sale is already gone; surface the failure so the caller can
		// retry, but don't pretend the un-sell didn't happen.
		return fmt.Errorf("reset dh fields after un-sell for purchase %s: %w", purchaseID, err)
	}
	return nil
}
