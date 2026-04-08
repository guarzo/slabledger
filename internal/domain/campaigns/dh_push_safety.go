package campaigns

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// DHPushConfig holds admin-configurable thresholds for DH push safety gates.
type DHPushConfig struct {
	SwingPctThreshold            int       `json:"swingPctThreshold"`
	SwingMinCents                int       `json:"swingMinCents"`
	DisagreementPctThreshold     int       `json:"disagreementPctThreshold"`
	UnreviewedChangePctThreshold int       `json:"unreviewedChangePctThreshold"`
	UnreviewedChangeMinCents     int       `json:"unreviewedChangeMinCents"`
	UpdatedAt                    time.Time `json:"updatedAt"`
}

// DefaultDHPushConfig returns sensible defaults for push safety thresholds.
func DefaultDHPushConfig() DHPushConfig {
	return DHPushConfig{
		SwingPctThreshold:            20,
		SwingMinCents:                5000,
		DisagreementPctThreshold:     25,
		UnreviewedChangePctThreshold: 15,
		UnreviewedChangeMinCents:     3000,
	}
}

// ResolveMarketValueCents returns the best available price for DH push:
// reviewed price > CL value > 0.
func ResolveMarketValueCents(p *Purchase) int {
	if p.ReviewedPriceCents > 0 {
		return p.ReviewedPriceCents
	}
	if p.CLValueCents > 0 {
		return p.CLValueCents
	}
	return 0
}

// EvaluateHoldTriggers checks whether a re-push should be held for review.
// Returns empty string if the push should proceed, or a reason string if held.
// Only applies to re-pushes (DHInventoryID != 0).
func EvaluateHoldTriggers(p *Purchase, cfg DHPushConfig) string {
	if p.DHInventoryID == 0 {
		return ""
	}

	newValue := ResolveMarketValueCents(p)
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

func checkSourceDisagreement(p *Purchase, pctThreshold int) string {
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

func checkUnreviewedCLChange(p *Purchase, lastPushed, pctThreshold, minCents int) string {
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
