package scheduler

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

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
// Returns (0, nil) on a resolved-but-no-value card (caller tags no_value) and
// (0, err) on a transient API failure (caller tags api_error).
func (s *CardLadderRefreshScheduler) fetchCLEstimate(ctx context.Context, client *cardladder.Client, gemRateID, displayCondition, description string) (float64, error) {
	if client == nil || gemRateID == "" || displayCondition == "" {
		return 0, nil
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
		return 0, err
	}
	if resp == nil {
		return 0, nil
	}
	return resp.EstimatedValue, nil
}
