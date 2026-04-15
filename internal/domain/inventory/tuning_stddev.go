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
// Tier match uses BuyCostCents only (matching ComputePriceTierPerformance); the ROI
// denominator uses BuyCostCents + PSASourcingFeeCents (consistent with ComputeROIStats'
// per-sale net-profit-over-total-cost intent). The top tier is expected to carry
// TierMaxCents == math.MaxInt (as emitted by ComputePriceTierPerformance); there is
// no special case for TierMaxCents == 0.
func EnrichPriceTierStddev(tiers []PriceTierPerformance, data []PurchaseWithSale) {
	if len(tiers) == 0 {
		return
	}
	tierROIs := make([][]float64, len(tiers))
	for _, d := range data {
		if d.Sale == nil {
			continue
		}
		bucketCost := d.Purchase.BuyCostCents
		roiCost := d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		if roiCost <= 0 {
			continue
		}
		roi := float64(d.Sale.NetProfitCents) / float64(roiCost)
		for i, tier := range tiers {
			if bucketCost >= tier.TierMinCents && bucketCost < tier.TierMaxCents {
				tierROIs[i] = append(tierROIs[i], roi)
				break
			}
		}
	}
	for i := range tiers {
		tiers[i].RoiStddev, tiers[i].CV = ComputeROIStats(tierROIs[i])
	}
}
