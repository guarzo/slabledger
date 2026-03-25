package pricecharting

import (
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
)

func TestMapChineseNumber(t *testing.T) {
	tests := []struct {
		name        string
		setName     string
		number      string
		want        string
		wantUnknown bool
	}{
		// CBB1 / Vol 1 — base 700
		{"CBB1 card 9", "CHINESE CBB1 C GEM PACK VOL 1", "009/015", "709", false},
		{"Vol 1 card 1", "Chinese GEM PACK VOL 1", "001", "701", false},
		{"CBB1 card 15", "CN CBB1 C GEM PACK VOL 1", "15", "715", false},

		// CBB2 / Vol 2 — base 600
		{"CBB2 card 5", "CHINESE CBB2 C GEM PACK VOL 2", "005/015", "605", false},
		{"Vol 2 card 12", "Chinese GEM PACK VOL 2", "12", "612", false},

		// CBB3 / Vol 3 — base 300
		{"CBB3 card 5", "CHINESE CBB3 C GEM PACK VOL 3", "005", "305", false},
		{"CBB3 card 7", "SIMPLIFIED CHINESE CBB3 C-GEM PACK VOL 3", "07", "307", false},

		// Unknown volume — returns "" and flags unknown
		{"unknown vol", "Chinese GEM PACK VOL 5", "010", "", true},

		// Edge cases
		{"empty number", "CHINESE CBB1 C GEM PACK VOL 1", "", "", false},
		{"non-numeric", "CHINESE CBB1 C GEM PACK VOL 1", "ABC", "ABC", false},
		{"zero", "CHINESE CBB1 C GEM PACK VOL 1", "000", "000", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, unknownVol := cardutil.MapChineseNumber(tc.setName, tc.number)
			if got != tc.want {
				t.Errorf("cardutil.MapChineseNumber(%q, %q) = %q, want %q", tc.setName, tc.number, got, tc.want)
			}
			if unknownVol != tc.wantUnknown {
				t.Errorf("cardutil.MapChineseNumber(%q, %q) unknownVol = %v, want %v", tc.setName, tc.number, unknownVol, tc.wantUnknown)
			}
		})
	}
}

func TestIsChineseSet(t *testing.T) {
	tests := []struct {
		setName string
		want    bool
	}{
		{"CHINESE CBB1 C GEM PACK VOL 1", true},
		{"CN CBB1 C GEM PACK VOL 1", true},
		{"SIMPLIFIED CHINESE CBB2 C GEM PACK VOL 2", true},
		{"Chinese GEM PACK VOL 1", true},
		{"Scarlet Violet 151", false},
		{"Base Set", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.setName, func(t *testing.T) {
			if got := cardutil.IsChineseSet(tc.setName); got != tc.want {
				t.Errorf("cardutil.IsChineseSet(%q) = %v, want %v", tc.setName, got, tc.want)
			}
		})
	}
}

func TestIsChineseGemPackSet(t *testing.T) {
	tests := []struct {
		setName string
		want    bool
	}{
		{"CHINESE CBB1 C GEM PACK VOL 1", true},
		{"Chinese GEM PACK VOL 2", true},
		{"CN CBB2 C GEM PACK VOL 2", true},
		{"Chinese Gem Pack Vol 1", true},
		{"SIMPLIFIED CHINESE CBB3 C-GEM PACK VOL 3", true},
		{"Chinese GEM PACK VOL 3", true},
		{"CHINESE SOME OTHER SET", false},
		{"Base Set", false},
	}
	for _, tc := range tests {
		t.Run(tc.setName, func(t *testing.T) {
			if got := cardutil.IsChineseGemPackSet(tc.setName); got != tc.want {
				t.Errorf("cardutil.IsChineseGemPackSet(%q) = %v, want %v", tc.setName, got, tc.want)
			}
		})
	}
}

func TestResolveExpectedNumber(t *testing.T) {
	tests := []struct {
		name        string
		setName     string
		cardNumber  string
		wantNumber  string
		wantUnknown bool
	}{
		// Normal card — passthrough with era prefix
		{"normal card", "Scarlet Violet 151", "25", "25", false},

		// SWSH Black Star Promo — prepends era prefix
		{"SWSH promo", "SWSH Black Star Promo", "075", "SWSH075", false},

		// Chinese CBB1 — maps to species-based number
		{"Chinese CBB1", "CHINESE CBB1 C GEM PACK VOL 1", "009/015", "709", false},

		// Chinese CBB3 — maps to species-based number (base 300)
		{"Chinese CBB3", "CHINESE CBB3 C GEM PACK VOL 3", "005", "305", false},

		// Empty number — stays empty
		{"empty number", "Base Set", "", "", false},

		// Non-Chinese non-promo — simple passthrough
		{"base set card", "Base Set", "4", "4", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, unknownVol := resolveExpectedNumber(tc.setName, tc.cardNumber)
			if got != tc.wantNumber {
				t.Errorf("resolveExpectedNumber(%q, %q) = %q, want %q", tc.setName, tc.cardNumber, got, tc.wantNumber)
			}
			if unknownVol != tc.wantUnknown {
				t.Errorf("resolveExpectedNumber(%q, %q) unknownVol = %v, want %v", tc.setName, tc.cardNumber, unknownVol, tc.wantUnknown)
			}
		})
	}
}

func TestExtractSetHint(t *testing.T) {
	tests := []struct {
		name     string
		cardName string
		want     string
	}{
		// Parenthetical hints
		{"parenthetical set hint", "Charizard (Base Set)", "Base Set"},
		{"parenthetical with spaces", "Pikachu ( Expedition )", "Expedition"},

		// Multi-token hints from known set tokens
		{"multi-token hint", "Fossil Jungle Pikachu", "fossil jungle"},
		{"two known tokens", "Sword Shield Pikachu VMAX", "sword shield"},

		// Single-token rejected (too generic)
		{"single known token", "Promo Pikachu", ""},
		{"no known tokens", "Charizard VMAX", ""},

		// Empty/no hint
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractSetHint(tc.cardName)
			if got != tc.want {
				t.Errorf("extractSetHint(%q) = %q, want %q", tc.cardName, got, tc.want)
			}
		})
	}
}
