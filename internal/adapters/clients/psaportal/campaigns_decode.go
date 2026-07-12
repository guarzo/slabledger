package psaportal

import (
	"time"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// asString type-asserts a decoded any value to string, defaulting to "".
func asString(v any) string {
	s, _ := v.(string)
	return s
}

// asInt type-asserts a decoded any value (JSON number decodes as float64) to int.
func asInt(v any) int {
	f, _ := v.(float64)
	return int(f)
}

// asIntCents converts a decoded USD float64 value into integer cents.
func asIntCents(v any) int {
	f, _ := v.(float64)
	return int(f*100 + 0.5)
}

// asTime parses an RFC3339 timestamp string, returning the zero time on failure.
func asTime(v any) time.Time {
	s, _ := v.(string)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// asSubjectRefs converts a decoded []any of {id,name} objects into SubjectRef.
func asSubjectRefs(v any) []psacampaign.SubjectRef {
	arr, _ := v.([]any)
	out := make([]psacampaign.SubjectRef, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, psacampaign.SubjectRef{
			ID:   asInt(m["id"]),
			Name: asString(m["name"]),
		})
	}
	return out
}

// decodeFormData maps a decoded formData object into CampaignFormData.
func decodeFormData(fd map[string]any) (psacampaign.CampaignFormData, error) {
	return psacampaign.CampaignFormData{
		CampaignName:                asString(fd["campaignName"]),
		CampaignType:                asString(fd["campaignType"]),
		Category:                    asString(fd["category"]),
		IsActive:                    boolVal(fd["isActive"]),
		BidPercentage:               asInt(fd["bidPercentage"]),
		FlatFee:                     asInt(fd["flatFee"]),
		DailyBudget:                 asInt(fd["dailyBudget"]),
		DailySpecLimit:              asInt(fd["dailySpecLimit"]),
		GradeMinimum:                asString(fd["gradeMinimum"]),
		GradeMaximum:                asString(fd["gradeMaximum"]),
		YearMinimum:                 asInt(fd["yearMinimum"]),
		YearMaximum:                 asInt(fd["yearMaximum"]),
		PriceMinimum:                asInt(fd["priceMinimum"]),
		PriceMaximum:                asInt(fd["priceMaximum"]),
		CardLadderConfidenceMinimum: asInt(fd["cardLadderConfidenceMinimum"]),
		PublisherFilterType:         asString(fd["publisherFilterType"]),
		SelectedPublishers:          asSubjectRefs(fd["selectedPublishers"]),
		SubjectFilterType:           asString(fd["subjectFilterType"]),
		SelectedSubjects:            asSubjectRefs(fd["selectedSubjects"]),
		DeniedSpecs:                 asSubjectRefs(fd["deniedSpecs"]),
	}, nil
}

// boolVal type-asserts a decoded any value to bool, defaulting to false.
func boolVal(v any) bool {
	b, _ := v.(bool)
	return b
}
