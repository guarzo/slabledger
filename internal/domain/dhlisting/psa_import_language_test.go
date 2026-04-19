package dhlisting

import "testing"

func TestInferDHLanguage(t *testing.T) {
	tests := []struct {
		name     string
		setName  string
		cardName string
		want     string
	}{
		{"japanese promo", "JAPANESE PROMO", "Sceptile", "japanese"},
		{"japanese sm promo", "JAPANESE SM PROMO", "Pikachu", "japanese"},
		{"pokemon japanese black & white promo", "Pokemon Japanese Black & White Promo", "Lilligant", "japanese"},
		{"french", "FRENCH", "Leviator", "french"},
		{"german", "Base Set German", "Charizard", "german"},
		{"italian", "POKEMON ITALIAN", "Venusaur", "italian"},
		{"english swsh promo", "SWSH BLACK STAR PROMO", "Gengar", ""},
		{"english base", "Base Set", "Charizard", ""},
		{"empty", "", "", ""},
		{"case insensitive", "pokemon JaPaNeSe", "", "japanese"},
		{"name hints when set is empty", "", "Charizard Japanese Promo", "japanese"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferDHLanguage(tt.setName, tt.cardName)
			if got != tt.want {
				t.Errorf("InferDHLanguage(%q, %q) = %q, want %q", tt.setName, tt.cardName, got, tt.want)
			}
		})
	}
}
