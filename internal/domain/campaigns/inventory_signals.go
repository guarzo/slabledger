package campaigns

import (
	"time"
)

// Signal thresholds — tunable constants for inventory signal detection.
const (
	staleDays              = 14
	deepStaleDays          = 30
	spikeThreshold         = 0.10
	minRecentSalesForSpike = 2
	recentSoldMaxDays      = 14
)

// ComputeInventorySignals determines procedural flags for an unsold card.
// isCrackCandidate should be pre-computed from GetCrackOpportunities.
func ComputeInventorySignals(item *AgingItem, isCrackCandidate bool) InventorySignals {
	var sig InventorySignals

	mkt := item.CurrentMarket
	costBasis := item.Purchase.BuyCostCents + item.Purchase.PSASourcingFeeCents

	if isCrackCandidate {
		sig.CrackCandidate = true
	}

	if item.DaysHeld > staleDays {
		sig.StaleListing = true
	}
	if item.DaysHeld > deepStaleDays {
		sig.DeepStale = true
	}

	if mkt == nil {
		return sig
	}

	profitable := costBasis > 0 && mkt.MedianCents > costBasis
	recentSold := hasRecentLastSold(mkt)

	if recentSold && profitable && mkt.Trend30d < 0 {
		sig.ProfitCaptureDeclining = true
	}

	if profitable && mkt.Trend30d >= spikeThreshold && mkt.SalesLast30d >= minRecentSalesForSpike {
		sig.ProfitCaptureSpike = true
	}

	if sig.DeepStale {
		underwater := costBasis > 0 && mkt.MedianCents > 0 && mkt.MedianCents < costBasis
		if mkt.Trend30d < 0 || underwater {
			sig.CutLoss = true
		}
	}

	return sig
}

func hasRecentLastSold(mkt *MarketSnapshot) bool {
	if mkt.LastSoldDate == "" || mkt.LastSoldCents <= 0 {
		return false
	}
	t, err := time.Parse("2006-01-02", mkt.LastSoldDate)
	if err != nil {
		return false
	}
	return time.Since(t).Hours()/24 <= recentSoldMaxDays
}
