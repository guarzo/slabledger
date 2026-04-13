package scoring

import (
	"math"
	"time"
)

// Score computes a ScoreCard from a ScoreRequest and WeightProfile.
// Returns ErrInsufficientData if fewer than MinFactors non-gap factors are available.
func Score(req ScoreRequest, profile WeightProfile) (ScoreCard, error) {
	if len(req.Factors) < MinFactors {
		return ScoreCard{}, &ErrInsufficientData{
			Available: len(req.Factors),
			Required:  MinFactors,
			Gaps:      req.DataGaps,
		}
	}

	composite := computeComposite(req.Factors, profile.Weights)
	confidence := computeConfidence(req.Factors, profile.Weights)
	mixed := detectMixedSignals(req.Factors)
	verdict := VerdictFromComposite(composite)

	return ScoreCard{
		EntityID:     req.EntityID,
		EntityType:   req.EntityType,
		Factors:      req.Factors,
		Composite:    composite,
		Confidence:   confidence,
		Verdict:      verdict,
		DataGaps:     req.DataGaps,
		MixedSignals: mixed,
		ScoredAt:     time.Now(),
	}, nil
}

// computeComposite calculates the weighted sum of factors, re-normalizing
// weights to account for missing factors.
func computeComposite(factors []Factor, weights []FactorWeight) float64 {
	factorMap := make(map[string]float64, len(factors))
	for _, f := range factors {
		factorMap[f.Name] = f.Value
	}

	var totalAvailableWeight float64
	for _, w := range weights {
		if _, ok := factorMap[w.Name]; ok {
			totalAvailableWeight += w.Weight
		}
	}
	if totalAvailableWeight == 0 {
		return 0
	}

	var composite float64
	for _, w := range weights {
		if val, ok := factorMap[w.Name]; ok {
			normalizedWeight := w.Weight / totalAvailableWeight
			composite += val * normalizedWeight
		}
	}

	return clamp(composite, -1.0, 1.0)
}

// computeConfidence calculates overall confidence from 4 components.
func computeConfidence(factors []Factor, weights []FactorWeight) float64 {
	totalExpected := len(weights)
	if totalExpected == 0 {
		return 0.2
	}

	n := float64(len(factors))
	if n == 0 {
		return clamp(0, 0.2, 0.95)
	}
	coverage := n / float64(totalExpected) * 0.3

	var sumAbs, sumConf float64
	var positive, negative int
	for _, f := range factors {
		sumAbs += math.Abs(f.Value)
		sumConf += f.Confidence
		if f.Value > 0.1 {
			positive++
		} else if f.Value < -0.1 {
			negative++
		}
	}

	strength := (sumAbs / n) * 0.3
	quality := (sumConf / n) * 0.2

	significant := positive + negative
	agreement := 0.0
	if significant > 0 {
		majority := max(positive, negative)
		agreement = float64(majority) / float64(significant) * 0.2
	}

	return clamp(coverage+strength+quality+agreement, 0.2, 0.95)
}

// detectMixedSignals returns true if factors are strongly split:
// 2+ factors at >0.5 bullish AND 2+ factors at <-0.5 bearish.
func detectMixedSignals(factors []Factor) bool {
	var strongBull, strongBear int
	for _, f := range factors {
		if f.Value > 0.5 {
			strongBull++
		} else if f.Value < -0.5 {
			strongBear++
		}
	}
	return strongBull >= 2 && strongBear >= 2
}

func clamp(v, lo, hi float64) float64 {
	return max(lo, min(hi, v))
}
