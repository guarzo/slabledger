package campaigns

import (
	"context"
	"math"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	// clDeviationThreshold is the minimum abs(median - cl) / cl that triggers CL anchoring.
	// At 40%, single-source snapshots deviating this much from CL are replaced with CL-derived values.
	clDeviationThreshold = 0.40
)

// snapshotResult represents the outcome of a market snapshot capture attempt.
type snapshotResult int

const (
	snapshotSuccess       snapshotResult = iota // snapshot fetched and persisted
	snapshotSkipped                             // unprocessable input (missing name, low grade, generic set)
	snapshotProviderError                       // provider failed or returned nil
	snapshotDBError                             // snapshot fetched but DB write failed
	snapshotCancelled                           // context was cancelled during processing
)

// snapshotReceiver can receive market snapshot data (implemented by both Purchase and Sale).
type snapshotReceiver interface {
	applySnapshot(snapshot *MarketSnapshot, date string)
}

// applyMarketSnapshot copies snapshot data to a receiver if the snapshot is non-nil.
func applyMarketSnapshot(r snapshotReceiver, snapshot *MarketSnapshot) {
	if snapshot == nil {
		return
	}
	r.applySnapshot(snapshot, time.Now().Format("2006-01-02"))
}

// setCLAnchoredPrices overwrites the snapshot's median, percentile, and grade price fields
// with CL-derived values and marks the snapshot as CL-anchored.
func setCLAnchoredPrices(snapshot *MarketSnapshot, clValueCents int) {
	snapshot.MedianCents = clValueCents
	snapshot.GradePriceCents = clValueCents
	snapshot.ConservativeCents = int(math.Round(float64(clValueCents) * 0.85))
	snapshot.OptimisticCents = int(math.Round(float64(clValueCents) * 1.15))
	snapshot.P10Cents = int(math.Round(float64(clValueCents) * 0.70))
	snapshot.P90Cents = int(math.Round(float64(clValueCents) * 1.30))
	snapshot.PricingGap = false
	snapshot.IsEstimated = false
	snapshot.CLAnchorApplied = true
}

// applyCLCorrection adjusts snapshot values when the fusion pipeline produces unreliable results.
// It compares the snapshot median against the purchase's CL value and corrects when:
//   - The snapshot has a pricing gap (no median) and CL is available
//   - The market price is BELOW CL with high deviation and only 1 source (likely a variant mismatch)
//
// CL acts as a price floor: when a single source reports a price ABOVE CL, the market data
// is trusted (the card may genuinely be worth more than CL's estimate). Anchoring only
// applies downward to prevent underpricing.
//
// When multiple sources agree (sourceCount >= 2), the fusion result is trusted even if
// it diverges from CL — multi-source agreement is a stronger signal than CL alone.
func applyCLCorrection(snapshot *MarketSnapshot, clValueCents int) {
	if snapshot == nil || clValueCents <= 0 {
		return
	}
	snapshot.CLValueCents = clValueCents

	// Fill pricing gaps with CL-derived values.
	// Also covers the case where GradePriceCents > 0 but MedianCents == 0
	// (e.g. grade price from an adjacent grade with no direct sale data).
	if snapshot.MedianCents == 0 {
		// Record the pre-correction deviation (100% when median is zero).
		snapshot.CLDeviationPct = 1.0
		setCLAnchoredPrices(snapshot, clValueCents)
		return
	}

	// Compute deviation between snapshot median and CL.
	// MedianCents is guaranteed > 0 here (zero case returned above).
	deviation := math.Abs(float64(snapshot.MedianCents-clValueCents)) / float64(clValueCents)
	snapshot.CLDeviationPct = deviation

	// Only correct single-source results when market price is BELOW CL with high deviation.
	// When market is above CL, trust the market — the card may be worth more than CL thinks.
	// Multi-source fusion that diverges from CL is more likely correct (CL may be stale).
	if deviation > clDeviationThreshold && snapshot.SourceCount <= 1 && snapshot.MedianCents < clValueCents {
		setCLAnchoredPrices(snapshot, clValueCents)
	}
}

