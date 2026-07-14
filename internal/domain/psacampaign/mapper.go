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

// Portal constants for create-time fields the internal campaign doesn't carry.
const (
	createCampaignType   = "CATEGORY"
	createCategory       = "POKEMON"
	createDailySpecLimit = 2
	filterTypeTarget     = "Target"
)

// TranslateToCreate builds the full createCampaign formData for an internal
// campaign. The portal campaign is always created paused (IsActive false);
// money fields are whole USD on the wire (internal cents / 100). Subject and
// publisher lists are deferred (v1) — created empty, filled in the portal UI
// before activation.
func TranslateToCreate(internal inventory.Campaign) (CampaignFormData, error) {
	var fd CampaignFormData

	gMin, gMax, err := splitRange(internal.GradeRange)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: grade range: %w", err)
	}
	yMin, yMax, err := splitRangeInts(internal.YearRange)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: year range: %w", err)
	}
	pMin, pMax, err := splitRangeInts(internal.PriceRange)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: price range: %w", err)
	}
	clMinStr, _, err := splitRange(internal.CLConfidence)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: cl confidence: %w", err)
	}
	clF, err := strconv.ParseFloat(clMinStr, 64)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: cl confidence: %w", err)
	}
	clMin := int(clF)

	return CampaignFormData{
		CampaignName:                internal.Name,
		CampaignType:                createCampaignType,
		Category:                    createCategory,
		PrepackagedSpecListIDs:      []string{},
		IsActive:                    false,
		BidPercentage:               int(internal.BuyTermsCLPct*100 + 0.5),
		FlatFee:                     centsToWholeUSD(internal.PSASourcingFeeCents),
		DailyBudget:                 centsToWholeUSD(internal.DailySpendCapCents),
		DailySpecLimit:              createDailySpecLimit,
		GradeMinimum:                gMin,
		GradeMaximum:                gMax,
		YearMinimum:                 yMin,
		YearMaximum:                 yMax,
		PriceMinimum:                pMin,
		PriceMaximum:                pMax,
		CardLadderConfidenceMinimum: clMin,
		PublisherFilterType:         filterTypeTarget,
		SelectedPublishers:          []SubjectRef{},
		SubjectFilterType:           filterTypeTarget,
		SelectedSubjects:            []SubjectRef{},
		DeniedSpecs:                 []SubjectRef{},
	}, nil
}

// centsToWholeUSD converts a cent value to whole USD for the portal wire,
// rounding to nearest dollar so sub-dollar remainders aren't silently dropped.
func centsToWholeUSD(cents int) int {
	return (cents + 50) / 100
}

// splitRangeInts parses "a-b" (or "a") into integer ends.
func splitRangeInts(s string) (lo, hi int, err error) {
	loS, hiS, err := splitRange(s)
	if err != nil {
		return 0, 0, err
	}
	if lo, err = strconv.Atoi(loS); err != nil {
		return 0, 0, fmt.Errorf("low bound %q: %w", loS, err)
	}
	if hi, err = strconv.Atoi(hiS); err != nil {
		return 0, 0, fmt.Errorf("high bound %q: %w", hiS, err)
	}
	return lo, hi, nil
}
