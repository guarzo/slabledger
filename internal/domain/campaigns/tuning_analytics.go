package campaigns

import (
	"fmt"
	"math"
	"sort"
)

// computePriceTierPerformance computes P&L segmented by cost basis tiers.
// Returns two views: fixed dollar buckets and campaign-relative quartiles.
func computePriceTierPerformance(data []PurchaseWithSale) (fixed []PriceTierPerformance, relative []PriceTierPerformance) {
	fixed = computeTiersForBuckets(data, fixedTierBuckets())
	relative = computeTiersForBuckets(data, quartileBuckets(data))
	return fixed, relative
}

func fixedTierBuckets() []tierBucket {
	buckets := make([]tierBucket, len(fixedTiers))
	for i, t := range fixedTiers {
		buckets[i] = tierBucket{Label: t.Label, MinCents: t.MinCents, MaxCents: t.MaxCents}
	}
	return buckets
}

func quartileBuckets(data []PurchaseWithSale) []tierBucket {
	if len(data) == 0 {
		return nil
	}
	costs := make([]int, len(data))
	for i, d := range data {
		costs[i] = d.Purchase.BuyCostCents
	}
	sort.Ints(costs)

	q1 := percentileInt(costs, 25)
	q2 := percentileInt(costs, 50)
	q3 := percentileInt(costs, 75)

	return []tierBucket{
		{Label: fmt.Sprintf("Q1 ($%d-$%d)", costs[0]/100, q1/100), MinCents: 0, MaxCents: q1},
		{Label: fmt.Sprintf("Q2 ($%d-$%d)", q1/100, q2/100), MinCents: q1, MaxCents: q2},
		{Label: fmt.Sprintf("Q3 ($%d-$%d)", q2/100, q3/100), MinCents: q2, MaxCents: q3},
		{Label: fmt.Sprintf("Q4 ($%d+)", q3/100), MinCents: q3, MaxCents: math.MaxInt},
	}
}

type tierBucket struct {
	Label    string
	MinCents int
	MaxCents int
}

func computeTiersForBuckets(data []PurchaseWithSale, buckets []tierBucket) []PriceTierPerformance {
	result := make([]PriceTierPerformance, len(buckets))
	for i, b := range buckets {
		result[i] = PriceTierPerformance{
			TierLabel:    b.Label,
			TierMinCents: b.MinCents,
			TierMaxCents: b.MaxCents,
		}
	}

	for _, d := range data {
		cost := d.Purchase.BuyCostCents
		for i, b := range buckets {
			if cost >= b.MinCents && cost < b.MaxCents {
				accumulateTier(&result[i], d)
				break
			}
		}
	}

	for i := range result {
		finalizeTierMetrics(&result[i])
	}
	return result
}

func accumulateTier(t *PriceTierPerformance, d PurchaseWithSale) {
	t.PurchaseCount++
	spend := d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
	t.TotalSpendCents += spend
	if d.Purchase.CLValueCents > 0 {
		t.AvgBuyPctOfCL += float64(d.Purchase.BuyCostCents) / float64(d.Purchase.CLValueCents)
	}
	if d.Sale != nil {
		t.SoldCount++
		t.TotalRevenueCents += d.Sale.SalePriceCents
		t.TotalFeesCents += d.Sale.SaleFeeCents
		t.NetProfitCents += d.Sale.NetProfitCents
		t.AvgDaysToSell += float64(d.Sale.DaysToSell)
	}
}

func finalizeTierMetrics(t *PriceTierPerformance) {
	if t.PurchaseCount > 0 {
		t.SellThroughPct = float64(t.SoldCount) / float64(t.PurchaseCount)
		t.AvgBuyPctOfCL /= float64(t.PurchaseCount)
	}
	if t.SoldCount > 0 {
		t.AvgDaysToSell /= float64(t.SoldCount)
	}
	if t.TotalSpendCents > 0 {
		t.ROI = float64(t.NetProfitCents) / float64(t.TotalSpendCents)
	}
}

