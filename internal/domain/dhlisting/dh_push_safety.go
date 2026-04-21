package dhlisting

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// ResolveListingPriceCents returns the operator-committed price for DH
// listing. Both ReviewedPriceCents (price-review flow) and OverridePriceCents
// (Set Price dialog) are treated as human commitments; when both are set,
// the newest commit wins. Timestamps are compared by parsed time so RFC3339
// values with different offsets compare chronologically — string comparison
// alone would mis-order "2026-04-21T08:00:00-05:00" (13:00 UTC) against
// "2026-04-21T12:00:00Z" (12:00 UTC). When either timestamp fails to parse
// (most commonly because it's empty on legacy rows), we fall back to
// lexicographic comparison — which preserves the historical "reviewed wins
// when both empty" and "populated timestamp beats empty" behaviors.
// CL is deliberately excluded: it can be stale and we don't want to
// silently list at a wrong price. Returns 0 when neither field is set —
// callers treat that as "omit listing_price_cents and let DH's catalog
// fallback take over" (fine for in_stock, rejected at list time).
func ResolveListingPriceCents(p *inventory.Purchase) int {
	if p.OverridePriceCents == 0 {
		return p.ReviewedPriceCents
	}
	if p.ReviewedPriceCents == 0 {
		return p.OverridePriceCents
	}
	if overrideNewer(p.OverrideSetAt, p.ReviewedAt) {
		return p.OverridePriceCents
	}
	return p.ReviewedPriceCents
}

// overrideNewer reports whether the override commit timestamp is strictly
// after the reviewed commit timestamp. Both are RFC3339 strings written by
// the storage layer; we parse them to compare wall-clock instants rather
// than text. On parse failure (empty or malformed timestamps) we fall back
// to string comparison so legacy rows preserve their prior resolution.
func overrideNewer(overrideSetAt, reviewedAt string) bool {
	tOverride, errO := time.Parse(time.RFC3339, overrideSetAt)
	tReviewed, errR := time.Parse(time.RFC3339, reviewedAt)
	if errO == nil && errR == nil {
		return tOverride.After(tReviewed)
	}
	return overrideSetAt > reviewedAt
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
