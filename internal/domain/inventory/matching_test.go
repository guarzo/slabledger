package inventory

import "testing"

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
		name     string
		in       MatchInput
		campaign Campaign
		want     bool
	}{
		{
			name:     "no filters set - matches anything",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{},
			want:     true,
		},
		{
			name:     "grade in range",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{GradeRange: "9-10"},
			want:     true,
		},
		{
			name:     "grade out of range",
			in:       MatchInput{Grade: 7, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{GradeRange: "9-10"},
			want:     false,
		},
		{
			name:     "half-grade 9.5 in range 9-10",
			in:       MatchInput{Grade: 9.5, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{GradeRange: "9-10"},
			want:     true,
		},
		{
			name:     "price in range",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{PriceRange: "50-500"},
			want:     true,
		},
		{
			name:     "price below range",
			in:       MatchInput{Grade: 9, BuyCostCents: 2000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{PriceRange: "50-500"},
			want:     false,
		},
		{
			name:     "price range scaled by buy terms - in range",
			in:       MatchInput{Grade: 10, BuyCostCents: 19799, CardName: "Umbreon EX", SetName: "Pokemon"},
			campaign: Campaign{PriceRange: "200-500", BuyTermsCLPct: 0.78},
			want:     true, // effective range: $156-$390
		},
		{
			name:     "price range scaled by buy terms - below range",
			in:       MatchInput{Grade: 10, BuyCostCents: 10000, CardName: "Umbreon EX", SetName: "Pokemon"},
			campaign: Campaign{PriceRange: "200-500", BuyTermsCLPct: 0.78},
			want:     false,
		},
		{
			name:     "malformed grade range rejects match",
			in:       MatchInput{Grade: 7, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{GradeRange: "bad"},
			want:     false,
		},
		{
			name:     "cardYear inside campaign year range",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set", CardYear: 2000},
			campaign: Campaign{YearRange: "1999-2003"},
			want:     true,
		},
		{
			name:     "cardYear outside campaign year range",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Vivid Voltage", CardYear: 2020},
			campaign: Campaign{YearRange: "1999-2003"},
			want:     false,
		},
		{
			name: "language axis rejects mismatched set",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "SWSH BLACK STAR PROMO"},
			campaign: Campaign{
				TargetLanguages: []string{"japanese"},
			},
			want: false,
		},
		{
			name: "language axis accepts matching set",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "JAPANESE M1S-MEGA SYMPHONIA"},
			campaign: Campaign{
				TargetLanguages: []string{"japanese"},
			},
			want: true,
		},
		{
			// The shape of all six live campaigns: BOTH curated spec lists are
			// selected on the portal. The single-token model could not express
			// this, so it rejected half of what these campaigns actually buy.
			name: "both languages selected accepts an english printing",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "SWSH BLACK STAR PROMO"},
			campaign: Campaign{
				TargetLanguages: []string{"english", "japanese"},
			},
			want: true,
		},
		{
			name: "both languages selected accepts a japanese printing",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "JAPANESE M1S-MEGA SYMPHONIA"},
			campaign: Campaign{
				TargetLanguages: []string{"english", "japanese"},
			},
			want: true,
		},
		{
			// A set is still a closed net, not a wildcard: chinese is in neither
			// token, so it must not match even with both tokens selected.
			name: "both languages selected still rejects a chinese printing",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Pikachu", SetName: "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1"},
			campaign: Campaign{
				TargetLanguages: []string{"english", "japanese"},
			},
			want: false,
		},
		{
			name: "subject axis Target mode - matches",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterTarget,
				Subjects:          []TargetSubject{{ID: 100, Name: "Charizard"}},
			},
			want: true,
		},
		{
			name: "subject axis Target mode - no match",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Blastoise", SetName: "Jungle"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterTarget,
				Subjects:          []TargetSubject{{ID: 100, Name: "Charizard"}},
			},
			want: false,
		},
		{
			name: "subject axis Exclude mode - excluded card rejected",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterExclude,
				Subjects:          []TargetSubject{{ID: 100, Name: "Charizard"}},
			},
			want: false,
		},
		{
			name: "subject axis Exclude mode - other card accepted",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Pikachu VMAX", SetName: "Vivid Voltage"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterExclude,
				Subjects:          []TargetSubject{{ID: 100, Name: "Charizard"}},
			},
			want: true,
		},
		{
			name: "empty subjects is an open net regardless of mode",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Anything", SetName: "Any Set"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterExclude,
			},
			want: true,
		},
		{
			name: "denied spec by PSASpecID overrides a subject match",
			in: MatchInput{
				Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set", CardNumber: "004", PSASpecID: 4807,
			},
			campaign: Campaign{
				Subjects:    []TargetSubject{{ID: 100, Name: "Charizard"}},
				DeniedSpecs: []TargetSubject{{ID: 4807, Name: "Base Set 004"}},
			},
			want: false,
		},
		{
			name: "denied spec by set+number fallback when PSASpecID is 0",
			in: MatchInput{
				Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set", CardNumber: "004",
			},
			campaign: Campaign{
				Subjects:    []TargetSubject{{ID: 100, Name: "Charizard"}},
				DeniedSpecs: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			},
			want: false,
		},
		{
			name: "no deny when neither identity is available (card number missing)",
			in: MatchInput{
				Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set",
			},
			campaign: Campaign{
				Subjects:    []TargetSubject{{ID: 100, Name: "Charizard"}},
				DeniedSpecs: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			},
			want: true, // fail-open: PSASpecID/ID both 0, and CardNumber is empty so no composite key can be built
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PurchaseMatchesCampaign(tt.in, &tt.campaign)
			if got != tt.want {
				t.Errorf("PurchaseMatchesCampaign() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLanguageAxisMatches(t *testing.T) {
	tests := []struct {
		name            string
		setName         string
		targetLanguages []string
		want            bool
	}{
		{"nil set is open net", "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", nil, true},
		{"empty set is open net", "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", []string{}, true},
		{"japanese set matches japanese-only set", "JAPANESE M1S-MEGA SYMPHONIA", []string{"japanese"}, true},
		{"chinese set does not match japanese-only set", "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", []string{"japanese"}, false},
		{"english set matches english-only set", "SWSH BLACK STAR PROMO", []string{"english"}, true},
		{"english set matches a both-languages set", "SWSH BLACK STAR PROMO", []string{"english", "japanese"}, true},
		{"japanese set matches a both-languages set", "JAPANESE M1S-MEGA SYMPHONIA", []string{"english", "japanese"}, true},
		// The set is unordered: reversing the tokens must not change the answer.
		{"membership is order-insensitive", "JAPANESE M1S-MEGA SYMPHONIA", []string{"japanese", "english"}, true},
		{"korean set matches neither token", "KOREAN S1-SWORD SHIELD", []string{"english", "japanese"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LanguageAxisMatches(tt.setName, tt.targetLanguages); got != tt.want {
				t.Errorf("LanguageAxisMatches(%q, %v) = %v, want %v", tt.setName, tt.targetLanguages, got, tt.want)
			}
		})
	}
}

func TestSubjectAxisMatches(t *testing.T) {
	tests := []struct {
		name     string
		cardName string
		subjects []TargetSubject
		mode     string
		want     bool
	}{
		{"empty list is open net in Target mode", "Charizard", nil, SubjectFilterTarget, true},
		{"empty list is open net in Exclude mode", "Charizard", nil, SubjectFilterExclude, true},
		{"Target mode matches", "Charizard VMAX", []TargetSubject{{ID: 1, Name: "Charizard"}}, SubjectFilterTarget, true},
		{"Target mode no match", "Blastoise", []TargetSubject{{ID: 1, Name: "Charizard"}}, SubjectFilterTarget, false},
		{"Exclude mode rejects listed", "Charizard VMAX", []TargetSubject{{ID: 1, Name: "Charizard"}}, SubjectFilterExclude, false},
		{"Exclude mode accepts unlisted", "Pikachu", []TargetSubject{{ID: 1, Name: "Charizard"}}, SubjectFilterExclude, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SubjectAxisMatches(tt.cardName, tt.subjects, tt.mode); got != tt.want {
				t.Errorf("SubjectAxisMatches(%q, %v, %q) = %v, want %v", tt.cardName, tt.subjects, tt.mode, got, tt.want)
			}
		})
	}
}

func TestSpecDenied(t *testing.T) {
	tests := []struct {
		name   string
		in     MatchInput
		denied []TargetSubject
		want   bool
	}{
		{
			name:   "denied by matching PSASpecID",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004", PSASpecID: 4807},
			denied: []TargetSubject{{ID: 4807, Name: "Base Set 004"}},
			want:   true,
		},
		{
			name:   "not denied when PSASpecID differs",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004", PSASpecID: 100},
			denied: []TargetSubject{{ID: 4807, Name: "Base Set 004"}},
			want:   false,
		},
		{
			name:   "falls back to set+number composite when PSASpecID is 0",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004"},
			denied: []TargetSubject{{ID: 4807, Name: "Base Set 004"}},
			want:   true,
		},
		{
			name:   "falls back to set+number composite when denied entry id is 0",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004", PSASpecID: 999},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			want:   true,
		},
		{
			name:   "composite comparison is case-insensitive",
			in:     MatchInput{SetName: "base set", CardNumber: "004"},
			denied: []TargetSubject{{ID: 0, Name: "BASE SET 004"}},
			want:   true,
		},
		{
			name:   "no match when card number differs",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004"},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 005"}},
			want:   false,
		},
		{
			name:   "fail-open when card number is missing (no composite key can be built)",
			in:     MatchInput{SetName: "Base Set"},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			want:   false,
		},
		{
			name:   "fail-open when set name is missing (no composite key can be built)",
			in:     MatchInput{CardNumber: "004"},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			want:   false,
		},
		{
			name:   "empty deny list never denies",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004", PSASpecID: 4807},
			denied: nil,
			want:   false,
		},
		{
			name:   "regression: English deny entry does not over-deny a Japanese printing",
			in:     MatchInput{SetName: "JAPANESE BASE SET", CardNumber: "004"},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			want:   false,
		},
		{
			name:   "regression: Japanese deny entry does not over-deny an English printing",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004"},
			denied: []TargetSubject{{ID: 0, Name: "JAPANESE BASE SET 004"}},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpecDenied(tt.in, tt.denied); got != tt.want {
				t.Errorf("SpecDenied(%+v, %v) = %v, want %v", tt.in, tt.denied, got, tt.want)
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
		ID:                "campaign-c",
		Name:              "Pokemon Only",
		SubjectFilterMode: SubjectFilterTarget,
		Subjects:          []TargetSubject{{ID: 1, Name: "Charizard"}, {ID: 2, Name: "Pikachu"}, {ID: 3, Name: "Mewtwo"}},
	}
	campaignNoFilters := Campaign{
		ID:   "campaign-none",
		Name: "No Filters",
	}

	t.Run("single match by grade and price", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 9.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignA, campaignB},
		)
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-a" {
			t.Errorf("expected campaign-a, got %s", result.CampaignID)
		}
	})

	t.Run("single match to campaign B", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 7.0, BuyCostCents: 5000, CardName: "Blastoise", SetName: "Base Set"},
			[]Campaign{campaignA, campaignB},
		)
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-b" {
			t.Errorf("expected campaign-b, got %s", result.CampaignID)
		}
	})

	t.Run("no match", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 5.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignA, campaignB},
		)
		if result.Status != "unmatched" {
			t.Fatalf("expected unmatched, got %s", result.Status)
		}
	})

	t.Run("ambiguous match", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 9.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignA, campaignC},
		)
		if result.Status != "ambiguous" {
			t.Fatalf("expected ambiguous, got %s", result.Status)
		}
		if len(result.Candidates) != 2 {
			t.Errorf("expected 2 candidates, got %d", len(result.Candidates))
		}
	})

	t.Run("campaign with no filters matches everything", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 9.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignNoFilters},
		)
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-none" {
			t.Errorf("expected campaign-none, got %s", result.CampaignID)
		}
	})

	t.Run("empty campaign list", func(t *testing.T) {
		result := FindMatchingCampaign(MatchInput{Grade: 9.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"}, nil)
		if result.Status != "unmatched" {
			t.Fatalf("expected unmatched, got %s", result.Status)
		}
	})

	t.Run("half-grade 9.5 matches range 9-10", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 9.5, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignA},
		)
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-a" {
			t.Errorf("expected campaign-a, got %s", result.CampaignID)
		}
	})
}
