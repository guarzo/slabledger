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
// The reset must happen whenever the purchase needs it, independent of
// whether a DH sale was ever recorded: a sale with no dh_sale_id and no
// dh_idempotency_key -- every sale row that predates this feature, plus any
// eBay/Shopify import with no DH history -- still needs its DH-linked
// purchase reset if that purchase reads as sold, or it never re-enrols in the
// push pipeline (this was the un-sell behavior before this feature and must
// not regress).
//
// A DH-side failure (recovering the handle, or the void itself) must not
// permanently block the local un-sell (design I2): the sale is already
// committed locally and that is what the user asked for. Such a failure is
// logged and flagged for human review, then the sequence falls through to the
// local reset+delete as if there had been nothing to void.
func (s *service) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	sale, err := s.sales.GetSaleByPurchaseID(ctx, purchaseID)
	if err != nil {
		return fmt.Errorf("delete sale for purchase %s: %w", purchaseID, err)
	}

	p, err := s.purchases.GetPurchase(ctx, purchaseID)
	if err != nil {
		return fmt.Errorf("delete sale for purchase %s: load purchase: %w", purchaseID, err)
	}

	dhSaleID := ""
	if sale.DHSaleID != "" || sale.DHIdempotencyKey != "" {
		dhSaleID, err = s.ensureDHSaleHandle(ctx, sale, p)
		if err != nil {
			s.flagDHUnsellFailure(ctx, p, "recover dh sale handle", err)
			dhSaleID = ""
		}
	}

	switch {
	case dhSaleID != "":
		if err := s.voidDHSale(ctx, purchaseID, dhSaleID); err != nil {
			s.flagDHUnsellFailure(ctx, p, "void on dh", err)
		}
		if err := s.purchases.ResetDHFieldsForRelistAfterVoid(ctx, purchaseID); err != nil {
			// The sale row is still here (we delete last), so dh_sale_id
			// survives and a retry re-runs void — idempotent, DH returns
			// reversed:false — then reset, harmlessly.
			return fmt.Errorf("delete sale for purchase %s: reset dh fields after void: %w", purchaseID, err)
		}
	case p.DHInventoryID != 0 && p.DHStatus == DHStatusSold:
		// No DH sale was ever recorded for this sale (legacy row predating
		// this feature, or the void handle could not be recovered above) but
		// the purchase is DH-linked and still reads as sold locally. Reset
		// anyway so it re-enrols in the push pipeline; there is nothing to
		// void on DH's side.
		if err := s.purchases.ResetDHFieldsForRelistAfterVoid(ctx, purchaseID); err != nil {
			return fmt.Errorf("delete sale for purchase %s: reset dh fields: %w", purchaseID, err)
		}
	}

	if err := s.sales.DeleteSaleByPurchaseID(ctx, purchaseID); err != nil {
		return fmt.Errorf("delete sale for purchase %s: %w", purchaseID, err)
	}
	return nil
}

// flagDHUnsellFailure logs a DH interaction failure encountered during
// un-sell and, unless the purchase already carries a conflict flag, raises
// one for human review. Never returns an error: the caller falls through to
// the local reset+delete regardless (design I2).
func (s *service) flagDHUnsellFailure(ctx context.Context, p *Purchase, step string, err error) {
	if s.logger != nil {
		s.logger.Warn(ctx, "un-sell: "+step+" failed, continuing with local reset",
			observability.String("purchaseID", p.ID),
			observability.Err(err))
	}
	if p.DHSaleConflict != "" {
		return
	}
	s.flagDHSaleConflict(ctx, "un-sell", p.ID, "un-sell "+step+" failed: "+err.Error())
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
