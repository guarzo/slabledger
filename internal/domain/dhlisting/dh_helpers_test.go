package dhlisting

import "testing"

// TestCleanCardNameForDH covers the card name cleaning rules:
// structured names, art prefixes, variant suffixes, rarity suffixes, and
// edition suffixes — in the order the function applies them.
func TestCleanCardNameForDH(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantName    string
		wantVariant string
	}{
		// Structured "Name - Set - #Number [Language]" format (parseStructuredName)
		{
			name:     "structured name with hash: returns name portion only",
			raw:      "Charizard - Base Set - #4",
			wantName: "Charizard",
		},
		{
			name:     "structured name with bracket: returns name portion only",
			raw:      "Dragonite - Team Rocket [JPN]",
			wantName: "Dragonite",
		},
		{
			name:     "structured name without hash or bracket: not parsed as structured",
			raw:      "Pikachu - Base Set",
			wantName: "Pikachu - Base Set",
		},

		// Art prefix stripping
		{
			name:     "FA/ prefix stripped",
			raw:      "FA/Charizard",
			wantName: "Charizard",
		},
		{
			name:     "FULL ART/ prefix stripped",
			raw:      "FULL ART/Pikachu",
			wantName: "Pikachu",
		},
		{
			name:     "fa/ lowercase prefix stripped",
			raw:      "fa/Squirtle",
			wantName: "Squirtle",
		},

		// Variant suffixes (extractVariant)
		{
			name:        "HOLO variant extracted",
			raw:         "Charizard-HOLO",
			wantName:    "Charizard",
			wantVariant: "Holo",
		},
		{
			name:        "REVERSE HOLO variant extracted",
			raw:         "Blastoise-REVERSE HOLO",
			wantName:    "Blastoise",
			wantVariant: "Reverse Holo",
		},
		{
			name:        "REVERSE FOIL maps to Reverse Holo",
			raw:         "Venusaur-REVERSE FOIL",
			wantName:    "Venusaur",
			wantVariant: "Reverse Holo",
		},
		{
			name:        "REV.FOIL maps to Reverse Holo",
			raw:         "Mewtwo-REV.FOIL",
			wantName:    "Mewtwo",
			wantVariant: "Reverse Holo",
		},
		{
			name:        "GOLD STAR variant extracted",
			raw:         "Pikachu-GOLD STAR",
			wantName:    "Pikachu",
			wantVariant: "Gold Star",
		},

		// Rarity suffix stripping
		{
			name:     "ULTRA RARE suffix stripped",
			raw:      "Charizard ULTRA RARE",
			wantName: "Charizard",
		},
		{
			name:     "HYPER RARE suffix stripped",
			raw:      "Lugia HYPER RARE",
			wantName: "Lugia",
		},
		{
			name:     "ILLUSTRATION RARE suffix stripped",
			raw:      "Pikachu ILLUSTRATION RARE",
			wantName: "Pikachu",
		},
		{
			name:     "SPECIAL ILLUSTRATION RARE stripped (longest match wins)",
			raw:      "Mewtwo SPECIAL ILLUSTRATION RARE",
			wantName: "Mewtwo",
		},
		{
			name:     "ART RARE suffix stripped",
			raw:      "Eevee ART RARE",
			wantName: "Eevee",
		},

		// Edition suffix stripping
		{
			name:     "1ST EDITION suffix stripped",
			raw:      "Charizard 1ST EDITION",
			wantName: "Charizard",
		},
		{
			name:     "1ST ED. suffix stripped",
			raw:      "Blastoise 1ST ED.",
			wantName: "Blastoise",
		},
		{
			name:     "edition suffix with leading dash stripped",
			raw:      "Charizard - 1ST EDITION",
			wantName: "Charizard",
		},

		// Title-case conversion
		{
			name:     "all-caps name converted to title case",
			raw:      "DRAGONITE",
			wantName: "Dragonite",
		},
		{
			name:     "multi-word name title-cased",
			raw:      "DARK CHARIZARD",
			wantName: "Dark Charizard",
		},

		// Edge cases
		{
			name:     "empty string returns empty",
			raw:      "",
			wantName: "",
		},
		{
			name:     "plain lowercase name title-cased",
			raw:      "pikachu",
			wantName: "Pikachu",
		},
		{
			name:        "combined: FA prefix + HOLO variant",
			raw:         "FA/Charizard-HOLO",
			wantName:    "Charizard",
			wantVariant: "Holo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotVariant := CleanCardNameForDH(tc.raw)
			if gotName != tc.wantName {
				t.Errorf("name: got %q, want %q", gotName, tc.wantName)
			}
			if gotVariant != tc.wantVariant {
				t.Errorf("variant: got %q, want %q", gotVariant, tc.wantVariant)
			}
		})
	}
}

// TestNormalizeCardNum covers leading-zero stripping and edge cases.
func TestNormalizeCardNum(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"004", "4"},
		{"001", "1"},
		{"100", "100"},
		{"0", "0"},
		{"000", "0"},
		{"", ""},
		{"abc", "abc"},
		{"0ab", "ab"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeCardNum(tc.input)
			if got != tc.want {
				t.Errorf("normalizeCardNum(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