// computeCardPerformance ranks cards by realized/unrealized P&L.
// Returns top N and bottom N performers.
func computeCardPerformance(data []PurchaseWithSale, limit int) (top []CardPerformance, bottom []CardPerformance) {
	if len(data) == 0 {
		return nil, nil
	}

	cards := make([]CardPerformance, 0, len(data))
	for _, d := range data {
		cp := CardPerformance{
			Purchase: d.Purchase,
			Sale:     d.Sale,
		}
		if d.Purchase.CLValueCents > 0 {
			cp.BuyPctOfCL = float64(d.Purchase.BuyCostCents) / float64(d.Purchase.CLValueCents)
		}
		costBasis := d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		if d.Sale != nil {
			cp.RealizedPnL = d.Sale.NetProfitCents
		} else {
			// Unrealized: use purchase snapshot median if available
			if d.Purchase.MedianCents > 0 {
				cp.UnrealizedPnL = d.Purchase.MedianCents - costBasis
			}
		}
		cards = append(cards, cp)
	}

	// Sort by effective P&L (realized for sold, unrealized for unsold)
	sort.Slice(cards, func(i, j int) bool {
		return effectivePnL(cards[i]) > effectivePnL(cards[j])
	})

	if limit > len(cards) {
		limit = len(cards)
	}
	top = cards[:limit]

	// Bottom: take from end, but start no earlier than index `limit` to
	// prevent overlap with the top slice. When len(cards) < 2*limit the
	// bottom slice will be smaller than limit (or empty).
	bottomStart := len(cards) - limit
	bottomStart = max(bottomStart, limit)
	if bottomStart < len(cards) {
		bottom = cards[bottomStart:]
	}

	return top, bottom
}

func effectivePnL(cp CardPerformance) int {
	if cp.Sale != nil {
		return cp.RealizedPnL
	}
	return cp.UnrealizedPnL
}

// computeBuyThresholdAnalysis computes the empirical optimal BuyTermsCLPct.
func computeBuyThresholdAnalysis(data []PurchaseWithSale, currentPct float64) *BuyThresholdAnalysis {
	// Filter to purchases with valid CL values
	var points []BuyThresholdDataPoint
	for _, d := range data {
		if d.Purchase.CLValueCents <= 0 {
			continue
		}
		pctOfCL := float64(d.Purchase.BuyCostCents) / float64(d.Purchase.CLValueCents)
		costBasis := d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents

		dp := BuyThresholdDataPoint{
			PurchaseID: d.Purchase.ID,
			BuyPctOfCL: pctOfCL,
			CostBasis:  costBasis,
		}

		if d.Sale != nil {
			dp.Sold = true
			dp.ProfitCents = d.Sale.NetProfitCents
			if costBasis > 0 {
				dp.ROI = float64(d.Sale.NetProfitCents) / float64(costBasis)
			}
		} else if d.Purchase.MedianCents > 0 {
			dp.ProfitCents = d.Purchase.MedianCents - costBasis
			if costBasis > 0 {
				dp.ROI = float64(dp.ProfitCents) / float64(costBasis)
			}
		}

		points = append(points, dp)
	}

	if len(points) == 0 {
		return nil
	}

	// Create 5% buckets from 50% to 100%
	var buckets []ThresholdBucket
	for pct := 0.50; pct < 1.0; pct += 0.05 {
		maxPct := pct + 0.05
		label := fmt.Sprintf("%.0f-%.0f%%", pct*100, maxPct*100)
		bucket := ThresholdBucket{
			RangeLabel:  label,
			RangeMinPct: pct,
			RangeMaxPct: maxPct,
		}

		var rois []float64
		for _, dp := range points {
			if dp.BuyPctOfCL >= pct && dp.BuyPctOfCL < maxPct {
				bucket.Count++
				bucket.TotalProfit += dp.ProfitCents
				rois = append(rois, dp.ROI)
			}
		}

		if bucket.Count > 0 {
			sum := 0.0
			for _, r := range rois {
				sum += r
			}
			bucket.AvgROI = sum / float64(bucket.Count)

			// Inline median calculation
			sort.Float64s(rois)
			n := len(rois)
			if n%2 == 0 {
				bucket.MedianROI = (rois[n/2-1] + rois[n/2]) / 2
			} else {
				bucket.MedianROI = rois[n/2]
			}
		}

		buckets = append(buckets, bucket)
	}

	// Find optimal bucket (best median ROI with at least 3 data points)
	optimalPct := currentPct
	bestMedianROI := -math.MaxFloat64
	for _, b := range buckets {
		if b.Count >= 3 && b.MedianROI > bestMedianROI {
			bestMedianROI = b.MedianROI
			optimalPct = (b.RangeMinPct + b.RangeMaxPct) / 2
		}
	}

	confidence := len(points)

	return &BuyThresholdAnalysis{
		DataPoints:  points,
		OptimalPct:  optimalPct,
		CurrentPct:  currentPct,
		BucketedROI: buckets,
		SampleSize:  len(points),
		Confidence:  confidence,
	}
}

