package psacampaign

import (
	"errors"
	"strconv"
)

// ErrFieldStale is returned when the portal's live value for a field no longer
// matches the value the approver saw. It is deliberately terminal: a push that
// would overwrite a change made after approval must not be retried.
var ErrFieldStale = errors.New("psacampaign: portal value changed since approval")

// ErrNoRenderer is returned for a field with no canonical rendering. It fails
// closed on purpose — a field whose live value cannot be rendered cannot be
// compare-and-swapped, and an unverifiable field is not a safe one to push.
var ErrNoRenderer = errors.New("psacampaign: no canonical rendering for field")

// RenderFieldValue renders one formData field as the canonical string used on
// both sides of a push: TranslateToDiff records it as FieldChange.Old, and the
// portal adapter re-renders the live record just before writing and refuses the
// push if the two disagree. Both sides must go through this one function — if
// they rendered independently, a formatting drift between them would turn every
// legitimate update into a false staleness failure (or, worse, hide a real one).
//
// The second return is false for fields with no rendering, which callers must
// treat as a refusal rather than as an empty value.
func RenderFieldValue(field string, fd CampaignFormData) (string, bool) {
	switch field {
	case "campaignName":
		return fd.CampaignName, true
	case "campaignType":
		return fd.CampaignType, true
	case "category":
		return fd.Category, true
	case "bidPercentage":
		return strconv.Itoa(fd.BidPercentage), true
	case "flatFee":
		return strconv.Itoa(fd.FlatFee), true
	case "dailyBudget":
		return strconv.Itoa(fd.DailyBudget), true
	case "dailySpecLimit":
		return strconv.Itoa(fd.DailySpecLimit), true
	case "gradeMinimum":
		return fd.GradeMinimum, true
	case "gradeMaximum":
		return fd.GradeMaximum, true
	case "yearMinimum":
		return strconv.Itoa(fd.YearMinimum), true
	case "yearMaximum":
		return strconv.Itoa(fd.YearMaximum), true
	case "priceMinimum":
		return strconv.Itoa(fd.PriceMinimum), true
	case "priceMaximum":
		return strconv.Itoa(fd.PriceMaximum), true
	case "cardLadderConfidenceMinimum":
		return strconv.Itoa(fd.CardLadderConfidenceMinimum), true
	case "publisherFilterType":
		return fd.PublisherFilterType, true
	case "selectedPublishers":
		return renderSubjectRefs(fd.SelectedPublishers), true
	case "subjectFilterType":
		return fd.SubjectFilterType, true
	case "selectedSubjects":
		return renderSubjectRefs(fd.SelectedSubjects), true
	case "deniedSpecs":
		return renderSubjectRefs(fd.DeniedSpecs), true
	case "prepackagedSpecListIds":
		return renderStringList(fd.PrepackagedSpecListIDs), true
	}
	return "", false
}

// PortalFormData projects a PortalCampaign into the edit-form shape so the
// list-endpoint view and the edit-form view can be rendered by one function.
// Unit normalization lives here and only here: the list endpoint reports money
// in cents (asIntCents), the edit form in whole USD, and a divergence between
// those two conversions would read as staleness on every money field.
//
// Fields the list endpoint does not carry (IsActive) stay zero; they are not
// diffable and therefore never reach RenderFieldValue from this side.
func PortalFormData(pc PortalCampaign) CampaignFormData {
	return CampaignFormData{
		CampaignName:                pc.Name,
		CampaignType:                pc.Type,
		Category:                    pc.Category,
		PrepackagedSpecListIDs:      pc.SpecListIDs,
		BidPercentage:               pc.BuyPercentClv,
		FlatFee:                     pc.BuyBox.BuyerFlatFeeCents / 100,
		DailyBudget:                 pc.DailyBudgetCents / 100,
		DailySpecLimit:              pc.DailySpecLimit,
		GradeMinimum:                pc.BuyBox.GradeMin,
		GradeMaximum:                pc.BuyBox.GradeMax,
		YearMinimum:                 pc.BuyBox.YearMin,
		YearMaximum:                 pc.BuyBox.YearMax,
		PriceMinimum:                pc.BuyBox.PriceMinCents / 100,
		PriceMaximum:                pc.BuyBox.PriceMaxCents / 100,
		CardLadderConfidenceMinimum: pc.BuyBox.ClvConfidenceMin,
		PublisherFilterType:         pc.PublisherFilter.Type,
		SelectedPublishers:          pc.PublisherFilter.Subjects,
		SubjectFilterType:           pc.SubjectFilter.Type,
		SelectedSubjects:            pc.SubjectFilter.Subjects,
		DeniedSpecs:                 pc.DeniedSpecs,
	}
}
