package scoring

// ApplySafetyFilters applies confidence clamping and mixed signal guards to a ScoreCard.
func ApplySafetyFilters(sc ScoreCard) ScoreCard {
	sc.Verdict = clampByConfidence(sc.Confidence, sc.Verdict)
	if sc.MixedSignals {
		sc.Verdict = clampMixedSignals(sc.Verdict)
	}
	return sc
}

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
