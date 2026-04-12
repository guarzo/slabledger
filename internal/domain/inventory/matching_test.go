package inventory

import (
	"testing"
)

func TestParseRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin int
		wantMax int
		wantOK  bool
	}{
		{"empty", "", 0, 0, false},
		{"valid grade range", "9-10", 9, 10, true},
		{"valid price range", "50-500", 50, 500, true},
		{"single value", "10-10", 10, 10, true},
		{"inverted", "10-5", 0, 0, false},
		{"no dash", "910", 0, 0, false},
		{"non-numeric", "abc-def", 0, 0, false},
		{"whitespace", " 9 - 10 ", 9, 10, true},
		{"zero based", "0-100", 0, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lo, hi, ok := ParseRange(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ParseRange(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && (lo != tt.wantMin || hi != tt.wantMax) {
				t.Errorf("ParseRange(%q) = (%d, %d), want (%d, %d)", tt.input, lo, hi, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestPurchaseMatchesCampaign(t *testing.T) {
	tests := []struct {
		name         string
		grade        float64
		buyCostCents int
		cardName     string
		setName      string
		campaign     Campaign
		want         bool
	}{
		{
			name:         "no filters set - matches anything",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{},
			want:         true,
		},
		{
			name:         "grade in range",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{GradeRange: "9-10"},
			want:         true,
		},
		{
			name:         "grade out of range",
			grade:        7,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{GradeRange: "9-10"},
			want:         false,
		},
		{
			name:         "grade at lower boundary",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{GradeRange: "9-10"},
			want:         true,
		},
		{
			name:         "grade at upper boundary",
			grade:        10,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{GradeRange: "9-10"},
			want:         true,
		},
		{
			name:         "price in range",
			grade:        9,
			buyCostCents: 15000, // $150
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{PriceRange: "50-500"},
			want:         true,
		},
		{
			name:         "price below range",
			grade:        9,
			buyCostCents: 2000, // $20
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{PriceRange: "50-500"},
			want:         false,
		},
		{
			name:         "price above range",
			grade:        9,
			buyCostCents: 60000, // $600
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{PriceRange: "50-500"},
			want:         false,
		},
		{
			name:         "price at lower boundary",
			grade:        9,
			buyCostCents: 5000, // $50 exactly
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{PriceRange: "50-500"},
			want:         true,
		},
		{
			name:         "price at upper boundary",
			grade:        9,
			buyCostCents: 50000, // $500 exactly
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{PriceRange: "50-500"},
			want:         true,
		},
		{
			name:         "inclusion list matches card name",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard VMAX PSA 9",
			setName:      "Vivid Voltage",
			campaign:     Campaign{InclusionList: "Charizard,Pikachu"},
			want:         true,
		},
		{
			name:         "inclusion list matches set name",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Some Card",
			setName:      "Base Set",
			campaign:     Campaign{InclusionList: "Base Set"},
			want:         true,
		},
		{
			name:         "inclusion list no match",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Blastoise",
			setName:      "Jungle",
			campaign:     Campaign{InclusionList: "Charizard,Pikachu"},
			want:         false,
		},
		{
			name:         "inclusion list case insensitive",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "charizard vmax",
			setName:      "Base Set",
			campaign:     Campaign{InclusionList: "CHARIZARD"},
			want:         true,
		},
		{
			name:         "exclusion mode - card matches exclusion list",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard VMAX",
			setName:      "Base Set",
			campaign:     Campaign{InclusionList: "Charizard", ExclusionMode: true},
			want:         false,
		},
		{
			name:         "exclusion mode - card does not match exclusion list",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Pikachu VMAX",
			setName:      "Vivid Voltage",
			campaign:     Campaign{InclusionList: "Charizard", ExclusionMode: true},
			want:         true,
		},
		{
			name:         "all criteria match",
			grade:        10,
			buyCostCents: 25000, // $250
			cardName:     "Charizard VMAX",
			setName:      "Base Set",
			campaign: Campaign{
				GradeRange:    "9-10",
				PriceRange:    "100-500",
				InclusionList: "Charizard",
			},
			want: true,
		},
		{
			name:         "grade matches but price fails",
			grade:        10,
			buyCostCents: 8000, // $80
			cardName:     "Charizard VMAX",
			setName:      "Base Set",
			campaign: Campaign{
				GradeRange:    "9-10",
				PriceRange:    "100-500",
				InclusionList: "Charizard",
			},
			want: false,
		},
		{
			name:         "malformed grade range rejects match",
			grade:        7,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{GradeRange: "bad"},
			want:         false,
		},
		{
			name:         "malformed price range rejects match",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{PriceRange: "bad"},
			want:         false,
		},
		{
			name:         "empty entries in inclusion list are skipped",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{InclusionList: ",,Charizard,,"},
			want:         true,
		},
		{
			name:         "price range scaled by buy terms - in range",
			grade:        10,
			buyCostCents: 19799, // $197.99 paid, market value ~$253.83 at 78%
			cardName:     "Umbreon EX",
			setName:      "Pokemon",
			campaign:     Campaign{PriceRange: "200-500", BuyTermsCLPct: 0.78},
			want:         true, // effective range: $156-$390
		},
		{
			name:         "price range scaled by buy terms - below range",
			grade:        10,
			buyCostCents: 10000, // $100 paid
			cardName:     "Umbreon EX",
			setName:      "Pokemon",
			campaign:     Campaign{PriceRange: "200-500", BuyTermsCLPct: 0.78},
			want:         false, // effective range: $156-$390, $100 < $156
		},
		{
			name:         "price range with zero buy terms defaults to 1",
			grade:        9,
			buyCostCents: 25000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{PriceRange: "50-500", BuyTermsCLPct: 0},
			want:         true,
		},
		{
			name:         "space-separated inclusion list matches",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard VMAX PSA 9",
			setName:      "Vivid Voltage",
			campaign:     Campaign{InclusionList: "pikachu charizard mewtwo"},
			want:         true,
		},
		{
			name:         "space-separated inclusion list no match",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Blastoise",
			setName:      "Jungle",
			campaign:     Campaign{InclusionList: "pikachu charizard mewtwo"},
			want:         false,
		},
		{
			name:         "mixed comma and space separated inclusion list",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Umbreon VMAX",
			setName:      "Evolving Skies",
			campaign:     Campaign{InclusionList: "pikachu charizard,umbreon mewtwo"},
			want:         true,
		},
		{
			name:         "half-grade 9.5 in range 9-10",
			grade:        9.5,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{GradeRange: "9-10"},
			want:         true,
		},
		{
			name:         "half-grade 8.5 below range 9-10",
			grade:        8.5,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			campaign:     Campaign{GradeRange: "9-10"},
			want:         false,
		},
	}

	// Add year-range test cases with non-zero cardYear
	yearTests := []struct {
		name         string
		grade        float64
		buyCostCents int
		cardName     string
		setName      string
		cardYear     int
		campaign     Campaign
		want         bool
	}{
		{
			name:         "cardYear inside campaign year range",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			cardYear:     2000,
			campaign:     Campaign{YearRange: "1999-2003"},
			want:         true,
		},
		{
			name:         "cardYear outside campaign year range",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Vivid Voltage",
			cardYear:     2020,
			campaign:     Campaign{YearRange: "1999-2003"},
			want:         false,
		},
		{
			name:         "cardYear at lower boundary",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			cardYear:     1999,
			campaign:     Campaign{YearRange: "1999-2003"},
			want:         true,
		},
		{
			name:         "cardYear at upper boundary",
			grade:        9,
			buyCostCents: 15000,
			cardName:     "Charizard",
			setName:      "Base Set",
			cardYear:     2003,
			campaign:     Campaign{YearRange: "1999-2003"},
			want:         true,
		},
	}

	for _, tt := range yearTests {
		t.Run(tt.name, func(t *testing.T) {
			got := PurchaseMatchesCampaign(tt.grade, tt.buyCostCents, tt.cardName, tt.setName, tt.cardYear, &tt.campaign)
			if got != tt.want {
				t.Errorf("PurchaseMatchesCampaign() = %v, want %v", got, tt.want)
			}
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PurchaseMatchesCampaign(tt.grade, tt.buyCostCents, tt.cardName, tt.setName, 0, &tt.campaign)
			if got != tt.want {
				t.Errorf("PurchaseMatchesCampaign() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindMatchingCampaign(t *testing.T) {
	campaignA := Campaign{
		ID:         "campaign-a",
		Name:       "High Grade",
		GradeRange: "9-10",
		PriceRange: "50-500",
	}
	campaignB := Campaign{
		ID:         "campaign-b",
		Name:       "Low Grade",
		GradeRange: "7-8",
		PriceRange: "10-100",
	}
	campaignC := Campaign{
		ID:            "campaign-c",
		Name:          "Pokemon Only",
		InclusionList: "Charizard,Pikachu,Mewtwo",
	}
	campaignNoFilters := Campaign{
		ID:   "campaign-none",
		Name: "No Filters",
	}

	t.Run("single match by grade and price", func(t *testing.T) {
		result := FindMatchingCampaign(9.0, 15000, "Charizard", "Base Set", 0, []Campaign{campaignA, campaignB})
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-a" {
			t.Errorf("expected campaign-a, got %s", result.CampaignID)
		}
	})

	t.Run("single match to campaign B", func(t *testing.T) {
		result := FindMatchingCampaign(7.0, 5000, "Blastoise", "Base Set", 0, []Campaign{campaignA, campaignB})
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-b" {
			t.Errorf("expected campaign-b, got %s", result.CampaignID)
		}
	})

	t.Run("no match", func(t *testing.T) {
		// Grade 5 doesn't match either campaign
		result := FindMatchingCampaign(5.0, 15000, "Charizard", "Base Set", 0, []Campaign{campaignA, campaignB})
		if result.Status != "unmatched" {
			t.Fatalf("expected unmatched, got %s", result.Status)
		}
	})

	t.Run("ambiguous match", func(t *testing.T) {
		// Grade 9, Charizard matches both campaignA (grade+price) and campaignC (inclusion list)
		result := FindMatchingCampaign(9.0, 15000, "Charizard", "Base Set", 0, []Campaign{campaignA, campaignC})
		if result.Status != "ambiguous" {
			t.Fatalf("expected ambiguous, got %s", result.Status)
		}
		if len(result.Candidates) != 2 {
			t.Errorf("expected 2 candidates, got %d", len(result.Candidates))
		}
	})

	t.Run("campaign with no filters matches everything", func(t *testing.T) {
		result := FindMatchingCampaign(9.0, 15000, "Charizard", "Base Set", 0, []Campaign{campaignNoFilters})
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-none" {
			t.Errorf("expected campaign-none, got %s", result.CampaignID)
		}
	})

	t.Run("no-filter campaign causes ambiguity with specific campaign", func(t *testing.T) {
		result := FindMatchingCampaign(9.0, 15000, "Charizard", "Base Set", 0, []Campaign{campaignA, campaignNoFilters})
		if result.Status != "ambiguous" {
			t.Fatalf("expected ambiguous, got %s", result.Status)
		}
	})

	t.Run("empty campaign list", func(t *testing.T) {
		result := FindMatchingCampaign(9.0, 15000, "Charizard", "Base Set", 0, nil)
		if result.Status != "unmatched" {
			t.Fatalf("expected unmatched, got %s", result.Status)
		}
	})

	t.Run("half-grade 9.5 matches range 9-10", func(t *testing.T) {
		result := FindMatchingCampaign(9.5, 15000, "Charizard", "Base Set", 0, []Campaign{campaignA})
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-a" {
			t.Errorf("expected campaign-a, got %s", result.CampaignID)
		}
	})
}
