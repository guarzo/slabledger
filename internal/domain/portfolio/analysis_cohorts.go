package portfolio

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// confidenceBucket returns the frozen CL confidence at purchase as a string
// bucket key, or "unknown" when no snapshot was captured.
func confidenceBucket(p inventory.Purchase) string {
	if p.CLConfidenceAtPurchase == nil {
		return "unknown"
	}
	return strconv.Itoa(*p.CLConfidenceAtPurchase)
}

// buyTermsBucket buckets buy cost as an integer percentage of the frozen
// CL-at-purchase snapshot, floored to avoid float boundary drift. Bands are
// lower-inclusive 5-point-wide bands between 50 and 100; below 50 and at/above
// 100 get their own catch-all buckets. Purchases without a CL snapshot are
// "unknown".
func buyTermsBucket(p inventory.Purchase) string {
	if p.CLValueAtPurchaseCents <= 0 {
		return "unknown"
	}
	pct := p.BuyCostCents * 100 / p.CLValueAtPurchaseCents
	if pct < 50 {
		return "<50"
	}
	if pct >= 100 {
		return ">=100"
	}
	lo := (pct / 5) * 5
	return fmt.Sprintf("%d-%d", lo, lo+5)
}

// cohortAccum is the mutable accumulator for one (confidence, buyTerms) cohort
// while scanning rows; converted to a ConfBuyCohortRow once scanning is done.
type cohortAccum struct {
	confidenceBucket string
	buyTermsBucket   string

	n         int
	soldCount int
	revenue   int
	netProfit int

	sumSourceCount     int
	sumActiveListings  int
	sumSalesLast30d    int
	sumPopulationAtBuy int

	coverageSourceCount int
	coverageMarket      int
	coveragePopulation  int
}

// computeConfidenceBuyCohorts groups purchases by frozen (CL confidence at
// purchase) x (buy cost as % of CL at purchase) and aggregates realized P&L
// plus provenance averages. Nil provenance pointers are skipped from their
// average (never treated as 0); the matching coverage counter tracks how many
// rows contributed. Output is sorted deterministically by ConfidenceBucket
// then BuyTermsBucket, with "unknown" sorted last in each dimension.
func computeConfidenceBuyCohorts(rows []inventory.PurchaseWithSale) []ConfBuyCohortRow {
	byKey := make(map[[2]string]*cohortAccum)

	for _, r := range rows {
		cb := confidenceBucket(r.Purchase)
		bb := buyTermsBucket(r.Purchase)
		key := [2]string{cb, bb}
		acc := byKey[key]
		if acc == nil {
			acc = &cohortAccum{confidenceBucket: cb, buyTermsBucket: bb}
			byKey[key] = acc
		}
		acc.n++

		if r.Purchase.SourceCountAtPurchase != nil {
			acc.sumSourceCount += *r.Purchase.SourceCountAtPurchase
			acc.coverageSourceCount++
		}
		if r.Purchase.ActiveListingsAtPurchase != nil && r.Purchase.SalesLast30dAtPurchase != nil {
			acc.sumActiveListings += *r.Purchase.ActiveListingsAtPurchase
			acc.sumSalesLast30d += *r.Purchase.SalesLast30dAtPurchase
			acc.coverageMarket++
		}
		if r.Purchase.PopulationAtPurchase != nil {
			acc.sumPopulationAtBuy += *r.Purchase.PopulationAtPurchase
			acc.coveragePopulation++
		}

		if r.Sale == nil {
			continue
		}
		acc.soldCount++
		acc.revenue += r.Sale.SalePriceCents
		acc.netProfit += r.Sale.NetProfitCents
	}

	rowsOut := make([]ConfBuyCohortRow, 0, len(byKey))
	for _, acc := range byKey {
		row := ConfBuyCohortRow{
			ConfidenceBucket:    acc.confidenceBucket,
			BuyTermsBucket:      acc.buyTermsBucket,
			N:                   acc.n,
			SoldCount:           acc.soldCount,
			RevenueCents:        acc.revenue,
			NetProfitCents:      acc.netProfit,
			ROIPct:              roiPct(acc.revenue, acc.netProfit),
			CoverageSourceCount: acc.coverageSourceCount,
			CoverageMarket:      acc.coverageMarket,
			CoveragePopulation:  acc.coveragePopulation,
		}
		if acc.coverageSourceCount > 0 {
			row.AvgSourceCount = float64(acc.sumSourceCount) / float64(acc.coverageSourceCount)
		}
		if acc.coverageMarket > 0 {
			row.AvgActiveListings = float64(acc.sumActiveListings) / float64(acc.coverageMarket)
			row.AvgSalesLast30d = float64(acc.sumSalesLast30d) / float64(acc.coverageMarket)
		}
		if acc.coveragePopulation > 0 {
			row.AvgPopulationAtBuy = float64(acc.sumPopulationAtBuy) / float64(acc.coveragePopulation)
		}
		rowsOut = append(rowsOut, row)
	}

	sort.Slice(rowsOut, func(i, j int) bool {
		a, b := rowsOut[i], rowsOut[j]
		if a.ConfidenceBucket != b.ConfidenceBucket {
			return confidenceBucketLess(a.ConfidenceBucket, b.ConfidenceBucket)
		}
		return buyTermsBucketLess(a.BuyTermsBucket, b.BuyTermsBucket)
	})

	return rowsOut
}

// confidenceBucketLess orders confidence buckets numerically ("2" < "10"),
// with "unknown" always last.
func confidenceBucketLess(a, b string) bool {
	if a == "unknown" {
		return false
	}
	if b == "unknown" {
		return true
	}
	ai, aErr := strconv.Atoi(a)
	bi, bErr := strconv.Atoi(b)
	if aErr == nil && bErr == nil {
		return ai < bi
	}
	return a < b
}

// buyTermsBucketRank gives each buy-terms bucket a sort key: "<50" first,
// then the 5-point bands by their lower bound, then ">=100", then "unknown"
// last.
func buyTermsBucketRank(s string) int {
	switch s {
	case "unknown":
		return 1000
	case "<50":
		return -1
	case ">=100":
		return 100
	default:
		// "NN-MM" — rank by the lower bound.
		var lo int
		if _, err := fmt.Sscanf(s, "%d-", &lo); err == nil {
			return lo
		}
		return 999 // unrecognized format sorts just before "unknown"
	}
}

func buyTermsBucketLess(a, b string) bool {
	return buyTermsBucketRank(a) < buyTermsBucketRank(b)
}
