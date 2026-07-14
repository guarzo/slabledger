package psacampaign

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestTranslateToDiff_ScalarFields(t *testing.T) {
	internal := inventory.Campaign{
		BuyTermsCLPct:      0.75, // 75%
		DailySpendCapCents: 400000,
		GradeRange:         "9-10",
		YearRange:          "2020-2024",
		PriceRange:         "100-3000", // dollars
		CLConfidence:       "3-4",
	}
	portal := PortalCampaign{
		BuyPercentClv:    70,
		DailyBudgetCents: 300000,
		BuyBox: CampaignBuyBox{
			GradeMin: "10", GradeMax: "10", YearMin: 2002, YearMax: 2003,
			PriceMinCents: 50000, PriceMaxCents: 200000, ClvConfidenceMin: 2,
		},
	}
	diff, err := TranslateToDiff(internal, portal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[string]FieldChange{}
	for _, c := range diff.Changes {
		got[c.Field] = c
	}
	if c := got["bidPercentage"]; c.Old != "70" || c.New != "75" {
		t.Errorf("bidPercentage = %+v, want old 70 new 75", c)
	}
	if c := got["gradeMinimum"]; c.New != "9" {
		t.Errorf("gradeMinimum new = %q, want 9", c.New)
	}
	if c := got["priceMaximum"]; c.New != "3000" {
		t.Errorf("priceMaximum new = %q, want 3000 (dollars)", c.New)
	}
	if _, ok := got["yearMinimum"]; !ok {
		t.Error("expected yearMinimum change (2002 -> 2020)")
	}
}