// captureMarketSnapshot performs a best-effort market snapshot lookup and applies it to the receiver.
// Skips lookup when the set name is generic (e.g. "TCG Cards") to avoid capturing wrong data.
func (s *service) captureMarketSnapshot(ctx context.Context, r snapshotReceiver, card CardIdentity, grade float64, clValueCents int) {
	if s.priceProv != nil && card.CardName != "" && grade > 0 && !isGenericSetName(card.SetName) {
		snapshot, err := s.priceProv.GetMarketSnapshot(ctx, card, grade)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "captureMarketSnapshot failed",
					observability.String("card", card.CardName),
					observability.String("set", card.SetName),
					observability.Float64("grade", grade),
					observability.Err(err))
			}
			return
		}
		applyCLCorrection(snapshot, clValueCents)
		applyMarketSnapshot(r, snapshot)
	}
}

// recaptureMarketSnapshot fetches a fresh market snapshot for the given card identity and persists it
// to the purchase. Used after card metadata is corrected (e.g. CL import backfill).
// Returns true only when a non-nil snapshot was fetched and persisted successfully.
func (s *service) recaptureMarketSnapshot(ctx context.Context, purchaseID string, card CardIdentity, grade float64, clValueCents int) bool {
	return s.recaptureMarketSnapshotDetailed(ctx, purchaseID, card, grade, clValueCents) == snapshotSuccess
}

// recaptureMarketSnapshotDetailed fetches a fresh market snapshot and returns a detailed result
// distinguishing skipped/unprocessable inputs, provider errors, and DB write failures.
func (s *service) recaptureMarketSnapshotDetailed(ctx context.Context, purchaseID string, card CardIdentity, grade float64, clValueCents int) snapshotResult {
	if s.priceProv == nil || card.CardName == "" || grade <= 0 || isGenericSetName(card.SetName) {
		return snapshotSkipped
	}
	snapshot, err := s.priceProv.GetMarketSnapshot(ctx, card, grade)
	if err != nil {
		if ctx.Err() != nil {
			return snapshotCancelled
		}
		if s.logger != nil {
			s.logger.Warn(ctx, "recaptureMarketSnapshot failed",
				observability.String("card", card.CardName),
				observability.String("set", card.SetName),
				observability.Float64("grade", grade),
				observability.Err(err))
		}
		return snapshotProviderError
	}
	if snapshot == nil {
		return snapshotProviderError
	}
	applyCLCorrection(snapshot, clValueCents)
	var data MarketSnapshotData
	data.applySnapshot(snapshot, time.Now().Format("2006-01-02"))
	if err := s.repo.UpdatePurchaseMarketSnapshot(ctx, purchaseID, data); err != nil {
		if s.logger != nil {
			s.logger.Debug(ctx, "UpdatePurchaseMarketSnapshot failed",
				observability.String("purchaseID", purchaseID),
				observability.Err(err))
		}
		return snapshotDBError
	}
	return snapshotSuccess
}

// RefreshPurchaseSnapshot fetches a fresh market snapshot and persists it to the purchase.
// Exported wrapper around recaptureMarketSnapshot for use by background schedulers.
func (s *service) RefreshPurchaseSnapshot(ctx context.Context, purchaseID string, card CardIdentity, grade float64, clValueCents int) bool {
	return s.recaptureMarketSnapshot(ctx, purchaseID, card, grade, clValueCents)
}

// ProcessPendingSnapshots fetches and persists market snapshots for purchases
// that were imported without one (snapshot_status = "pending").
func (s *service) ProcessPendingSnapshots(ctx context.Context, limit int) (processed, skipped, failed int) {
	return s.processSnapshotsByStatus(ctx, SnapshotStatusPending, limit)
}

// RetryFailedSnapshots retries market snapshot capture for purchases where
// a previous attempt failed (snapshot_status = "failed").
func (s *service) RetryFailedSnapshots(ctx context.Context, limit int) (processed, skipped, failed int) {
	return s.processSnapshotsByStatus(ctx, SnapshotStatusFailed, limit)
}

