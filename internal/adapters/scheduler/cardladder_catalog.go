package scheduler

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// clEstimate is CardLadder's answer for a single (gemRateID, condition) pair:
// the estimated value plus the per-card comp confidence CardLadder reports
// alongside it. Threading both through (rather than just the value) is what
// lets analysis separate thin-comp cards from thick-comp ones — see
// cl_card_confidence_at_purchase.
type clEstimate struct {
	value      float64
	confidence int
}

// estimateKey identifies a (gemRateID, condition) pair for the batch cache in
// applyCLEstimates below.
type estimateKey struct{ gemRateID, condition string }

// fetchCLEstimate calls CL's CardEstimate Firebase callable (the same function
// powering the CL website's live card value) for a single
// (gemRateID, displayCondition) pair.
//
// Why not the `cards` search index? The gemRateIDs returned by
// BuildCollectionCard are not reliably present in the public `cards` index —
// especially for newer sets — so filtering that index produced 0-hit results
// for most of our inventory even though CL has valuations for those cards.
// CardEstimate reads the canonical valuation store directly and returns
// non-zero values for the same IDs the index misses.
//
// Returns (clEstimate{}, nil) on a resolved-but-no-value card (caller tags
// no_value) and (clEstimate{}, err) on a transient API failure (caller tags
// api_error).
func (s *CardLadderRefreshScheduler) fetchCLEstimate(ctx context.Context, client *cardladder.Client, gemRateID, displayCondition, description string) (clEstimate, error) {
	if client == nil || gemRateID == "" || displayCondition == "" {
		return clEstimate{}, nil
	}
	resp, err := client.CardEstimate(ctx, cardladder.CardEstimateRequest{
		GemRateID:      gemRateID,
		GradingCompany: "psa",
		Condition:      firestoreConditionFor(displayCondition),
		Description:    description,
	})
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: card estimate failed",
			observability.String("gemRateId", gemRateID),
			observability.String("condition", displayCondition),
			observability.Err(err))
		return clEstimate{}, err
	}
	if resp == nil {
		return clEstimate{}, nil
	}
	return clEstimate{value: resp.EstimatedValue, confidence: resp.Confidence}, nil
}

// clResolvedPurchase is a purchase paired with the (gemRateID, condition)
// Phase 1 of runOnce resolved for it. Package-level (rather than local to
// runOnce) so applyCLEstimates below — and its unit tests — can construct it
// directly without a real Postgres-backed CardLadderStore.
type clResolvedPurchase struct {
	purchase  *inventory.Purchase
	gemRateID string
	condition string
}

// applyCLEstimates is runOnce's Phase 2+3: batch-fetch the live CardEstimate
// for each resolved purchase and apply it. Successful and no-value estimates
// are cached per (gemRateID, condition) so multiple physical copies of the
// same card cost a single callable — the dominant fix for quota burn, since
// the index doesn't reliably surface these gemRateIDs and we hit the
// canonical estimate function directly. (A hard error is not cached, so a
// persistently-failing card still retries once per copy — acceptable, and
// lets transient errors recover.) When CL's daily quota is hit, stop making
// further estimate calls for this cycle (they would all fail identically)
// but keep applying values already fetched this run.
//
// Extracted out of runOnce (which needs a live CardLadderStore to reach this
// point) so the cache-shape regression this exists to prevent — a
// float-keyed cache silently zeroing every confidence after the first cache
// hit — is unit-testable without a database.
func (s *CardLadderRefreshScheduler) applyCLEstimates(ctx context.Context, client *cardladder.Client, resolvedPurchases []clResolvedPurchase, stats *CLRunStats) error {
	estimates := make(map[estimateKey]clEstimate, len(resolvedPurchases))
	for _, r := range resolvedPurchases {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := estimateKey{r.gemRateID, r.condition}
		est, cached := estimates[key]
		if !cached {
			if stats.QuotaExhausted {
				// Quota wall already hit this cycle — don't burn more calls.
				// Tagged quota_exhausted (not api_error) so the admin /failures
				// view doesn't read these as genuine CL failures: they were
				// never attempted.
				stats.SkippedQuota++
				s.recordCLError(ctx, r.purchase.ID, CLReasonQuotaExhausted)
				continue
			}
			var err error
			est, err = s.fetchCLEstimate(ctx, client, r.gemRateID, r.condition, r.purchase.CardName)
			if err != nil {
				if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit) {
					stats.QuotaExhausted = true
					stats.SkippedQuota++
					s.recordCLError(ctx, r.purchase.ID, CLReasonQuotaExhausted)
					s.logger.Warn(ctx, "CL refresh: daily quota exhausted, skipping remaining estimates this cycle",
						observability.Int("updatedSoFar", stats.Updated))
					continue
				}
				stats.EstimateFailed++
				s.recordCLError(ctx, r.purchase.ID, CLReasonAPIError)
				continue
			}
			estimates[key] = est
		}
		if est.value <= 0 {
			stats.NoValue++
			s.recordCLError(ctx, r.purchase.ID, CLReasonNoValue)
			continue
		}
		newCLCents := mathutil.ToCentsInt(est.value)
		oldCLCents := r.purchase.CLValueCents
		conf := est.confidence
		if err := s.valueUpdater.UpdatePurchaseCLValue(ctx, r.purchase.ID, newCLCents, r.purchase.Population, &conf); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to update CL value",
				observability.String("cert", r.purchase.CertNumber),
				observability.Err(err))
			continue
		}
		r.purchase.CLValueCents = newCLCents
		stats.Updated++
		s.recordCLError(ctx, r.purchase.ID, "")

		// Re-enroll for DH push when market value changes. Two qualifying cases:
		//  1. Already-pushed rows (DHInventoryID != 0) — so DH picks up the new price.
		//  2. Received-but-unmatched rows — so a fresh cert resolve is attempted
		//     with the new market value, which may push it above a price floor.
		if s.dhPushUpdater != nil && newCLCents != oldCLCents && shouldReenrollForCLChange(r.purchase) {
			if err := s.dhPushUpdater.UpdatePurchaseDHPushStatus(ctx, r.purchase.ID, inventory.DHPushStatusPending); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to re-enroll for DH push",
					observability.String("cert", r.purchase.CertNumber),
					observability.Err(err))
			} else {
				r.purchase.DHPushStatus = inventory.DHPushStatusPending
				s.recordEvent(ctx, dhevents.Event{
					PurchaseID:    r.purchase.ID,
					CertNumber:    r.purchase.CertNumber,
					Type:          dhevents.TypeEnrolled,
					NewPushStatus: inventory.DHPushStatusPending,
					Source:        dhevents.SourceCLRefresh,
				})
			}
		}
	}
	return nil
}
