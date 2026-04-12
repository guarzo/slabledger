package dhlisting

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// DHPushConfig holds admin-configurable thresholds for DH push safety gates.
// This is a duplicate definition to avoid circular dependencies.
// TODO: Consider moving this to a shared types package if it grows further.
type DHPushConfig struct {
	SwingPctThreshold            int       `json:"swingPctThreshold"`
	SwingMinCents                int       `json:"swingMinCents"`
	DisagreementPctThreshold     int       `json:"disagreementPctThreshold"`
	UnreviewedChangePctThreshold int       `json:"unreviewedChangePctThreshold"`
	UnreviewedChangeMinCents     int       `json:"unreviewedChangeMinCents"`
	InitialPushValueFloorPct     int       `json:"initialPushValueFloorPct"`
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
		InitialPushValueFloorPct:     50,
	}
}

// ResolveMarketValueCents returns the best available price for DH push:
// reviewed price > CL value > 0.
func ResolveMarketValueCents(p *inventory.Purchase) int {
	if p.ReviewedPriceCents > 0 {
		return p.ReviewedPriceCents
	}
	if p.CLValueCents > 0 {
		return p.CLValueCents
	}
	return 0
}

// EvaluateHoldTriggers checks whether a push should be held for review.
// For initial pushes (DHInventoryID == 0): checks market value vs buy cost.
// For re-pushes: checks price swing, source disagreement, and unreviewed CL change.
// Returns empty string if the push should proceed, or a reason string if held.
//
// Note: cfg can be either dhlisting.DHPushConfig or inventory.DHPushConfig;
// they have identical structure so type conversion is safe.
func EvaluateHoldTriggers(p *inventory.Purchase, cfg interface{}) string {
	// Handle both inventory.DHPushConfig and dhlisting.DHPushConfig
	var dhCfg DHPushConfig
	switch v := cfg.(type) {
	case inventory.DHPushConfig:
		dhCfg = DHPushConfig{
			SwingPctThreshold:            v.SwingPctThreshold,
			SwingMinCents:                v.SwingMinCents,
			DisagreementPctThreshold:     v.DisagreementPctThreshold,
			UnreviewedChangePctThreshold: v.UnreviewedChangePctThreshold,
			UnreviewedChangeMinCents:     v.UnreviewedChangeMinCents,
			InitialPushValueFloorPct:     v.InitialPushValueFloorPct,
			UpdatedAt:                    v.UpdatedAt,
		}
	case *inventory.DHPushConfig:
		dhCfg = DHPushConfig{
			SwingPctThreshold:            v.SwingPctThreshold,
			SwingMinCents:                v.SwingMinCents,
			DisagreementPctThreshold:     v.DisagreementPctThreshold,
			UnreviewedChangePctThreshold: v.UnreviewedChangePctThreshold,
			UnreviewedChangeMinCents:     v.UnreviewedChangeMinCents,
			InitialPushValueFloorPct:     v.InitialPushValueFloorPct,
			UpdatedAt:                    v.UpdatedAt,
		}
	case DHPushConfig:
		dhCfg = v
	case *DHPushConfig:
		dhCfg = *v
	default:
		// Fallback to default config if type is unknown
		dhCfg = DefaultDHPushConfig()
	}

	if p.DHInventoryID == 0 {
		return checkInitialPushValueMismatch(p, dhCfg.InitialPushValueFloorPct)
	}

	newValue := ResolveMarketValueCents(p)
	lastPushed := p.DHListingPriceCents
	if lastPushed == 0 || newValue == 0 {
		return ""
	}

	// Trigger 1: Price swing
	if reason := checkPriceSwing(newValue, lastPushed, dhCfg.SwingPctThreshold, dhCfg.SwingMinCents); reason != "" {
		return reason
	}

	// Trigger 2: Source disagreement
	if reason := checkSourceDisagreement(p, dhCfg.DisagreementPctThreshold); reason != "" {
		return reason
	}

	// Trigger 3: Unreviewed CL change
	if reason := checkUnreviewedCLChange(p, lastPushed, dhCfg.UnreviewedChangePctThreshold, dhCfg.UnreviewedChangeMinCents); reason != "" {
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
		floorPct = DefaultDHPushConfig().InitialPushValueFloorPct
	}
	if p.BuyCostCents == 0 {
		return ""
	}
	marketValue := ResolveMarketValueCents(p)
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
