package dhpricing

import (
	"context"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// service is the default implementation of Service.
type service struct {
	lookup   PurchaseLookup
	updater  DHPriceUpdater
	writer   DHPriceWriter
	resetter DHReconcileResetter
	logger   observability.Logger
	now      func() time.Time
}

// NewService wires the domain price-sync service.
func NewService(
	lookup PurchaseLookup,
	updater DHPriceUpdater,
	writer DHPriceWriter,
	resetter DHReconcileResetter,
	logger observability.Logger,
) Service {
	return &service{
		lookup:   lookup,
		updater:  updater,
		writer:   writer,
		resetter: resetter,
		logger:   logger,
		now:      time.Now,
	}
}

// resolveListingPrice is the one-line rule from dhlisting.ResolveListingPriceCents,
// inlined here to preserve the flat-siblings invariant.
func resolveListingPrice(p *inventory.Purchase) int { return p.ReviewedPriceCents }

func (s *service) SyncPurchasePrice(ctx context.Context, purchaseID string) SyncResult {
	res := SyncResult{PurchaseID: purchaseID}

	p, err := s.lookup.GetPurchase(ctx, purchaseID)
	if err != nil {
		res.Outcome = OutcomeError
		res.Err = err
		return res
	}

	if p.DHInventoryID == 0 {
		res.Outcome = OutcomeSkippedNoInventory
		return res
	}
	reviewed := resolveListingPrice(p)
	if reviewed <= 0 {
		res.Outcome = OutcomeSkippedZeroReviewed
		return res
	}
	if reviewed == p.DHListingPriceCents {
		res.Outcome = OutcomeSkippedNoDrift
		return res
	}

	res.OldListingCents = p.DHListingPriceCents

	status := string(p.DHStatus)
	newDHPrice, err := s.updater.UpdateInventoryStatus(ctx, p.DHInventoryID, status, reviewed)
	if err != nil {
		if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderNotFound) {
			s.logger.Warn(ctx, "dh price sync: stale inventory id — resetting",
				observability.String("purchaseID", p.ID),
				observability.Int("dhInventoryID", p.DHInventoryID),
				observability.Err(err))
			if resetErr := s.resetter.ResetDHFieldsForRepush(ctx, p.ID); resetErr != nil {
				s.logger.Warn(ctx, "dh price sync: reset failed",
					observability.String("purchaseID", p.ID),
					observability.Err(resetErr))
			}
			res.Outcome = OutcomeStaleInventoryID
			res.Err = err
			return res
		}
		s.logger.Warn(ctx, "dh price sync: PATCH failed",
			observability.String("purchaseID", p.ID),
			observability.Int("dhInventoryID", p.DHInventoryID),
			observability.Err(err))
		res.Outcome = OutcomeError
		res.Err = err
		return res
	}

	res.NewListingCents = newDHPrice

	if err := s.writer.UpdatePurchaseDHPriceSync(ctx, p.ID, newDHPrice, s.now()); err != nil {
		s.logger.Warn(ctx, "dh price sync: local persist failed (DH already patched)",
			observability.String("purchaseID", p.ID),
			observability.Err(err))
		res.Outcome = OutcomeError
		res.Err = err
		return res
	}

	s.logger.Info(ctx, "dh price sync: ok",
		observability.String("purchaseID", p.ID),
		observability.Int("oldCents", res.OldListingCents),
		observability.Int("newCents", res.NewListingCents))
	res.Outcome = OutcomeSynced
	return res
}

func (s *service) SyncDriftedPurchases(ctx context.Context) SyncBatchResult {
	result := SyncBatchResult{ByOutcome: map[Outcome]int{}}

	drift, err := s.lookup.ListDHPriceDrift(ctx)
	if err != nil {
		s.logger.Warn(ctx, "dh price sync: list drift failed", observability.Err(err))
		return result
	}

	for i := range drift {
		r := s.SyncPurchasePrice(ctx, drift[i].ID)
		result.Total++
		result.ByOutcome[r.Outcome]++
	}

	s.logger.Info(ctx, "dh price sync: batch done",
		observability.Int("total", result.Total),
		observability.Int("synced", result.ByOutcome[OutcomeSynced]),
		observability.Int("errors", result.ByOutcome[OutcomeError]),
		observability.Int("stale", result.ByOutcome[OutcomeStaleInventoryID]))
	return result
}
