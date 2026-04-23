package liquidation

import (
	"math"
	"sort"
	"time"
)

// ComputeCompPrice derives a price and confidence level from historical sale comps.
func ComputeCompPrice(comps []SaleComp, clValueCents int) CompPriceResult {
	if len(comps) == 0 {
		return CompPriceResult{ConfidenceLevel: ConfidenceNone}
	}

	now := time.Now()
	day90 := now.AddDate(0, 0, -90)
	day180 := now.AddDate(0, 0, -180)

	filtered := filterByDate(comps, day90)
	if len(filtered) < 3 {
		filtered = filterByDate(comps, day180)
	}
	if len(filtered) < 3 {
		filtered = make([]SaleComp, len(comps))
		copy(filtered, comps)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].PriceCents < filtered[j].PriceCents
	})

	trimmed := filtered
	if len(filtered) >= 5 {
		drop := max(1, len(filtered)/10)
		trimmed = filtered[drop : len(filtered)-drop]
	}

	mean := meanPriceCents(trimmed)

	// Most recent comp date from trimmed set
	mostRecent := ""
	for _, c := range filtered {
		if mostRecent == "" || c.SaleDate > mostRecent {
			mostRecent = c.SaleDate
		}
	}

	gapPct := 0.0
	if clValueCents > 0 {
		gapPct = math.Abs(float64(mean-clValueCents)) / float64(clValueCents) * 100
	}

	countLevel := confidenceByCount(len(filtered))
	recencyLevel := confidenceByRecency(mostRecent, now)

	// Base confidence is min of count and recency; gap ≥25% lowers by one notch.
	confidence := minConfidence(countLevel, recencyLevel)
	if gapPct >= 25 {
		confidence = lowerConfidence(confidence)
	}

	return CompPriceResult{
		CompPriceCents:     mean,
		CompCount:          len(filtered),
		MostRecentCompDate: mostRecent,
		ConfidenceLevel:    confidence,
		GapPct:             gapPct,
	}
}

func filterByDate(comps []SaleComp, since time.Time) []SaleComp {
	cutoff := since.Format("2006-01-02")
	var out []SaleComp
	for _, c := range comps {
		if c.SaleDate >= cutoff {
			out = append(out, c)
		}
	}
	return out
}

func meanPriceCents(comps []SaleComp) int {
	if len(comps) == 0 {
		return 0
	}
	sum := 0
	for _, c := range comps {
		sum += c.PriceCents
	}
	return sum / len(comps)
}

func confidenceByCount(n int) ConfidenceLevel {
	switch {
	case n >= 10:
		return ConfidenceHigh
	case n >= 5:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

func confidenceByRecency(mostRecent string, now time.Time) ConfidenceLevel {
	if mostRecent == "" {
		return ConfidenceLow
	}
	t, err := time.Parse("2006-01-02", mostRecent)
	if err != nil {
		return ConfidenceLow
	}
	days := now.Sub(t).Hours() / 24
	switch {
	case days <= 30:
		return ConfidenceHigh
	case days <= 90:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

func lowerConfidence(c ConfidenceLevel) ConfidenceLevel {
	switch c {
	case ConfidenceHigh:
		return ConfidenceMedium
	case ConfidenceMedium:
		return ConfidenceLow
	default:
		return ConfidenceLow
	}
}

func minConfidence(levels ...ConfidenceLevel) ConfidenceLevel {
	order := map[ConfidenceLevel]int{
		ConfidenceNone:   0,
		ConfidenceLow:    1,
		ConfidenceMedium: 2,
		ConfidenceHigh:   3,
	}
	min := ConfidenceHigh
	for _, l := range levels {
		if order[l] < order[min] {
			min = l
		}
	}
	return min
}
