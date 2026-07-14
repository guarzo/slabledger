package psacampaign

import (
	"strings"
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

func TestTranslateToCreate(t *testing.T) {
	base := inventory.Campaign{
		Name: "Modern 10s", BuyTermsCLPct: 0.72, DailySpendCapCents: 300000,
		GradeRange: "10", YearRange: "2024-2026", PriceRange: "500-3000",
		CLConfidence: "3-4", PSASourcingFeeCents: 300,
	}
	tests := []struct {
		name    string
		mutate  func(c *inventory.Campaign)
		wantErr string
		check   func(t *testing.T, fd CampaignFormData)
	}{
		{
			name:   "full mapping, born paused",
			mutate: func(c *inventory.Campaign) {},
			check: func(t *testing.T, fd CampaignFormData) {
				if fd.IsActive {
					t.Fatal("create must be born paused (isActive=false)")
				}
				if fd.CampaignName != "Modern 10s" || fd.CampaignType != "CATEGORY" || fd.Category != "POKEMON" {
					t.Fatalf("identity fields wrong: %+v", fd)
				}
				if fd.BidPercentage != 72 {
					t.Fatalf("BidPercentage = %d, want 72", fd.BidPercentage)
				}
				if fd.DailyBudget != 3000 {
					t.Fatalf("DailyBudget = %d, want 3000 (whole USD)", fd.DailyBudget)
				}
				if fd.FlatFee != 3 {
					t.Fatalf("FlatFee = %d, want 3 (PSASourcingFeeCents/100)", fd.FlatFee)
				}
				if fd.DailySpecLimit != 2 {
					t.Fatalf("DailySpecLimit = %d, want default 2", fd.DailySpecLimit)
				}
				if fd.GradeMinimum != "10" || fd.GradeMaximum != "10" {
					t.Fatalf("grades = %q/%q, want \"10\"/\"10\"", fd.GradeMinimum, fd.GradeMaximum)
				}
				if fd.YearMinimum != 2024 || fd.YearMaximum != 2026 {
					t.Fatalf("years = %d/%d", fd.YearMinimum, fd.YearMaximum)
				}
				if fd.PriceMinimum != 500 || fd.PriceMaximum != 3000 {
					t.Fatalf("prices = %d/%d", fd.PriceMinimum, fd.PriceMaximum)
				}
				if fd.CardLadderConfidenceMinimum != 3 {
					t.Fatalf("clConf = %d, want 3", fd.CardLadderConfidenceMinimum)
				}
				if fd.PublisherFilterType != "Target" || fd.SubjectFilterType != "Target" {
					t.Fatalf("filter types wrong: %+v", fd)
				}
				// Wire format requires [] not null for the list fields.
				if fd.PrepackagedSpecListIDs == nil || fd.SelectedPublishers == nil || fd.SelectedSubjects == nil || fd.DeniedSpecs == nil {
					t.Fatal("list fields must be non-nil empty slices (JSON [] not null)")
				}
			},
		},
		{
			name:   "decimal cl confidence truncates",
			mutate: func(c *inventory.Campaign) { c.CLConfidence = "2.5-4" },
			check: func(t *testing.T, fd CampaignFormData) {
				if fd.CardLadderConfidenceMinimum != 2 {
					t.Fatalf("clConf = %d, want 2 (truncated from 2.5)", fd.CardLadderConfidenceMinimum)
				}
			},
		},
		{
			name: "non-multiple-of-100 cents round to nearest USD",
			mutate: func(c *inventory.Campaign) {
				c.PSASourcingFeeCents = 350   // $3.50 -> $4
				c.DailySpendCapCents = 299900 // $2999.00 -> $2999
			},
			check: func(t *testing.T, fd CampaignFormData) {
				if fd.FlatFee != 4 {
					t.Fatalf("FlatFee = %d, want 4 (350c rounds up)", fd.FlatFee)
				}
				if fd.DailyBudget != 2999 {
					t.Fatalf("DailyBudget = %d, want 2999", fd.DailyBudget)
				}
			},
		},
		{name: "empty grade range", mutate: func(c *inventory.Campaign) { c.GradeRange = "" }, wantErr: "grade range"},
		{name: "empty year range", mutate: func(c *inventory.Campaign) { c.YearRange = "" }, wantErr: "year range"},
		{name: "non-numeric year", mutate: func(c *inventory.Campaign) { c.YearRange = "vintage-2020" }, wantErr: "year range"},
		{name: "empty price range", mutate: func(c *inventory.Campaign) { c.PriceRange = "" }, wantErr: "price range"},
		{name: "empty CL confidence", mutate: func(c *inventory.Campaign) { c.CLConfidence = "" }, wantErr: "cl confidence"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			tt.mutate(&c)
			fd, err := TranslateToCreate(c)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("TranslateToCreate: %v", err)
			}
			tt.check(t, fd)
		})
	}
}
