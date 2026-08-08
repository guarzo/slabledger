package psacampaign

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TranslateToDiff compares an internal campaign against the current portal
// campaign and returns the field changes needed to make the portal match
// internal, across both the scalar buy-box fields and the three targeting
// axes (spec list, subject filter, denied specs).
func TranslateToDiff(internal inventory.Campaign, portal PortalCampaign, r Resolver) (ProposedDiff, error) {
	var d ProposedDiff
	// The old side of every change is rendered by RenderFieldValue rather than
	// inline, because the portal adapter re-renders the live record through the
	// same function to compare-and-swap before writing. Two renderings that can
	// drift would make that check meaningless.
	oldFD := PortalFormData(portal)
	var renderErr error
	add := func(field, newv string) {
		old, ok := RenderFieldValue(field, oldFD)
		if !ok {
			renderErr = fmt.Errorf("%w: %q", ErrNoRenderer, field)
			return
		}
		if old != newv {
			d.Changes = append(d.Changes, FieldChange{Field: field, Old: old, New: newv})
		}
	}
	// addList is add's list-valued sibling: it carries the typed new value
	// push.go sends to the portal instead of the display string in New.
	addList := func(field, newRendering string, value any) {
		old, ok := RenderFieldValue(field, oldFD)
		if !ok {
			renderErr = fmt.Errorf("%w: %q", ErrNoRenderer, field)
			return
		}
		if old != newRendering {
			d.Changes = append(d.Changes, FieldChange{Field: field, Old: old, New: newRendering, Value: value})
		}
	}

	newBid := strconv.Itoa(int(internal.BuyTermsCLPct*100 + 0.5))
	add("bidPercentage", newBid)

	add("dailyBudget", strconv.Itoa(internal.DailySpendCapCents/100))

	gMin, gMax, err := splitRange(internal.GradeRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: grade range: %w", err)
	}
	add("gradeMinimum", gMin)
	add("gradeMaximum", gMax)

	yMin, yMax, err := splitRange(internal.YearRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: year range: %w", err)
	}
	add("yearMinimum", yMin)
	add("yearMaximum", yMax)

	pMin, pMax, err := splitRange(internal.PriceRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: price range: %w", err)
	}
	add("priceMinimum", pMin)
	add("priceMaximum", pMax)

	if cMin, _, err := splitRange(internal.CLConfidence); err == nil {
		add("cardLadderConfidenceMinimum", cMin)
	}

	add("subjectFilterType", internal.SubjectFilterMode)

	selectedSubjects, err := toSubjectRefs(internal.Subjects, r)
	if err != nil {
		return d, err
	}
	addList("selectedSubjects", renderSubjectRefs(selectedSubjects), selectedSubjects)

	deniedSpecs, err := toSubjectRefs(internal.DeniedSpecs, r)
	if err != nil {
		return d, err
	}
	addList("deniedSpecs", renderSubjectRefs(deniedSpecs), deniedSpecs)

	// An empty TargetLanguages set means either that this campaign has no
	// spec-list axis to propose yet (legacy/unlinked), or that it is a
	// deliberate open net. The portal can express neither: a SPEC_LIST
	// campaign must carry at least one curated list, so proposing anything
	// here would mean proposing an EMPTY prepackagedSpecListIds — clearing
	// every curated list off a live campaign. The axis is therefore skipped,
	// which also keeps an unlinked campaign from blocking every other scalar
	// fix in this diff.
	if len(internal.TargetLanguages) > 0 {
		specListIDs, err := r.SpecListIDs(internal.TargetLanguages)
		if err != nil {
			return d, fmt.Errorf("psacampaign: resolve spec lists for languages %v: %w", internal.TargetLanguages, err)
		}
		addList("prepackagedSpecListIds", renderStringList(specListIDs), specListIDs)
	}

	if renderErr != nil {
		return ProposedDiff{}, renderErr
	}
	return d, nil
}

