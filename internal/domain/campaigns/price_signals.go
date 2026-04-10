package campaigns

import "math"

// applyCLSignal incorporates Card Ladder auction-based data into the market snapshot.
// CL is weighted at 30% alongside market data (70%) and serves as a floor — the
// market price should not fall below CL's auction-derived valuation.
//
// NOTE: Double CL correction — applyCLCorrection (service_snapshots.go) already
// blends CL into the stored SnapshotJSON median at write time. This function applies
// a second CL blend at read time. When SnapshotJSON is present, CL may be counted
// twice. This is a known limitation; the floor behavior ensures the net effect is
// conservative (prevents underpricing rather than overpricing).
func applyCLSignal(snap *MarketSnapshot, clCents int) {
	if snap == nil || clCents <= 0 {
		return
	}

	// Add CL as a visible source price
	snap.SourcePrices = append(snap.SourcePrices, SourcePrice{
		Source:     "CardLadder",
		PriceCents: clCents,
	})

	// Blend CL into the median: 70% market data, 30% CL
	if snap.MedianCents > 0 {
		blended := int(math.Round(float64(snap.MedianCents)*0.7 + float64(clCents)*0.3))
		// CL as floor: don't let blended price drop below CL
		if blended < clCents {
			blended = clCents
		}
		snap.MedianCents = blended
	} else {
		// No market median — use CL directly
		snap.MedianCents = clCents
	}

	// Recalculate fallback-derived percentiles from updated median
	// (only when they were originally computed as fallbacks, i.e., proportional to median)
	if snap.ConservativeCents > 0 && snap.P10Cents > 0 {
		// Distribution data exists — leave percentiles as-is
		return
	}
	if snap.MedianCents > 0 {
		if snap.ConservativeCents == 0 {
			snap.ConservativeCents = int(math.Round(float64(snap.MedianCents) * 0.85))
		}
		if snap.OptimisticCents == 0 {
			snap.OptimisticCents = int(math.Round(float64(snap.MedianCents) * 1.15))
		}
		if snap.P10Cents == 0 {
			snap.P10Cents = int(math.Round(float64(snap.MedianCents) * 0.70))
		}
		if snap.P90Cents == 0 {
			snap.P90Cents = int(math.Round(float64(snap.MedianCents) * 1.30))
		}
	}
}

// applyMMSignal incorporates Market Movers data into the market snapshot.
// MM avg price is added as a visible source price. When the DH snapshot lacks trend or
// volume data (e.g. card not in the DH catalog), MM values are used as fallbacks so that
// ComputeInventorySignals and advisor scoring remain fully functional.
// If no DH median exists, the MM avg price seeds the snapshot directly.
func applyMMSignal(snap *MarketSnapshot, p *Purchase) {
	if snap == nil || p.MMValueCents <= 0 {
		return
	}

	snap.SourcePrices = append(snap.SourcePrices, SourcePrice{
		Source:     "MarketMovers",
		PriceCents: p.MMValueCents,
	})

	// Use MM avg as median seed only when DH has not provided one
	if snap.MedianCents == 0 {
		snap.MedianCents = p.MMValueCents
		if snap.ConservativeCents == 0 {
			snap.ConservativeCents = int(math.Round(float64(p.MMValueCents) * 0.85))
		}
		if snap.OptimisticCents == 0 {
			snap.OptimisticCents = int(math.Round(float64(p.MMValueCents) * 1.15))
		}
	}

	// Populate LowestListCents from MM active BIN when DH hasn't set it
	if snap.LowestListCents == 0 && p.MMActiveLowCents > 0 {
		snap.LowestListCents = p.MMActiveLowCents
	}

	// Use MM trend and volume as fallbacks when DH snapshot has none
	if snap.Trend30d == 0 && p.MMTrendPct != 0 {
		snap.Trend30d = p.MMTrendPct
	}
	if snap.SalesLast30d == 0 && p.MMSales30d > 0 {
		snap.SalesLast30d = p.MMSales30d
	}
}
