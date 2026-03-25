package cardutil

import "testing"

func TestNormalizeCardName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name unchanged",
			input:    "Pikachu",
			expected: "Pikachu",
		},
		{
			name:     "removes bracket modifier",
			input:    "Pikachu [Reverse Holo]",
			expected: "Pikachu",
		},
		{
			name:     "removes jumbo modifier",
			input:    "Charizard [Jumbo]",
			expected: "Charizard",
		},
		{
			name:     "removes cosmos holo modifier",
			input:    "Moltres [Cosmos Holo]",
			expected: "Moltres",
		},
		{
			name:     "removes card number suffix",
			input:    "Pikachu #30",
			expected: "Pikachu",
		},
		{
			name:     "removes card number with set size",
			input:    "Charizard #139/195",
			expected: "Charizard",
		},
		{
			name:     "removes both bracket and number",
			input:    "Moltres [Reverse Holo] #30",
			expected: "Moltres",
		},
		{
			name:     "handles multiple brackets",
			input:    "Pikachu [Reverse Holo] [Special]",
			expected: "Pikachu",
		},
		{
			name:     "normalizes whitespace",
			input:    "  Pikachu   VMAX  ",
			expected: "Pikachu VMAX",
		},
		{
			name:     "preserves multi-word names",
			input:    "Charizard ex",
			expected: "Charizard ex",
		},
		{
			name:     "complex name with bracket",
			input:    "Mewtwo & Mew GX [Holo]",
			expected: "Mewtwo & Mew GX",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
		{
			name:     "number in middle preserved",
			input:    "Pikachu V 25",
			expected: "Pikachu V 25",
		},
		{
			name:     "japanese denominator with dash",
			input:    "Rayquaza Spirit Link #126/XY-P",
			expected: "Rayquaza Spirit Link",
		},
		{
			name:     "japanese promo with short number",
			input:    "Gardevoir ex #92/XY-P",
			expected: "Gardevoir ex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeCardName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeCardName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBracketModifierPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"[Reverse Holo]", " "},
		{"text [Jumbo] more", "text more"},
		{"[First] [Second]", "  "},
		{"no brackets", "no brackets"},
	}

	for _, tt := range tests {
		result := bracketModifierRegex.ReplaceAllString(tt.input, " ")
		if result != tt.expected {
			t.Errorf("bracketModifierRegex on %q = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCardNumberPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Card #30", "Card"},
		{"Card #139/195", "Card"},
		{"Card #201", "Card"},
		{"Card #126/XY-P", "Card"}, // Japanese-style denominator with dash
		{"Card #92/XY-P", "Card"},  // Japanese-style denominator with dash
		{"Card #SM162", "Card"},    // Era-prefixed number
		{"Card #126/S-P", "Card"},  // Short Japanese denominator
		{"Card 30", "Card 30"},     // No # prefix, not matched
		{"#30 Card", "#30 Card"},   // Not at end, not matched
	}

	for _, tt := range tests {
		result := cardNumberRegex.ReplaceAllString(tt.input, "")
		if result != tt.expected {
			t.Errorf("cardNumberRegex on %q = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractCollectorNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple number",
			input:    "Charizard ex #199",
			expected: "199",
		},
		{
			name:     "number with set total",
			input:    "Charizard #139/195",
			expected: "139",
		},
		{
			name:     "galarian gallery number",
			input:    "Zeraora VMAX #GG42",
			expected: "GG42",
		},
		{
			name:     "number with bracket modifier",
			input:    "Moltres [Reverse Holo] #30",
			expected: "30",
		},
		{
			name:     "japanese denominator with dash",
			input:    "Rayquaza Spirit Link #126/XY-P",
			expected: "126",
		},
		{
			name:     "era-prefixed number",
			input:    "Pikachu Spirit Link #SM162",
			expected: "SM162",
		},
		{
			name:     "no number",
			input:    "Pikachu",
			expected: "",
		},
		{
			name:     "number in bracket - should not extract",
			input:    "Pikachu [#123]",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCollectorNumber(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractCollectorNumber(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeCardNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes leading zeros",
			input:    "006/165",
			expected: "6",
		},
		{
			name:     "removes denominator",
			input:    "021/198",
			expected: "21",
		},
		{
			name:     "keeps prefix",
			input:    "GG42/GG70",
			expected: "GG42",
		},
		{
			name:     "simple number unchanged",
			input:    "201",
			expected: "201",
		},
		{
			name:     "keeps SV prefix",
			input:    "SV151",
			expected: "SV151",
		},
		{
			name:     "keeps SV prefix with denominator",
			input:    "SV151/SV200",
			expected: "SV151",
		},
		{
			name:     "all zeros becomes zero",
			input:    "000/100",
			expected: "0",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "all-letter input unchanged",
			input:    "ABC",
			expected: "ABC",
		},
		{
			name:     "single letter unchanged",
			input:    "X",
			expected: "X",
		},
		{
			name:     "all-letter with denominator",
			input:    "PROMO/SET",
			expected: "PROMO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeCardNumber(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeCardNumber(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSimplifyForSearch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "truncate after type suffix ex",
			input:    "Sylveon ex SPECIAL ILLUSTRATION RARE",
			expected: "Sylveon ex",
		},
		{
			name:     "deduplicate + truncate after type suffix",
			input:    "CHARIZARD ex CHARIZARD ex SUPER PREM COLL",
			expected: "CHARIZARD ex",
		},
		{
			name:     "strip trailing noise with type in name",
			input:    "UMBREON ex PRE PREMIUM FIGURE COLL",
			expected: "UMBREON ex",
		},
		{
			name:     "set-specific trailing words preserved",
			input:    "BIRTHDAY PIKACHU CLASSIC COLL BLACK STAR",
			expected: "BIRTHDAY PIKACHU CLASSIC COLL BLACK STAR",
		},
		{
			name:     "set-specific words preserved for hint resolution",
			input:    "SNORLAX SD.100 COROCORO COMIC VER.",
			expected: "SNORLAX SD.100 COROCORO COMIC VER.",
		},
		{
			name:     "preserve meaningful middle words",
			input:    "RAYQUAZA SPIRIT LINK POKEMON CENTER PROMO",
			expected: "RAYQUAZA SPIRIT LINK",
		},
		{
			name:     "strip edition suffix",
			input:    "DARK GYARADOS 1ST EDITION",
			expected: "DARK GYARADOS",
		},
		{
			name:     "truncate after EX type",
			input:    "DRAGONITE EX 1ST EDITION",
			expected: "DRAGONITE EX",
		},
		{
			name:     "strip universal noise from possessive name",
			input:    "TOHOKU'S PIKACHU SPECIAL BOX PC",
			expected: "TOHOKU'S PIKACHU",
		},
		{
			name:     "set-specific words preserved (hints handle these)",
			input:    "MEW SOUTHERN ISLAND R.I.",
			expected: "MEW SOUTHERN ISLAND R.I.",
		},
		{
			name:     "single word unchanged",
			input:    "MEWTWO",
			expected: "MEWTWO",
		},
		{
			name:     "single word no change",
			input:    "GENGAR",
			expected: "GENGAR",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "type suffix is only word - preserved",
			input:    "ex",
			expected: "ex",
		},
		{
			name:     "type suffix at position 0 - no truncation",
			input:    "EX DRAGON",
			expected: "EX DRAGON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SimplifyForSearch(tt.input)
			if result != tt.expected {
				t.Errorf("SimplifyForSearch(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractSetKeyword(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"expedition set", "Expedition", "Expedition"},
		{"neo promo", "NEO 2 PROMO", "NEO"},
		{"mcdonalds collection", "MCDONALD'S COLLECTION", "MCDONALD'S"},
		{"numeric set", "151", "151"},
		{"all generic words", "set cards pokemon", ""},
		{"empty string", "", ""},
		{"short tokens only", "of", ""},
		{"scarlet violet", "Scarlet Violet 151", "Scarlet"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSetKeyword(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractSetKeyword(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMissingSetTokens(t *testing.T) {
	tests := []struct {
		name      string
		resultSet string
		expected  string
		missing   []string
	}{
		{
			name:      "neo missing from cd promo",
			resultSet: "Japanese CD Promo",
			expected:  "JAPANESE NEO 2 PROMO",
			missing:   []string{"neo"},
		},
		{
			name:      "full match",
			resultSet: "Japanese Neo Premium File",
			expected:  "JAPANESE NEO 2 PROMO",
			missing:   nil,
		},
		{
			name:      "empty expected",
			resultSet: "Some Set",
			expected:  "",
			missing:   nil,
		},
		{
			name:      "generic words excluded",
			resultSet: "Expedition",
			expected:  "Pokemon Expedition Set",
			missing:   nil, // "pokemon" and "set" are generic
		},
		{
			name:      "connector word and excluded",
			resultSet: "Black White Promos",
			expected:  "Black and White Promos",
			missing:   nil, // "and" is a connector word
		},
		{
			name:      "connector word of excluded",
			resultSet: "Legends Arceus",
			expected:  "Legends of Arceus",
			missing:   nil, // "of" is a connector word
		},
		{
			name:      "ampersand normalized away",
			resultSet: "Scarlet Violet",
			expected:  "Scarlet & Violet",
			missing:   nil, // "&" becomes a space in normalizeSetTokens
		},
		{
			name:      "SVP promo after normalization",
			resultSet: "Pokemon Promo",
			expected:  "SV BLACK STAR PROMO",           // NormalizeSetNameForSearch("SVP EN-SV BLACK STAR PROMO")
			missing:   []string{"sv", "black", "star"}, // caller handles promo-set bypass
		},
		{
			name:      "Chinese after normalization",
			resultSet: "Chinese Gem Pack 2",
			expected:  "Chinese GEM PACK VOL 2",
			missing:   []string{"vol"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MissingSetTokens(tt.resultSet, tt.expected)
			if len(result) != len(tt.missing) {
				t.Errorf("MissingSetTokens(%q, %q) = %v, want %v", tt.resultSet, tt.expected, result, tt.missing)
				return
			}
			for i := range result {
				if result[i] != tt.missing[i] {
					t.Errorf("MissingSetTokens(%q, %q)[%d] = %q, want %q", tt.resultSet, tt.expected, i, result[i], tt.missing[i])
				}
			}
		})
	}
}

func TestNormalizeSetNameSimple(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short numeric set name - no prefix",
			input:    "151",
			expected: "151",
		},
		{
			name:     "full set name - no prefix added",
			input:    "Base Set",
			expected: "Base Set",
		},
		{
			name:     "set name with special characters",
			input:    "Scarlet & Violet",
			expected: "Scarlet Violet",
		},
		{
			name:     "set name with colon",
			input:    "Scarlet & Violet: 151",
			expected: "Scarlet Violet 151",
		},
		{
			name:     "set name with hyphen",
			input:    "Sword-Shield",
			expected: "Sword Shield",
		},
		{
			name:     "already has pokemon prefix - stripped",
			input:    "Pokemon Base Set",
			expected: "Base Set",
		},
		{
			name:     "PSA category with year and Pokemon prefix",
			input:    "2013 Pokemon Black & White Promos",
			expected: "Black White Promos",
		},
		{
			name:     "year prefix without Pokemon",
			input:    "2023 Scarlet & Violet",
			expected: "Scarlet Violet",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
		{
			name:     "neo revelation",
			input:    "Neo Revelation",
			expected: "Neo Revelation",
		},
		{
			name:     "chilling reign",
			input:    "Chilling Reign",
			expected: "Chilling Reign",
		},
		{
			name:     "Japanese prefix stripped before PSA code",
			input:    "Japanese SWSH45 EN Vivid Voltage",
			expected: "Vivid Voltage",
		},
		{
			name:     "Japanese prefix with short PSA code",
			input:    "Japanese PRE EN Prismatic Evolutions",
			expected: "Prismatic Evolutions",
		},
		{
			name:     "Simplified Chinese CBB2 set",
			input:    "SIMPLIFIED CHINESE CBB2 C GEM PACK VOL 2",
			expected: "Chinese GEM PACK VOL 2",
		},
		{
			name:     "Traditional Chinese CBB1 set",
			input:    "TRADITIONAL CHINESE CBB1 C GEM PACK VOL 1",
			expected: "Chinese GEM PACK VOL 1",
		},
		{
			name:     "CN prefix still works",
			input:    "CN CBB2 C GEM PACK VOL 2",
			expected: "Chinese GEM PACK VOL 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeSetNameSimple(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeSetNameSimple(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizePurchaseName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"holo suffix", "DARK GYARADOS-HOLO", "DARK GYARADOS Holo"},
		{"rev.foil suffix", "MEWTWO-REV.FOIL", "MEWTWO Reverse Foil"},
		{"rev.holo suffix", "SNORLAX-REV.HOLO", "SNORLAX Reverse Holo"},
		{"no suffix", "Charizard ex", "Charizard ex"},
		{"with card number", "Charizard ex #161", "Charizard ex"},
		{"uppercase holo", "UMBREON-HOLO", "UMBREON Holo"},
		{"bracket modifier and holo", "MEW-HOLO [Jumbo]", "MEW Holo"},
		{"simple name", "Pikachu", "Pikachu"},
		{"hyphenated name no suffix", "MEW-EX", "MEW EX"},
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePurchaseName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizePurchaseName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNumericOnly(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"pure digits", "75", "75"},
		{"SWSH prefix", "SWSH75", "75"},
		{"SM prefix", "SM162", "162"},
		{"GG prefix", "GG42", "42"},
		{"SV prefix", "SV151", "151"},
		{"no digits", "ABC", ""},
		{"empty", "", ""},
		{"single digit", "5", "5"},
		{"leading zero preserved by NumericOnly", "SWSH075", "075"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NumericOnly(tt.input)
			if got != tt.expect {
				t.Errorf("NumericOnly(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

// TestNormalizationChain traces card names through the full shared normalization pipeline
// to document expected composed behavior and catch regressions when individual functions change.
func TestNormalizationChain(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		afterNormalize string // NormalizePurchaseName output
		afterSimplify  string // SimplifyForSearch output
	}{
		{
			"holo abbreviation",
			"DARK GYARADOS-HOLO",
			"DARK GYARADOS Holo",
			"DARK GYARADOS Holo", // SimplifyForSearch doesn't strip variants; StripVariantSuffix does
		},
		{
			"reverse foil abbreviation",
			"MEWTWO-REV.FOIL",
			"MEWTWO Reverse Foil",
			"MEWTWO Reverse Foil", // variants preserved through SimplifyForSearch
		},
		{
			"ex type suffix truncation",
			"CHARIZARD ex CHARIZARD ex SUPER PREM COLL",
			"CHARIZARD ex CHARIZARD ex SUPER PREM COLL",
			"CHARIZARD ex",
		},
		{
			"no abbreviation plain name",
			"PIKACHU VMAX",
			"PIKACHU VMAX",
			"PIKACHU VMAX",
		},
		{
			"special delivery abbreviation",
			"SP.DELIVERY CHARIZARD-HOLO",
			"Special Delivery CHARIZARD Holo",
			"Special Delivery CHARIZARD Holo", // variants preserved
		},
		{
			"trailing noise stripped",
			"UMBREON EX PREMIUM COLLECTION",
			"UMBREON EX PREMIUM COLLECTION",
			"UMBREON EX",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			normalized := NormalizePurchaseName(tc.input)
			if normalized != tc.afterNormalize {
				t.Errorf("NormalizePurchaseName(%q) = %q, want %q", tc.input, normalized, tc.afterNormalize)
			}
			simplified := SimplifyForSearch(normalized)
			if simplified != tc.afterSimplify {
				t.Errorf("SimplifyForSearch(%q) = %q, want %q", normalized, simplified, tc.afterSimplify)
			}
		})
	}
}

// MapChineseNumber, IsChineseSet, and IsChineseGemPackSet tests live in
// pricecharting/domain_adapter_edge_test.go alongside resolveExpectedNumber
// tests that exercise the full pipeline with more comprehensive edge cases.
