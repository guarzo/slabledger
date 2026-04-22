package postgres

import (
	"sort"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// batchPriceRow holds per-row price data for batch comp queries.
type batchPriceRow struct {
	priceCents int
	saleDate   string
	platform   string
}

// medianInt returns the median of an int slice, or 0 if empty.
// Sorts the input in place.
func medianInt(vals []int) int {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sort.Ints(vals)
	if n%2 == 1 {
		return vals[n/2]
	}
	return (vals[n/2-1] + vals[n/2]) / 2
}

// computeTrend compares median of earlier half vs recent half of the 90-day window.
// midCutoff splits the window: dates < midCutoff are "earlier", >= midCutoff are "recent".
func computeTrend(prices []int, dates []string, midCutoff string) float64 {
	var earlier, recent []int
	for i, d := range dates {
		if d < midCutoff {
			earlier = append(earlier, prices[i])
		} else {
			recent = append(recent, prices[i])
		}
	}
	if len(earlier) == 0 || len(recent) == 0 {
		return 0
	}
	medEarlier := medianInt(earlier)
	medRecent := medianInt(recent)
	if medEarlier == 0 {
		return 0
	}
	return float64(medRecent-medEarlier) / float64(medEarlier)
}

// platformBreakdownFromRows builds per-platform statistics from a slice of batchPriceRow.
func platformBreakdownFromRows(rows []batchPriceRow) []inventory.PlatformBreakdown {
	byPlat := make(map[string][]int)
	for _, r := range rows {
		byPlat[r.platform] = append(byPlat[r.platform], r.priceCents)
	}
	out := make([]inventory.PlatformBreakdown, 0, len(byPlat))
	for plat, prices := range byPlat {
		sorted := make([]int, len(prices))
		copy(sorted, prices)
		med := medianInt(sorted)
		high, low := prices[0], prices[0]
		for _, p := range prices {
			if p > high {
				high = p
			}
			if p < low {
				low = p
			}
		}
		out = append(out, inventory.PlatformBreakdown{
			Platform:    plat,
			SaleCount:   len(prices),
			MedianCents: med,
			HighCents:   high,
			LowCents:    low,
		})
	}
	return out
}
