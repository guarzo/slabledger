package pricecharting

import "testing"

func TestBuildQuery(t *testing.T) {
	tests := []struct {
		name     string
		setName  string
		cardName string
		number   string
		expected string
	}{
		{
			name:     "basic query",
			setName:  "Surging Sparks",
			cardName: "Pikachu ex",
			number:   "250",
			expected: "pokemon Surging Sparks Pikachu ex #250",
		},
		{
			name:     "no number",
			setName:  "Surging Sparks",
			cardName: "Pikachu ex",
			number:   "",
			expected: "pokemon Surging Sparks Pikachu ex",
		},
		{
			name:     "normalized set name (SWSH)",
			setName:  "SWSH123",
			cardName: "Charizard",
			number:   "1",
			expected: "pokemon SWSH123 Charizard #1",
		},
		{
			name:     "set with special chars",
			setName:  "Sword & Shield: Base",
			cardName: "Zacian V",
			number:   "138",
			expected: "pokemon Sword and Shield Base Zacian V #138",
		},
		{
			name:     "PSA category format",
			setName:  "2013 Pokemon Black & White Promos",
			cardName: "UMBREON-HOLO",
			number:   "BW93",
			expected: "pokemon Black and White Promos UMBREON HOLO #BW93",
		},
		{
			name:     "CL SVP promo format",
			setName:  "Pokemon Svp En-Sv Black Star Promo",
			cardName: "Umbreon Ex",
			number:   "176",
			expected: "pokemon SVP Black Star Promos Umbreon Ex #176",
		},
		{
			name:     "CL Expedition with Rev.foil",
			setName:  "Pokemon Expedition",
			cardName: "Mewtwo-Rev.foil",
			number:   "56",
			expected: "pokemon Expedition Mewtwo Reverse Foil #56",
		},
		{
			name:     "SVP does not trigger SV expansion",
			setName:  "Svp Black Star Promos",
			cardName: "Pikachu",
			number:   "1",
			expected: "pokemon SVP Black Star Promos Pikachu #1",
		},
		{
			name:     "PSA listing title with embedded year, set, grade, number",
			setName:  "Pokemon Expedition",
			cardName: "2002 Pokemon Expedition Mewtwo-Rev.foil #56 PSA 9",
			number:   "56",
			expected: "pokemon Expedition Mewtwo Reverse Foil #56",
		},
		{
			name:     "PSA listing title for Umbreon ex (matching set)",
			setName:  "2025 Pokemon Scarlet Violet Prismatic Evolutions",
			cardName: "2025 Pokemon Scarlet Violet Prismatic Evolutions Umbreon Ex #176 PSA 10",
			number:   "176",
			expected: "pokemon Scarlet Violet Prismatic Evolutions Umbreon Ex #176",
		},
		{
			name:     "PSA listing title for Umbreon ex (generic set, fallback stripping)",
			setName:  "TCG Cards",
			cardName: "2025 Pokemon Scarlet Violet Prismatic Evolutions Umbreon Ex #176 PSA 10",
			number:   "176",
			expected: "pokemon TCG Cards Violet Prismatic Evolutions Umbreon Ex #176",
		},
		{
			name:     "gallery-style collector number TG16/TG30",
			setName:  "Silver Tempest",
			cardName: "Umbreon VMAX #TG16/TG30",
			number:   "TG16",
			expected: "pokemon Silver Tempest Umbreon VMAX #TG16",
		},
		{
			name:     "gallery-style collector number GG44/GG70",
			setName:  "Crown Zenith",
			cardName: "Pikachu VMAX #GG44/GG70",
			number:   "GG44",
			expected: "pokemon Crown Zenith Pikachu VMAX #GG44",
		},
		{
			name:     "PSA listing title with half grade",
			setName:  "Pokemon Expedition",
			cardName: "2002 Pokemon Expedition Mewtwo #56 PSA 9.5",
			number:   "56",
			expected: "pokemon Expedition Mewtwo #56",
		},
		{
			name:     "PSA listing title with alphanumeric card number",
			setName:  "2013 Pokemon Black & White Promos",
			cardName: "2013 Pokemon Black and White Promos Umbreon Holo #BW93 PSA 9",
			number:   "BW93",
			expected: "pokemon Black and White Promos Umbreon Holo #BW93",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildQuery(tt.setName, tt.cardName, tt.number)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestBuildQueryWithOptions(t *testing.T) {
	tests := []struct {
		name     string
		setName  string
		cardName string
		number   string
		options  QueryOptions
		expected string
	}{
		{
			name:     "no options",
			setName:  "Surging Sparks",
			cardName: "Pikachu",
			number:   "250",
			options:  QueryOptions{},
			expected: "pokemon Surging Sparks Pikachu #250",
		},
		{
			name:     "with variant",
			setName:  "Base Set",
			cardName: "Charizard",
			number:   "4",
			options:  QueryOptions{Variant: "1st Edition"},
			expected: "pokemon Base Set Charizard #4 1st edition",
		},
		{
			name:     "with japanese",
			setName:  "VMAX Climax",
			cardName: "Pikachu",
			number:   "1",
			options:  QueryOptions{Language: "Japanese"},
			expected: "pokemon VMAX Climax Pikachu #1 japanese",
		},
		{
			name:     "exact match",
			setName:  "Surging Sparks",
			cardName: "Pikachu ex",
			number:   "250",
			options:  QueryOptions{ExactMatch: true},
			expected: "\"pokemon Surging Sparks Pikachu ex #250\"",
		},
		{
			name:     "all options",
			setName:  "Base Set",
			cardName: "Blastoise",
			number:   "2",
			options: QueryOptions{
				Variant:   "Shadowless",
				Language:  "English",
				Condition: "Mint",
				Grader:    "PSA",
			},
			expected: "pokemon Base Set Blastoise #2 shadowless mint PSA",
		},
		{
			name:     "with condition and grader",
			setName:  "Crown Zenith",
			cardName: "Mewtwo",
			number:   "100",
			options: QueryOptions{
				Condition: "Near Mint",
				Grader:    "BGS",
			},
			expected: "pokemon Crown Zenith Mewtwo #100 near mint BGS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildQueryWithOptions(tt.setName, tt.cardName, tt.number, tt.options)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestNormalizeSetName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SWSH123", "SWSH123"},
		{"SM Base Set", "Sun Moon Base Set"},
		{"SV Scarlet", "Scarlet Violet Scarlet"},
		{"XYBase", "XYBase"},
		{"BW01", "BW01"},
		{"Base Set", "Base Set"},
		{"Sword-Shield", "Sword Shield"},
		{"Set: Name", "Set Name"},
		{"SIMPLIFIED CHINESE CBB2 C GEM PACK VOL 2", "Chinese GEM PACK VOL 2"},
		{"TRADITIONAL CHINESE CBB1 C GEM PACK VOL 1", "Chinese GEM PACK VOL 1"},
		{"CN CBB2 C GEM PACK VOL 2", "Chinese GEM PACK VOL 2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSetName(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestNormalizeVariant(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1st Edition", "1st edition"},
		{"First Edition", "1st edition"},
		{"Shadowless", "shadowless"},
		{"Reverse Holo", "reverse holo"},
		{"Reverse", "reverse holo"},
		{"Holo", "holo"},
		{"Staff Promo", "staff"},
		{"Prerelease", "prerelease"},
		{"Custom Variant", "Custom Variant"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeVariant(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Japanese", "japanese"},
		{"JP", "japanese"},
		{"Japan", "japanese"},
		{"Korean", "korean"},
		{"French", "french"},
		{"German", "german"},
		{"Spanish", "spanish"},
		{"Italian", "italian"},
		{"English", ""}, // English is default
		{"USA", ""},     // USA is English (default)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeLanguage(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestNormalizeRegion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Japan", "japanese"},
		{"Japanese", "japanese"},
		{"Europe", "european"},
		{"European", "european"},
		{"Korea", "korean"},
		{"USA", ""},     // USA is default
		{"English", ""}, // English is default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeRegion(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestNormalizeCondition(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Mint", "mint"},
		{"M", "mint"},
		{"Near Mint", "near mint"},
		{"NM", "near mint"},
		{"Excellent", "excellent"},
		{"EX", "excellent"},
		{"Good", "good"},
		{"Poor", "poor"},
		{"Graded", "graded"},
		{"Unknown", ""}, // Unknown condition returns empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeCondition(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestNormalizeGrader(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"PSA", "PSA"},
		{"psa", "PSA"},
		{"BGS", "BGS"},
		{"Beckett", "BGS"},
		{"CGC", "CGC"},
		{"SGC", "SGC"},
		{"Unknown", ""}, // Unknown grader returns empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeGrader(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestBuildAdvancedQuery(t *testing.T) {
	// Create a PriceCharting instance for testing
	pc, err := NewPriceCharting(DefaultConfig("test-token"), nil, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}

	tests := []struct {
		name     string
		setName  string
		cardName string
		number   string
		options  QueryOptions
		expected string
	}{
		{
			name:     "no options",
			setName:  "Surging Sparks",
			cardName: "Pikachu",
			number:   "250",
			options:  QueryOptions{},
			expected: "pokemon Surging Sparks Pikachu #250",
		},
		{
			name:     "with variant",
			setName:  "Base Set",
			cardName: "Charizard",
			number:   "4",
			options:  QueryOptions{Variant: "1st Edition"},
			expected: "pokemon Base Set Charizard #4 1st edition",
		},
		{
			name:     "with language",
			setName:  "VMAX Climax",
			cardName: "Charizard",
			number:   "3",
			options:  QueryOptions{Language: "Japanese"},
			expected: "pokemon VMAX Climax Charizard #3 japanese",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pc.BuildAdvancedQuery(tt.setName, tt.cardName, tt.number, tt.options)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
