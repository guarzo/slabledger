package dhlisting

import (
	"fmt"
	"math"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// ResolveListingPriceCents returns the operator-committed price for DH
// listing. ReviewedPriceCents wins; OverridePriceCents is the fallback so
// prices set through the "Set Price" dialog (which writes override) still
// flow to DH. CL is deliberately excluded: it can be stale and we don't
// want to silently list at a wrong price. Returns 0 when neither field is
// set, which callers treat as "omit listing_price_cents and let DH's
// catalog fallback take over" (fine for in_stock, rejected at list time).
func ResolveListingPriceCents(p *inventory.Purchase) int {
	if p.ReviewedPriceCents > 0 {
		return p.ReviewedPriceCents
	}
	return p.OverridePriceCents
}

// EvaluateHoldTriggers checks whether a push should be held for review.
// For initial pushes (DHInventoryID == 0): checks market value vs buy cost.
// For re-pushes: checks price swing, source disagreement, and unreviewed CL change.
// Returns empty string if the push should proceed, or a reason string if held.
func EvaluateHoldTriggers(p *inventory.Purchase, cfg inventory.DHPushConfig) string {
	if p.DHInventoryID == 0 {
		return checkInitialPushValueMismatch(p, cfg.InitialPushValueFloorPct)
	}

	newValue := ResolveListingPriceCents(p)
	lastPushed := p.DHListingPriceCents
	if lastPushed == 0 || newValue == 0 {
		return ""
	}

	// Trigger 1: Price swing
	if reason := checkPriceSwing(newValue, lastPushed, cfg.SwingPctThreshold, cfg.SwingMinCents); reason != "" {
		return reason
	}

	// Trigger 2: Source disagreement
	if reason := checkSourceDisagreement(p, cfg.DisagreementPctThreshold); reason != "" {
		return reason
	}

	// Trigger 3: Unreviewed CL change
	if reason := checkUnreviewedCLChange(p, lastPushed, cfg.UnreviewedChangePctThreshold, cfg.UnreviewedChangeMinCents); reason != "" {
		return reason
	}

	return ""
}

func checkPriceSwing(newValue, lastPushed, pctThreshold, minCents int) string {
	if lastPushed == 0 {
		return ""
	}
	delta := newValue - lastPushed
	absDelta := int(math.Abs(float64(delta)))
	pct := float64(delta) / float64(lastPushed) * 100

	if math.Abs(pct) > float64(pctThreshold) && absDelta > minCents {
		return fmt.Sprintf("price_swing:%+.0f%%", pct)
	}
	return ""
}

func checkSourceDisagreement(p *inventory.Purchase, pctThreshold int) string {
	prices := make(map[string]int)
	if p.CLValueCents > 0 {
		prices["cl"] = p.CLValueCents
	}
	if p.ReviewedPriceCents > 0 {
		prices["reviewed"] = p.ReviewedPriceCents
	}
	if p.LastSoldCents > 0 {
		prices["last_sold"] = p.LastSoldCents
	}

	if len(prices) < 2 {
		return ""
	}

	keys := make([]string, 0, len(prices))
	for name := range prices {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			nameA, nameB := keys[i], keys[j]
			a, b := prices[nameA], prices[nameB]
			maxVal := max(a, b)
			minVal := min(a, b)
			if maxVal > 0 {
				pct := float64(maxVal-minVal) / float64(maxVal) * 100
				if pct > float64(pctThreshold) {
					return fmt.Sprintf("source_disagreement:%s=%d,%s=%d", nameA, a, nameB, b)
				}
			}
		}
	}
	return ""
}

func checkUnreviewedCLChange(p *inventory.Purchase, lastPushed, pctThreshold, minCents int) string {
	if p.ReviewedPriceCents > 0 {
		return ""
	}
	if lastPushed == 0 {
		return ""
	}
	delta := p.CLValueCents - lastPushed
	absDelta := int(math.Abs(float64(delta)))
	pct := float64(delta) / float64(lastPushed) * 100

	if math.Abs(pct) > float64(pctThreshold) && absDelta > minCents {
		return fmt.Sprintf("unreviewed_cl_change:%+.0f%%", pct)
	}
	return ""
}

// checkInitialPushValueMismatch holds an initial push if the market value
// is significantly below the buy cost, suggesting a possible data error.
func checkInitialPushValueMismatch(p *inventory.Purchase, floorPct int) string {
	if floorPct <= 0 {
		floorPct = inventory.DefaultDHPushConfig().InitialPushValueFloorPct
	}
	if p.BuyCostCents == 0 {
		return ""
	}
	marketValue := ResolveListingPriceCents(p)
	if marketValue == 0 {
		return ""
	}
	floor := p.BuyCostCents * floorPct / 100
	if marketValue < floor {
		pct := float64(marketValue) / float64(p.BuyCostCents) * 100
		return fmt.Sprintf("initial_value_mismatch:market=%d,cost=%d,ratio=%.0f%%", marketValue, p.BuyCostCents, pct)
	}
	return ""
}