func (s *service) processSnapshotsByStatus(ctx context.Context, status SnapshotStatus, limit int) (processed, skipped, failed int) {
	purchases, err := s.repo.ListSnapshotPurchasesByStatus(ctx, status, limit)
	if err != nil {
		if s.logger != nil {
			s.logger.Error(ctx, "ListSnapshotPurchasesByStatus failed",
				observability.String("status", string(status)),
				observability.Err(err))
		}
		return 0, 0, 0
	}
	for _, p := range purchases {
		card := p.ToCardIdentity()
		result := s.recaptureMarketSnapshotDetailed(ctx, p.ID, card, p.GradeValue, p.CLValueCents)
		switch result {
		case snapshotCancelled:
			// Context was cancelled — stop processing and leave remaining purchases unchanged
			return processed, skipped, failed
		case snapshotSuccess:
			if err := s.repo.UpdatePurchaseSnapshotStatus(ctx, p.ID, SnapshotStatusNone, 0); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "failed to clear snapshot status",
					observability.String("purchaseID", p.ID),
					observability.Err(err))
			}
			// Snapshot data was persisted — status flag is stale but not a real failure
			processed++
		case snapshotSkipped:
			// Unprocessable (missing name, grade, or generic set) — mark exhausted, won't improve on retry
			if s.logger != nil {
				s.logger.Debug(ctx, "snapshot skipped: unprocessable purchase",
					observability.String("purchaseID", p.ID),
					observability.String("card", p.CardName),
					observability.String("set", p.SetName))
			}
			if err := s.repo.UpdatePurchaseSnapshotStatus(ctx, p.ID, SnapshotStatusExhausted, p.SnapshotRetryCount); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "failed to mark snapshot exhausted",
					observability.String("purchaseID", p.ID),
					observability.Err(err))
			}
			skipped++
		case snapshotProviderError, snapshotDBError:
			// Transient failure — increment retry count, mark failed (or exhausted if max reached)
			newCount := p.SnapshotRetryCount + 1
			newStatus := SnapshotStatusFailed
			if s.maxSnapshotRetries > 0 && newCount >= s.maxSnapshotRetries {
				newStatus = SnapshotStatusExhausted
			}
			if err := s.repo.UpdatePurchaseSnapshotStatus(ctx, p.ID, newStatus, newCount); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "failed to set failed snapshot status",
					observability.String("purchaseID", p.ID),
					observability.Err(err))
			}
			failed++
		}
	}
	return processed, skipped, failed
}

// needsSnapshotRecovery returns true when a purchase has valid card metadata
// but never had a market snapshot captured (e.g. provider was down during import).
func needsSnapshotRecovery(p *Purchase) bool {
	return p.SnapshotDate == "" && p.CardName != "" && p.GradeValue > 0 && !isGenericSetName(p.SetName)
}

// backfillMetadataFromCL updates card metadata (name, number, set) on the purchase
// from CL export data when the existing metadata is missing or generic.
// Each field is backfilled independently — existing good values are preserved.
// This is a DB-only operation with no external API calls.
func (s *service) backfillMetadataFromCL(ctx context.Context, purchase *Purchase, row CLExportRow) {
	// Determine which fields need backfill independently.
	nameNeeds := purchase.CardName == ""
	numberNeeds := purchase.CardNumber == ""
	setNeeds := isGenericSetName(purchase.SetName) && row.Set != ""

	if !nameNeeds && !numberNeeds && !setNeeds {
		return
	}

	// Compute candidate values, preserving existing good values.
	changed := false

	cardName := purchase.CardName
	if nameNeeds {
		if n := CLCardName(row); n != "" {
			cardName = n
			changed = true
		}
	}

	cardNumber := purchase.CardNumber
	if numberNeeds && row.Number != "" {
		cardNumber = row.Number
		changed = true
	}

	setName := purchase.SetName
	if setNeeds {
		setName = row.Set
		changed = true
	}

	if !changed {
		return
	}

	if err := s.repo.UpdatePurchaseCardMetadata(ctx, purchase.ID, cardName, cardNumber, setName); err != nil {
		if s.logger != nil {
			s.logger.Debug(ctx, "UpdatePurchaseCardMetadata failed", observability.String("purchaseID", purchase.ID), observability.Err(err))
		}
		return
	}

	// Sync in-memory purchase with the values written to DB.
	purchase.CardName = cardName
	purchase.CardNumber = cardNumber
	purchase.SetName = setName
}
