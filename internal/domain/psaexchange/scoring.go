package psaexchange

import "math"

// Policy captures the tunable levers for tier selection, target-offer
// computation, and the catalog filter thresholds. Defaults match the v1
// hardcoded values; callers can override per-deployment via env vars wired
// in cmd/slabledger.
type Policy struct {
	// Tier selection — a listing qualifies for the high-liquidity tier when
	// VelocityMonth >= HighLiquidityVelocity AND Confidence >= HighLiquidityConfidence.
	HighLiquidityVelocity   int
	HighLiquidityConfidence int

	// Max-offer percentages applied to comp to compute the target offer.
	HighLiquidityOfferPct float64
	DefaultOfferPct       float64

	// Catalog filter thresholds — listings below either are dropped.
	MinConfidence      int
	MinQuarterVelocity int
}

// DefaultPolicy returns the v1 hardcoded policy.
func DefaultPolicy() Policy {
	return Policy{
		HighLiquidityVelocity:   5,
		HighLiquidityConfidence: 5,
		HighLiquidityOfferPct:   0.75,
		DefaultOfferPct:         0.65,
		MinConfidence:           3,
		MinQuarterVelocity:      1,
	}
}

// SelectTier returns the offer tier for a listing given its velocity and confidence.
func (p Policy) SelectTier(velocityMonth, confidence int) Tier {
	if velocityMonth >= p.HighLiquidityVelocity && confidence >= p.HighLiquidityConfidence {
		return Tier{Name: "high_liquidity", MaxOfferPct: p.HighLiquidityOfferPct}
	}
	return Tier{Name: "default", MaxOfferPct: p.DefaultOfferPct}
}

// SelectTier returns the offer tier under DefaultPolicy. Retained for tests
// and external callers that don't need to override policy.
func SelectTier(velocityMonth, confidence int) Tier {
	return DefaultPolicy().SelectTier(velocityMonth, confidence)
}

// ScoreInputs captures the inputs to Score / ScoreListing.
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

// Score computes the offer + score fields for a listing under this policy.
// Returns a zero-value ScoreOutputs (with default tier) when CompCents <= 0
// or when the rounded target offer would be <= 0, since we can't make any
// meaningful offer in either case.
func (p Policy) Score(in ScoreInputs) ScoreOutputs {
	tier := p.SelectTier(in.VelocityMonth, in.Confidence)
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

// ScoreListing scores under DefaultPolicy.
func ScoreListing(in ScoreInputs) ScoreOutputs {
	return DefaultPolicy().Score(in)
}
