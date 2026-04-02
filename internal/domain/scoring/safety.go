package scoring

// ApplySafetyFilters applies confidence clamping and mixed signal guards to a ScoreCard.
func ApplySafetyFilters(sc ScoreCard) ScoreCard {
	sc.Verdict = clampByConfidence(sc.Confidence, sc.Verdict)
	if sc.MixedSignals {
		sc.Verdict = clampMixedSignals(sc.Verdict)
	}
	return sc
}

// clampByConfidence restricts verdict extremity based on model confidence:
//   - <0.3: low confidence → restrict to lean_sell..lean_buy (prevent strong calls on weak data)
//   - <0.5: medium confidence → restrict to sell..buy (allow moderate calls)
//   - >=0.5: high confidence → no restriction
func clampByConfidence(confidence float64, v Verdict) Verdict {
	switch {
	case confidence < 0.3:
		return clampVerdictRange(v, VerdictLeanSell, VerdictLeanBuy)
	case confidence < 0.5:
		return clampVerdictRange(v, VerdictSell, VerdictBuy)
	default:
		return v
	}
}

func clampMixedSignals(v Verdict) Verdict {
	return clampVerdictRange(v, VerdictLeanSell, VerdictLeanBuy)
}

func clampVerdictRange(v Verdict, lo, hi Verdict) Verdict {
	ord := verdictOrder[v]
	loOrd := verdictOrder[lo]
	hiOrd := verdictOrder[hi]
	if ord < loOrd {
		return lo
	}
	if ord > hiOrd {
		return hi
	}
	return v
}

// ValidateVerdictAdjustment checks that an LLM-adjusted verdict is at most one step from the engine verdict.
func ValidateVerdictAdjustment(engineVerdict, llmVerdict Verdict) bool {
	return VerdictDistance(engineVerdict, llmVerdict) <= 1
}