// renderSubjectRefs renders subject refs as a canonical, order-insensitive
// string: sorted by ID ascending, "id:name" pairs comma-joined. Two lists
// holding the same subjects in a different order render identically, so an
// unordered portal response never produces a spurious diff.
func renderSubjectRefs(refs []SubjectRef) string {
	sorted := append([]SubjectRef(nil), refs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	parts := make([]string, len(sorted))
	for i, ref := range sorted {
		parts[i] = fmt.Sprintf("%d:%s", ref.ID, ref.Name)
	}
	return strings.Join(parts, ",")
}

// renderStringList renders a string list canonically (sorted, comma-joined)
// for the same order-insensitivity reason as renderSubjectRefs.
func renderStringList(ss []string) string {
	sorted := append([]string(nil), ss...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// toSubjectRefs converts internal.TargetSubject entries to the portal's
// SubjectRef wire shape. An entry with a positive ID is portal-sourced and
// passes through verbatim — it is never re-resolved by name, because live
// portal ids span multiple id generations (4xxx/8xxx/22xxx) that getSubjects
// cannot reproduce. Only ID == 0 entries (operator-entered names never yet
// reconciled with the portal) are resolved via r; a resolution failure returns
// an error naming the subject rather than silently dropping it from what the
// campaign buys.
//
// inventory.LegacyUnreconciledSubjectID is refused outright. Migration 000023
// backfills legacy inclusion-list tokens with that sentinel precisely so they
// are distinguishable from the operator-typed ID == 0 case above: the two were
// previously identical, so a propose issued between deploy and the baseline
// pull would have re-resolved legacy subjects by name and swapped live
// 4xxx/8xxx portal ids for current-generation 22xxx ids. Refusing is the only
// safe answer, since translation has no portal session with which to reconcile
// them itself.
func toSubjectRefs(subjects []inventory.TargetSubject, r Resolver) ([]SubjectRef, error) {
	out := make([]SubjectRef, 0, len(subjects))
	for _, s := range subjects {
		id := s.ID
		switch id {
		case inventory.LegacyUnreconciledSubjectID:
			return nil, fmt.Errorf("%w (subject %q)", ErrLegacySubjectsUnreconciled, s.Name)
		case 0:
			resolved, err := r.SubjectID(s.Name)
			if err != nil {
				return nil, fmt.Errorf("psacampaign: resolve subject %q: %w", s.Name, err)
			}
			id = resolved
		}
		out = append(out, SubjectRef{ID: id, Name: s.Name})
	}
	return out, nil
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

// createDailySpecLimit is the portal-side daily spec cap SlabLedger always
// creates with; the internal campaign carries no equivalent field (v1).
const createDailySpecLimit = 2

// TranslateToCreate builds the full createCampaign formData for an internal
// campaign. The portal campaign is always created paused (IsActive false);
// money fields are whole USD on the wire (internal cents / 100). Campaigns
// are created as SPEC_LIST (the CATEGORY/POKEMON shape PSA has retired) with
// every curated spec list resolved from the campaign's TargetLanguages set,
// and the subject filter carries the campaign's Subjects/DeniedSpecs.
func TranslateToCreate(internal inventory.Campaign, r Resolver) (CampaignFormData, error) {
	var fd CampaignFormData

	if len(internal.TargetLanguages) == 0 {
		return fd, fmt.Errorf("psacampaign: campaign has no target languages set")
	}
	specListIDs, err := r.SpecListIDs(internal.TargetLanguages)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: resolve spec lists for languages %v: %w", internal.TargetLanguages, err)
	}

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

	selectedSubjects, err := toSubjectRefs(internal.Subjects, r)
	if err != nil {
		return fd, err
	}
	deniedSpecs, err := toSubjectRefs(internal.DeniedSpecs, r)
	if err != nil {
		return fd, err
	}

	return CampaignFormData{
		CampaignName:                internal.Name,
		CampaignType:                "SPEC_LIST",
		Category:                    "",
		PrepackagedSpecListIDs:      specListIDs,
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
		PublisherFilterType:         "Target",
		SelectedPublishers:          []SubjectRef{},
		SubjectFilterType:           internal.SubjectFilterMode,
		SelectedSubjects:            selectedSubjects,
		DeniedSpecs:                 deniedSpecs,
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
