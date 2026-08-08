package psacampaign

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestRenderFieldValue(t *testing.T) {
	fd := CampaignFormData{
		CampaignName:                "Crystal",
		BidPercentage:               70,
		FlatFee:                     3,
		DailyBudget:                 3000,
		DailySpecLimit:              2,
		GradeMinimum:                "1",
		GradeMaximum:                "10",
		YearMinimum:                 2002,
		YearMaximum:                 2003,
		PriceMinimum:                500,
		PriceMaximum:                3000,
		CardLadderConfidenceMinimum: 4,
		SubjectFilterType:           "Target",
		// Deliberately unsorted: the rendering is order-insensitive so an
		// unordered portal response never reads as a change.
		SelectedSubjects:       []SubjectRef{{ID: 4836, Name: "Umbreon"}, {ID: 4807, Name: "Charizard"}},
		DeniedSpecs:            []SubjectRef{},
		PrepackagedSpecListIDs: []string{"b", "a"},
	}

	tests := []struct {
		name   string
		field  string
		want   string
		wantOK bool
	}{
		{name: "scalar int", field: "bidPercentage", want: "70", wantOK: true},
		{name: "whole-usd money", field: "priceMinimum", want: "500", wantOK: true},
		{name: "grade bound stays a string", field: "gradeMaximum", want: "10", wantOK: true},
		{name: "string field", field: "campaignName", want: "Crystal", wantOK: true},
		{name: "subject list sorts by id", field: "selectedSubjects", want: "4807:Charizard,4836:Umbreon", wantOK: true},
		{name: "empty list renders empty", field: "deniedSpecs", want: "", wantOK: true},
		{name: "string list sorts", field: "prepackagedSpecListIds", want: "a,b", wantOK: true},
		// A zero value must still render, or a legitimately-zero field would be
		// indistinguishable from an unrenderable one.
		{name: "zero value still renders", field: "yearMinimum", want: "2002", wantOK: true},
		{name: "unknown field refuses", field: "isActive", want: "", wantOK: false},
		{name: "misspelled field refuses", field: "biddPercentage", want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RenderFieldValue(tt.field, fd)
			if ok != tt.wantOK {
				t.Fatalf("RenderFieldValue(%q) ok = %v, want %v", tt.field, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("RenderFieldValue(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

// Every field TranslateToDiff can emit must be renderable, or the push side
// would refuse a diff this side legitimately produced. TranslateToDiff itself
// fails closed on an unknown field, so this drives it with a campaign that
// differs on every axis and asserts it got that far.
func TestTranslateToDiffEmitsOnlyRenderableFields(t *testing.T) {
	internal := inventory.Campaign{
		BuyTermsCLPct:      0.75,
		DailySpendCapCents: 400000,
		GradeRange:         "9-10",
		YearRange:          "2020-2024",
		PriceRange:         "100-3000",
		CLConfidence:       "3-4",
		SubjectFilterMode:  "Target",
		Subjects:           []inventory.TargetSubject{{Name: "Pikachu"}},
		DeniedSpecs:        []inventory.TargetSubject{{ID: 4807, Name: "Charizard"}},
		TargetLanguages:    []string{"english"},
	}
	d, err := TranslateToDiff(internal, PortalCampaign{}, englishResolver())
	if err != nil {
		t.Fatalf("TranslateToDiff: %v", err)
	}
	if len(d.Changes) == 0 {
		t.Fatal("expected changes across every axis, got none")
	}
	for _, ch := range d.Changes {
		if _, ok := RenderFieldValue(ch.Field, CampaignFormData{}); !ok {
			t.Errorf("TranslateToDiff emits %q, which RenderFieldValue cannot render", ch.Field)
		}
	}
}

// PortalFormData is where the list endpoint's cents meet the edit form's whole
// dollars. A divergence here reads as staleness on every money field, so the
// conversion is pinned rather than left to inspection.
func TestPortalFormDataNormalizesMoneyToWholeUSD(t *testing.T) {
	fd := PortalFormData(PortalCampaign{
		DailyBudgetCents: 300000,
		BuyBox: CampaignBuyBox{
			PriceMinCents:     50000,
			PriceMaxCents:     300000,
			BuyerFlatFeeCents: 300,
		},
	})
	tests := []struct {
		field string
		want  string
	}{
		{field: "dailyBudget", want: "3000"},
		{field: "priceMinimum", want: "500"},
		{field: "priceMaximum", want: "3000"},
		{field: "flatFee", want: "3"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if got, _ := RenderFieldValue(tt.field, fd); got != tt.want {
				t.Errorf("%s = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}
