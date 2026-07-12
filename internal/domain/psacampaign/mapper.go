package psacampaign

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TranslateToDiff compares an internal campaign against the current portal
// campaign and returns the scalar/range field changes needed to make the
// portal match internal. Subject/inclusion-list translation is deferred (v1).
func TranslateToDiff(internal inventory.Campaign, portal PortalCampaign) (ProposedDiff, error) {
	var d ProposedDiff
	add := func(field, old, newv string) {
		if old != newv {
			d.Changes = append(d.Changes, FieldChange{Field: field, Old: old, New: newv})
		}
	}

	newBid := strconv.Itoa(int(internal.BuyTermsCLPct*100 + 0.5))
	add("bidPercentage", strconv.Itoa(portal.BuyPercentClv), newBid)

	add("dailyBudget", strconv.Itoa(portal.DailyBudgetCents/100),
		strconv.Itoa(internal.DailySpendCapCents/100))

	gMin, gMax, err := splitRange(internal.GradeRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: grade range: %w", err)
	}
	add("gradeMinimum", portal.BuyBox.GradeMin, gMin)
	add("gradeMaximum", portal.BuyBox.GradeMax, gMax)

	yMin, yMax, err := splitRange(internal.YearRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: year range: %w", err)
	}
	add("yearMinimum", strconv.Itoa(portal.BuyBox.YearMin), yMin)
	add("yearMaximum", strconv.Itoa(portal.BuyBox.YearMax), yMax)

	pMin, pMax, err := splitRange(internal.PriceRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: price range: %w", err)
	}
	add("priceMinimum", strconv.Itoa(portal.BuyBox.PriceMinCents/100), pMin)
	add("priceMaximum", strconv.Itoa(portal.BuyBox.PriceMaxCents/100), pMax)

	if cMin, _, err := splitRange(internal.CLConfidence); err == nil {
		add("cardLadderConfidenceMinimum", strconv.Itoa(portal.BuyBox.ClvConfidenceMin), cMin)
	}
	return d, nil
}

// splitRange parses "a-b" (or a single "a") into its two ends as trimmed strings.
func splitRange(s string) (lo, hi string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", fmt.Errorf("empty range")
	}
	parts := strings.SplitN(s, "-", 2)
	lo = strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return lo, lo, nil
	}
	return lo, strings.TrimSpace(parts[1]), nil
}
