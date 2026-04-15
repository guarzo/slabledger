package demand

// DefaultSaturationThreshold is the active-listing count above which a bucket
// is considered saturated. Chosen as a Phase 1 heuristic; Phase 2 will replace
// this with a calibrated value derived from historical sell-through.
const DefaultSaturationThreshold = 100

// OpportunityScore computes the Phase 1 niche-opportunity heuristic from its
// components. Phase 2 will replace this with a calibrated formula; the
// function signature is the seam.
//
// velocityChangePct is expected in DH's wire format — percentage points
// (e.g. 15.2 means +15.2%). It is normalized to a fraction (0.152) and
// clamped to ±0.5 before being folded into the bonus.
//
// Formula:
//
//	score = demand_score
//	      × velocity_bonus      (1 + clamp(velocityChangePct / 100, -0.5, +0.5); nil treated as 0)
//	      × saturation_penalty  (0.5 if active_listing_count > threshold, else 1.0)
//	      × coverage_penalty    (1.0 if uncovered, 0.3 if at least one campaign covers)
//
// The result is not clamped — Phase 2 ranks are relative, so an unbounded
// score keeps the ordering information intact.
func OpportunityScore(demandScore float64, velocityChangePct *float64, activeListingCount int, coverage NicheCoverage) float64 {
	velocityBonus := 1.0
	if velocityChangePct != nil {
		v := *velocityChangePct / 100.0 // DH returns percentage points; normalize to fraction
		if v < -0.5 {
			v = -0.5
		}
		if v > 0.5 {
			v = 0.5
		}
		velocityBonus = 1.0 + v
	}

	saturationPenalty := 1.0
	if activeListingCount > DefaultSaturationThreshold {
		saturationPenalty = 0.5
	}

	coveragePenalty := 1.0
	if coverage.Covered || len(coverage.ActiveCampaignIDs) > 0 {
		coveragePenalty = 0.3
	}

	return demandScore * velocityBonus * saturationPenalty * coveragePenalty
}