// computeMarketAlignment assesses how the campaign's segment is trending.
func computeMarketAlignment(data []PurchaseWithSale, currentSnapshots map[string]*MarketSnapshot) *MarketAlignment {
	ma := &MarketAlignment{}
	driftSamples := 0

	for _, d := range data {
		if d.Sale != nil {
			continue // only unsold
		}
		key := purchaseKey(d.Purchase.CardName, d.Purchase.CardNumber, d.Purchase.SetName, d.Purchase.GradeValue)
		snap, ok := currentSnapshots[key]
		if !ok || snap == nil {
			continue
		}

		ma.SampleSize++
		ma.AvgTrend30d += snap.Trend30d
		ma.AvgTrend90d += snap.Trend90d
		ma.AvgVolatility += snap.Volatility
		ma.AvgSalesLast30d += float64(snap.SalesLast30d)

		// Compute drift from purchase snapshot to current
		if d.Purchase.MedianCents > 0 && snap.MedianCents > 0 {
			drift := float64(snap.MedianCents-d.Purchase.MedianCents) / float64(d.Purchase.MedianCents)
			ma.AvgSnapshotDrift += drift
			driftSamples++
			if drift > marketDriftThreshold {
				ma.AppreciatingCount++
			} else if drift < -marketDriftThreshold {
				ma.DepreciatingCount++
			} else {
				ma.StableCount++
			}
		} else {
			ma.StableCount++
		}
	}

	if ma.SampleSize == 0 {
		return nil
	}

	n := float64(ma.SampleSize)
	ma.AvgTrend30d /= n
	ma.AvgTrend90d /= n
	ma.AvgVolatility /= n
	ma.AvgSalesLast30d /= n
	if driftSamples > 0 {
		ma.AvgSnapshotDrift /= float64(driftSamples)
	}

	// Determine signal
	switch {
	case ma.AvgTrend30d < trendWarningThreshold || ma.AvgSalesLast30d < lowLiquidityThreshold || ma.DepreciatingCount > 2*ma.AppreciatingCount:
		ma.Signal = "warning"
		if ma.AvgTrend30d < trendWarningThreshold {
			ma.SignalReason = fmt.Sprintf("Market trending down %.1f%% over 30 days", ma.AvgTrend30d*100)
		} else if ma.AvgSalesLast30d < lowLiquidityThreshold {
			ma.SignalReason = fmt.Sprintf("Low liquidity: avg %.1f sales/month in target segment", ma.AvgSalesLast30d)
		} else {
			ma.SignalReason = fmt.Sprintf("%d cards depreciating vs %d appreciating", ma.DepreciatingCount, ma.AppreciatingCount)
		}
	case ma.AvgTrend30d >= 0 && ma.AvgSalesLast30d >= healthyLiquidityThreshold && ma.DepreciatingCount < ma.AppreciatingCount:
		ma.Signal = "healthy"
		ma.SignalReason = fmt.Sprintf("Positive trend (+%.1f%%), good liquidity (%.1f sales/mo), %d/%d cards appreciating",
			ma.AvgTrend30d*100, ma.AvgSalesLast30d, ma.AppreciatingCount, ma.SampleSize)
	default:
		ma.Signal = "caution"
		ma.SignalReason = fmt.Sprintf("Mixed signals: trend %.1f%%, liquidity %.1f sales/mo, %d appreciating / %d depreciating",
			ma.AvgTrend30d*100, ma.AvgSalesLast30d, ma.AppreciatingCount, ma.DepreciatingCount)
	}

	return ma
}

// enrichCardPerformance sets CurrentMarket and recalculates UnrealizedPnL using live market data.
func enrichCardPerformance(cards []CardPerformance, snapshots map[string]*MarketSnapshot) {
	for i := range cards {
		if cards[i].Sale != nil {
			continue
		}
		key := purchaseKey(cards[i].Purchase.CardName, cards[i].Purchase.CardNumber, cards[i].Purchase.SetName, cards[i].Purchase.GradeValue)
		if snap, ok := snapshots[key]; ok && snap != nil {
			cards[i].CurrentMarket = snap
			if snap.MedianCents > 0 {
				costBasis := cards[i].Purchase.BuyCostCents + cards[i].Purchase.PSASourcingFeeCents
				cards[i].UnrealizedPnL = snap.MedianCents - costBasis
			}
		}
	}
}

func percentileInt(sorted []int, pct int) int {
	if len(sorted) == 0 {
		return 0
	}
	idx := len(sorted) * pct / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
