package portfolio

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// confidenceBucket returns the frozen CL policy-confidence minimum at
// purchase as a string bucket key, or "unknown" when no snapshot was
// captured. This reads the campaign's targeting policy floor, not the card's
// real CardLadder confidence -- see inventory.Purchase.CLCardConfidenceAtPurchase
// for that.
//
// It falls back to the pre-rename field because this is the read half of an
// expand/contract rename spanning three deploys (migration 000042). During the
// deploy-N rollout, instances of the previous binary are still writing rows
// that set only the old column; without this fallback every such row would
// bucket as "unknown" permanently, since nothing later revisits them. The
// fallback goes away with the field itself in SLA-106.
func confidenceBucket(p inventory.Purchase) string {
	v := p.CLPolicyConfidenceMinAtPurchase
	if v == nil {
		v = p.CLConfidenceAtPurchase
	}
	if v == nil {
		return "unknown"
	}
	return strconv.Itoa(*v)
}

// buyTermsBucketInfo pairs a buy-terms bucket label with the numeric rank
// used to sort it, so the rank never has to be recovered by re-parsing the
// label.
type buyTermsBucketInfo struct {
	label string
	rank  int
}

// buyTermsBucket buckets buy cost as an integer percentage of the frozen
// CL-at-purchase snapshot, floored to avoid float boundary drift. Bands are
// lower-inclusive 5-point-wide bands between 50 and 100; below 50 and at/above
// 100 get their own catch-all buckets. Purchases without a CL snapshot are
// "unknown".
func buyTermsBucket(p inventory.Purchase) buyTermsBucketInfo {
	if p.CLValueAtPurchaseCents <= 0 {
		return buyTermsBucketInfo{label: "unknown", rank: 1000}
	}
	pct := p.BuyCostCents * 100 / p.CLValueAtPurchaseCents
	if pct < 50 {
		return buyTermsBucketInfo{label: "<50", rank: -1}
	}
	if pct >= 100 {
		return buyTermsBucketInfo{label: ">=100", rank: 100}
	}
	lo := (pct / 5) * 5
	return buyTermsBucketInfo{label: fmt.Sprintf("%d-%d", lo, lo+5), rank: lo}
}

// cohortAccum is the mutable accumulator for one (confidence, buyTerms) cohort
// while scanning rows; converted to a ConfBuyCohortRow once scanning is done.
type cohortAccum struct {
	confidenceBucket string
	buyTermsBucket   string
	buyTermsRank     int

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
		key := [2]string{cb, bb.label}
		acc := byKey[key]
		if acc == nil {
			acc = &cohortAccum{confidenceBucket: cb, buyTermsBucket: bb.label, buyTermsRank: bb.rank}
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

	accs := make([]*cohortAccum, 0, len(byKey))
	for _, acc := range byKey {
		accs = append(accs, acc)
	}

	sort.Slice(accs, func(i, j int) bool {
		a, b := accs[i], accs[j]
		if a.confidenceBucket != b.confidenceBucket {
			return confidenceBucketLess(a.confidenceBucket, b.confidenceBucket)
		}
		return a.buyTermsRank < b.buyTermsRank
	})

	rowsOut := make([]ConfBuyCohortRow, 0, len(accs))
	for _, acc := range accs {
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
