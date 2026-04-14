package inventory

import "math"

// ComputeROIStats returns population stddev and CV for a slice of per-sale ROI values.
// Returns (0, 0) if fewer than 2 values are provided.
func ComputeROIStats(rois []float64) (stddev, cv float64) {
	if len(rois) < 2 {
		return 0, 0
	}
	sum := 0.0
	for _, r := range rois {
		sum += r
	}
	mean := sum / float64(len(rois))
	varSum := 0.0
	for _, r := range rois {
		d := r - mean
		varSum += d * d
	}
	stddev = math.Sqrt(varSum / float64(len(rois)))
	if math.Abs(mean) > 0 {
		cv = stddev / math.Abs(mean)
	}
	return stddev, cv
}

// EnrichPriceTierStddev computes RoiStddev and CV for each PriceTierPerformance entry
// using per-sale ROI values derived from data.
// A sale's tier is matched by (BuyCostCents + PSASourcingFeeCents) ∈ [TierMinCents, TierMaxCents).
// TierMaxCents == 0 is treated as +∞.
func EnrichPriceTierStddev(tiers []PriceTierPerformance, data []PurchaseWithSale) {
	if len(tiers) == 0 {
		return
	}
	tierROIs := make([][]float64, len(tiers))
	for _, d := range data {
		if d.Sale == nil {
			continue
		}
		cost := d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		if cost <= 0 {
			continue
		}
		roi := float64(d.Sale.NetProfitCents) / float64(cost)
		for i, tier := range tiers {
			if cost >= tier.TierMinCents && (tier.TierMaxCents == 0 || cost < tier.TierMaxCents) {
				tierROIs[i] = append(tierROIs[i], roi)
				break
			}
		}
	}
	for i := range tiers {
		tiers[i].RoiStddev, tiers[i].CV = ComputeROIStats(tierROIs[i])
	}
}
