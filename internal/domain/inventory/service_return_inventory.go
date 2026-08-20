package inventory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DeleteSaleByPurchaseID removes the sale associated with a purchase,
// returning the item to unsold inventory.
//
// Ordering (design §7): void on DH -> reset the purchase's DH linkage ->
// delete the sale row, LAST. Deleting last is what makes the sequence
// retry-safe: the sale row holds dh_sale_id, the only handle that can void, so
// a failure at the reset step leaves the row intact and the whole sequence can
// simply be retried. The reverse order loses the handle first and strands the
// purchase with no way to reverse the DH-side sale.
//
// The intermediate state (purchase reset, sale row still present) is safe
// rather than merely tolerable: the push query excludes purchases that have a
// sale row (`AND s.id IS NULL`, purchase_dh_query_store.go:21), so the item
// cannot be relisted while it still reads as sold.
//
// A purchase whose sale never reached DH — no dh_sale_id and no
// dh_idempotency_key, e.g. an eBay/Shopify import with no DH history — skips
// straight to the local reset and delete: there is nothing to void.
func (s *service) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	sale, err := s.sales.GetSaleByPurchaseID(ctx, purchaseID)
	if err != nil {
		return fmt.Errorf("delete sale for purchase %s: %w", purchaseID, err)
	}

	if sale.DHSaleID != "" || sale.DHIdempotencyKey != "" {
		p, getErr := s.purchases.GetPurchase(ctx, purchaseID)
		if getErr != nil {
			return fmt.Errorf("delete sale for purchase %s: load purchase: %w", purchaseID, getErr)
		}

		dhSaleID, handleErr := s.ensureDHSaleHandle(ctx, sale, p)
		if handleErr != nil {
			return fmt.Errorf("delete sale for purchase %s: recover dh sale handle: %w", purchaseID, handleErr)
		}

		if dhSaleID != "" {
			if err := s.voidDHSale(ctx, purchaseID, dhSaleID); err != nil {
				return fmt.Errorf("delete sale for purchase %s: void on dh: %w", purchaseID, err)
			}
			if err := s.purchases.ResetDHFieldsForRelistAfterVoid(ctx, purchaseID); err != nil {
				// The sale row is still here (we delete last), so dh_sale_id
				// survives and a retry re-runs void — idempotent, DH returns
				// reversed:false — then reset, harmlessly.
				return fmt.Errorf("delete sale for purchase %s: reset dh fields after void: %w", purchaseID, err)
			}
		}
	}

	if err := s.sales.DeleteSaleByPurchaseID(ctx, purchaseID); err != nil {
		return fmt.Errorf("delete sale for purchase %s: %w", purchaseID, err)
	}
	return nil
}

// ensureDHSaleHandle returns the dh_sale_id needed to void, replaying the
// original request if the sale has a key but its handle was never persisted
// (design §5b "Concurrent un-sell"). It never skips the void and deletes the
// row anyway — that would orphan a recorded sale on DH with no way to reverse
// it.
func (s *service) ensureDHSaleHandle(ctx context.Context, sale *Sale, p *Purchase) (string, error) {
	if sale.DHSaleID != "" {
		return sale.DHSaleID, nil
	}
	if s.dhSaleRecorder == nil || p.DHInventoryID == 0 {
		return "", nil
	}

	req := s.buildDHSaleRequest(sale, p, sale.DHIdempotencyKey)
	result, err := s.dhSaleRecorder.RecordInventorySale(ctx, req)
	if err != nil {
		return "", err
	}
	if setErr := s.sales.SetSaleDHSaleID(ctx, sale.ID, result.DHSaleID, time.Now()); setErr != nil && s.logger != nil {
		s.logger.Warn(ctx, "un-sell: failed to persist recovered dh_sale_id",
			observability.String("purchaseID", p.ID),
			observability.String("dhSaleID", result.DHSaleID),
			observability.Err(setErr))
	}
	return result.DHSaleID, nil
}

// voidDHSale reverses a recorded sale on DH. A 404 is success-with-a-log
// (design §7): it covers not-found, another account's sale, a DH
// marketplace-mirror deal, or a UI-created deal — none of which we can void
// and none of which should fail an un-sell the user already performed locally.
func (s *service) voidDHSale(ctx context.Context, purchaseID, dhSaleID string) error {
	if s.dhSaleRecorder == nil {
		return nil
	}
	err := s.dhSaleRecorder.VoidInventorySale(ctx, dhSaleID, "un-sell")
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrDHSaleNotFound) {
		if s.logger != nil {
			s.logger.Info(ctx, "un-sell: dh sale already gone, treating void as success",
				observability.String("purchaseID", purchaseID),
				observability.String("dhSaleID", dhSaleID))
		}
		return nil
	}
	return err
}
