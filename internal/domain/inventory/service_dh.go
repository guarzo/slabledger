package inventory

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ApproveDHPush clears the hold on a purchase and re-queues it for DH push.
func (s *service) ApproveDHPush(ctx context.Context, purchaseID string) error {
	p, err := s.purchases.GetPurchase(ctx, purchaseID)
	if err != nil {
		return err
	}
	if p.DHPushStatus != DHPushStatusHeld {
		return errors.NewAppError(ErrCodeCampaignValidation, "purchase is not in held status")
	}
	holdReasonCleared := p.DHHoldReason
	if err := s.purchases.ApproveHeldPurchase(ctx, purchaseID); err != nil {
		return err
	}
	if s.logger != nil {
		s.logger.Info(ctx, "dh push approved",
			observability.String("purchaseID", purchaseID),
			observability.String("previousStatus", DHPushStatusHeld),
			observability.String("newStatus", DHPushStatusPending),
			observability.String("holdReasonCleared", holdReasonCleared),
			observability.Time("timestamp", time.Now()),
		)
	}
	return nil
}

// GetDHPushConfig returns the current DH push safety configuration.
func (s *service) GetDHPushConfig(ctx context.Context) (*DHPushConfig, error) {
	return s.dh.GetDHPushConfig(ctx)
}

// SaveDHPushConfig persists the DH push safety configuration.
func (s *service) SaveDHPushConfig(ctx context.Context, cfg *DHPushConfig) error {
	if cfg == nil {
		return errors.NewAppError(ErrCodeCampaignValidation, "dh push config cannot be nil")
	}
	if cfg.SwingPctThreshold <= 0 || cfg.SwingMinCents <= 0 ||
		cfg.DisagreementPctThreshold <= 0 || cfg.UnreviewedChangePctThreshold <= 0 ||
		cfg.UnreviewedChangeMinCents <= 0 {
		return errors.NewAppError(ErrCodeCampaignValidation, "all threshold values must be positive")
	}
	return s.dh.SaveDHPushConfig(ctx, cfg)
}

// notifyDHSold retires a sold item on DH so it stops being offered there.
// Local dh_status alone is bookkeeping: until DH is told, the card stays live
// on their marketplace and can sell a second time. Best-effort by design — the
// sale is already committed locally, and a DH outage must not fail it. The
// dh-sold reconciler sweeps up anything missed here.
//
// Superseded by recordDHSale, which uses the purpose-built sale endpoint
// instead of this status-PATCH (DH rejects it with 422 "Invalid status
// 'sold'"). No longer called from CreateSale/CreateBulkSales — kept declared,
// along with dhSoldNotifier/WithDHSoldNotifier, until Task 12 deletes them.
func (s *service) notifyDHSold(ctx context.Context, op, purchaseID string, dhInventoryID int) {
	if s.dhSoldNotifier == nil || dhInventoryID == 0 {
		return
	}
	if err := s.dhSoldNotifier.MarkInventorySold(ctx, dhInventoryID); err != nil && s.logger != nil {
		s.logger.Warn(ctx, op+": failed to mark DH inventory as sold",
			observability.String("purchaseID", purchaseID),
			observability.Int("dhInventoryID", dhInventoryID),
			observability.Err(err))
	}
}

// buildDHSaleRequest builds the DH sale-record body from persisted columns
// only — never the wall clock (design §2 corollary) — so a retry with the
// same key issues a byte-identical body.
func (s *service) buildDHSaleRequest(sa *Sale, purchase *Purchase, key string) DHSaleRequest {
	return DHSaleRequest{
		DHInventoryID:  purchase.DHInventoryID,
		IdempotencyKey: key,
		SalePriceCents: sa.SalePriceCents,
		SoldAt:         DeriveDHSoldAt(sa.SaleDate, purchase.PurchaseDate, sa.CreatedAt),
	}
}

// recordDHSale records the sale on DH so the item is retired there, via the
// purpose-built sale endpoint rather than notifyDHSold's status-PATCH, which
// DH rejects (422 "Invalid status 'sold'. Must be one of: in_stock,
// listed"). Both are called during the migration; only this one survives
// Task 12's cleanup. Best-effort by design: the sale is already committed
// locally and a DH outage must not fail it.
//
// A retryable failure is left unflagged for the §5b recovery pass — the key is
// already persisted, so the next cycle's identical request IS the retry. Any
// other failure, or a success that leaves the item not delisted, is flagged on
// the purchase for human review. A 409 item_sold_on_channel is never turned
// into a synthesized sale row: DH supplies no price, and both sold_at and
// channel may be nil (design §4).
func (s *service) recordDHSale(ctx context.Context, op string, sa *Sale, purchase *Purchase) {
	if s.dhSaleRecorder == nil || purchase.DHInventoryID == 0 {
		return
	}

	req := s.buildDHSaleRequest(sa, purchase, sa.DHIdempotencyKey)
	result, err := s.dhSaleRecorder.RecordInventorySale(ctx, req)
	if err != nil {
		if IsRetryableDHSaleError(err) {
			if s.logger != nil {
				s.logger.Warn(ctx, op+": dh sale record retryable failure, deferring to recovery pass",
					observability.String("purchaseID", sa.PurchaseID),
					observability.Err(err))
			}
			return
		}
		s.flagDHSaleConflict(ctx, op, purchase.ID, err.Error())
		return
	}

	if setErr := s.sales.SetSaleDHSaleID(ctx, sa.ID, result.DHSaleID, time.Now()); setErr != nil && s.logger != nil {
		s.logger.Warn(ctx, op+": failed to persist dh_sale_id",
			observability.String("purchaseID", sa.PurchaseID),
			observability.String("dhSaleID", result.DHSaleID),
			observability.Err(setErr))
	}

	// delisted == false means an ask may still be live — the exact failure
	// mode of the 2026-08-15 incident — so it is surfaced, not assumed benign.
	if !result.Delisted {
		s.flagDHSaleConflict(ctx, op, purchase.ID, "dh sale recorded but item not delisted")
	}
}

// flagDHSaleConflict records a purchase-level conflict for human review.
// Best-effort: a failure to write the flag is logged, not propagated — the
// sale already committed and the DH call already resolved either way.
func (s *service) flagDHSaleConflict(ctx context.Context, op, purchaseID, reason string) {
	if err := s.purchases.SetDHSaleConflict(ctx, purchaseID, reason); err != nil && s.logger != nil {
		s.logger.Warn(ctx, op+": failed to flag dh sale conflict",
			observability.String("purchaseID", purchaseID),
			observability.Err(err))
	}
}
