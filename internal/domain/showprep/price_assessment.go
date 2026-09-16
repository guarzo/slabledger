package showprep

import (
	"cmp"
	"math/bits"
	"slices"
	"time"
)

// assessRecentPrice consumes validated, deduplicated sales. It does not decide
// source health or mutate the caller's wider history.
func assessRecentPrice(askingCents int, eligibleSales []Sale, now time.Time) (Status, string, RecentPriceEvidence) {
	d := now.UTC()
	recent := RecentPriceEvidence{
		WindowStart: d.AddDate(0, 0, -6).Format(time.DateOnly),
		WindowEnd:   d.Format(time.DateOnly),
		SaleIDs:     []string{},
	}
	selected := make([]Sale, 0, len(eligibleSales))
	for _, sale := range eligibleSales {
		if sale.Date >= recent.WindowStart && sale.Date <= recent.WindowEnd {
			selected = append(selected, sale)
		}
	}
	// IDs provide deterministic ordering, not an inferred intraday sequence.
	slices.SortFunc(selected, func(a, b Sale) int {
		if dateOrder := cmp.Compare(b.Date, a.Date); dateOrder != 0 {
			return dateOrder
		}
		return cmp.Compare(a.ID, b.ID)
	})
	if len(selected) > 5 {
		end := 5
		for end < len(selected) && selected[end].Date == selected[4].Date {
			end++
		}
		selected = selected[:end]
	}
	recent.Count = len(selected)
	prices := make([]int, 0, recent.Count)
	for i, sale := range selected {
		recent.SaleIDs = append(recent.SaleIDs, sale.ID)
		prices = append(prices, sale.PriceCents)
		if i == 0 {
			recent.LatestSaleDate = sale.Date
			recent.LatestSaleMinCents = sale.PriceCents
			recent.LatestSaleMaxCents = sale.PriceCents
		}
		if sale.Date == recent.LatestSaleDate {
			recent.LatestSaleCount++
			recent.LatestSaleMinCents = min(recent.LatestSaleMinCents, sale.PriceCents)
			recent.LatestSaleMaxCents = max(recent.LatestSaleMaxCents, sale.PriceCents)
		}
	}
	// Twice any positive int fits uint64, preserving half cents even at MaxInt.
	var twiceMedian uint64
	if recent.Count > 0 {
		slices.Sort(prices)
		middle := recent.Count / 2
		twiceMedian = 2 * uint64(prices[middle])
		if recent.Count%2 == 0 {
			twiceMedian = uint64(prices[middle-1]) + uint64(prices[middle])
		}
		recent.MedianCents = int((twiceMedian + 1) / 2)
		if askingCents > 0 {
			gap := 100 * (1 - float64(twiceMedian)/(2*float64(askingCents)))
			recent.GapPct = &gap
		}
	}
	switch {
	case askingCents <= 0:
		return NoListedPrice, "No positive SlabLedger asking price", recent
	case recent.Count == 0:
		return NoRecentComps, "No matching sales in the past seven UTC dates", recent
	case recent.Count == 1:
		return ThinEvidence, "Only one recent matching sale; review its amount", recent
	case priceProductLess(5, twiceMedian, 9, uint64(askingCents)):
		return BelowTarget, "Recent median below 90% of asking price", recent
	case priceProductLess(5, uint64(recent.LatestSaleMinCents), 4, uint64(askingCents)):
		return MixedEvidence, "A newest-day sale is more than 20% below asking", recent
	default:
		return Supported, "Recent matching sales support the asking price", recent
	}
}

// Compare exact scaled prices without overflowing or rounding the median.
func priceProductLess(a, b, c, d uint64) bool {
	leftHigh, leftLow := bits.Mul64(a, b)
	rightHigh, rightLow := bits.Mul64(c, d)
	return leftHigh < rightHigh || (leftHigh == rightHigh && leftLow < rightLow)
}
