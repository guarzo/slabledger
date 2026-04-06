package campaigns

import "testing"

func TestCleanCardNameForDH(t *testing.T) {
	tests := []struct {
		input       string
		wantName    string
		wantVariant string
	}{
		{"GENGAR-HOLO", "Gengar", "Holo"},
		{"CHARIZARD-REVERSE HOLO", "Charizard", "Reverse Holo"},
		{"PIKACHU", "Pikachu", ""},
		{"DRAGONITE 1ST EDITION", "Dragonite 1st Edition", ""},
		{"VENUSAUR-HOLO CD PROMO", "Venusaur Cd Promo", "Holo"},
		{"M GENGAR EX", "M Gengar Ex", ""},
		{"", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			name, variant := CleanCardNameForDH(tc.input)
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if variant != tc.wantVariant {
				t.Errorf("variant = %q, want %q", variant, tc.wantVariant)
			}
		})
	}
}
