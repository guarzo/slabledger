package psaexchange

import "math"

// Tier policy thresholds — v1 hardcoded; will move to config in v2.
const (
	highLiquidityVelocity   = 5
	highLiquidityConfidence = 5
	highLiquidityOfferPct   = 0.75
	defaultOfferPct         = 0.65
)

// SelectTier returns the offer tier for a listing given its velocity and confidence.
func SelectTier(velocityMonth, confidence int) Tier {
	if velocityMonth >= highLiquidityVelocity && confidence >= highLiquidityConfidence {
		return Tier{Name: "high_liquidity", MaxOfferPct: highLiquidityOfferPct}
	}
	return Tier{Name: "default", MaxOfferPct: defaultOfferPct}
}

// ScoreInputs captures the inputs to ScoreListing.
type ScoreInputs struct {
	ListPriceCents int64
	CompCents      int64
	VelocityMonth  int
	Confidence     int
}

// ScoreOutputs captures the computed score fields written onto a Listing.
type ScoreOutputs struct {
	Tier             Tier
	TargetOfferCents int64
	MaxOfferPct      float64
	EdgeAtOffer      float64
	Score            float64
	ListRunwayPct    float64
	MayTakeAtList    bool
}

// ScoreListing computes the offer + score fields for a listing.
// Returns a zero-value ScoreOutputs (with default tier) when CompCents <= 0
// or when the rounded target offer would be <= 0, since we can't make any
// meaningful offer in either case.
func ScoreListing(in ScoreInputs) ScoreOutputs {
	tier := SelectTier(in.VelocityMonth, in.Confidence)
	if in.CompCents <= 0 {
		return ScoreOutputs{Tier: tier, MaxOfferPct: tier.MaxOfferPct}
	}
	// Round to nearest cent (rather than truncating) for sub-cent accuracy.
	// Guard target <= 0: a sub-cent comp paired with a low offer pct would
	// otherwise produce edge = +Inf and corrupt sort order.
	target := int64(math.Round(float64(in.CompCents) * tier.MaxOfferPct))
	if target <= 0 {
		return ScoreOutputs{Tier: tier, MaxOfferPct: tier.MaxOfferPct}
	}
	edge := float64(in.CompCents-target) / float64(target)
	// Clamp velocity at 0 to avoid NaN from log(<=0) if upstream ever
	// returns a negative count. Velocity = 0 yields velocityScore = 0,
	// which is the correct "no-movement" weight.
	velocityScore := math.Log(1.0 + float64(max(in.VelocityMonth, 0)))
	score := edge * velocityScore

	out := ScoreOutputs{
		Tier:             tier,
		TargetOfferCents: target,
		MaxOfferPct:      tier.MaxOfferPct,
		EdgeAtOffer:      edge,
		Score:            score,
		MayTakeAtList:    in.ListPriceCents > 0 && in.ListPriceCents <= target,
	}
	if in.ListPriceCents > 0 {
		out.ListRunwayPct = float64(in.ListPriceCents-target) / float64(in.ListPriceCents)
	}
	return out
}
