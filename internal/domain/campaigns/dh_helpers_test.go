package campaigns

import "testing"

func TestCleanCardNameForDH(t *testing.T) {
	tests := []struct {
		input       string
		wantName    string
		wantVariant string
	}{
		// Basic variants
		{"GENGAR-HOLO", "Gengar", "Holo"},
		{"CHARIZARD-REVERSE HOLO", "Charizard", "Reverse Holo"},
		{"PIKACHU", "Pikachu", ""},
		{"", "", ""},

		// -HOLO with trailing set/edition info (should be discarded)
		{"VENUSAUR-HOLO CD PROMO", "Venusaur", "Holo"},
		{"CHARIZARD-HOLO BASE SET 1999-2000", "Charizard", "Holo"},
		{"CHARIZARD-HOLO CLASSIC COLL-BASE SET", "Charizard", "Holo"},
		{"ALAKAZAM-HOLO 1ST EDITION", "Alakazam", "Holo"},
		{"DARK CHARIZARD-HOLO 1ST EDITION", "Dark Charizard", "Holo"},
		{"GYARADOS-HOLO EX HOLON PHANTOMS", "Gyarados", "Holo"},
		{"UMBREON-HOLO UNDAUNTED-CRACKED ICE", "Umbreon", "Holo"},
		{"GENGAR-HOLO BRKTHR.-COSMOS-CHMPS.TIN", "Gengar", "Holo"},

		// REV.FOIL / REVERSE FOIL variants
		{"SNORLAX-REV.FOIL", "Snorlax", "Reverse Holo"},
		{"PARASECT-REV.FOIL FIRE RED & LEAF GREEN", "Parasect", "Reverse Holo"},
		{"CHARIZARD G-REV.FOIL SUPREME VICTORS", "Charizard G", "Reverse Holo"},
		{"RAYQUAZA-REVERSE FOIL", "Rayquaza", "Reverse Holo"},
		{"CHARIZARD PROMO-REVERSE FOIL", "Charizard Promo", "Reverse Holo"},

		// Gold Star
		{"UMBREON-GOLD STAR CLASSIC COLL-POP SERIES 5", "Umbreon", "Gold Star"},

		// FA/ prefix
		{"FA/GIRATINA PROMO-TEAM PLASMA", "Giratina Promo-team Plasma", ""},
		{"FA/UMBREON VMAX BRILLIANT STARS", "Umbreon Vmax Brilliant Stars", ""},
		{"FULL ART/M TYRANITAR EX ANCIENT ORIGINS-PORTUGUESE", "M Tyranitar Ex Ancient Origins-portuguese", ""},

		// Rarity suffixes stripped
		{"BLASTOISE ex SPECIAL ILLUSTRATION RARE", "Blastoise Ex", ""},
		{"CHARIZARD ex ULTRA RARE", "Charizard Ex", ""},
		{"PIKACHU ILLUSTRATION RARE", "Pikachu", ""},
		{"MEGA GARDEVOIR ex MEGA HYPER RARE", "Mega Gardevoir Ex", ""},
		{"CHARIZARD ex SPECIAL ART RARE", "Charizard Ex", ""},

		// Edition suffixes stripped
		{"DRAGONITE 1ST EDITION", "Dragonite", ""},
		{"DARK CHARIZARD 1ST EDITION", "Dark Charizard", ""},
		{"GENGAR 1ST EDITION", "Gengar", ""},
		{"M GALLADE EX EMERALD BREAK-1ST ED.", "M Gallade Ex Emerald Break", ""},

		// Structured format: "Name - Set - #Number [Language]"
		{"Alakazam - Game Base Ii - #1", "Alakazam", ""},
		{"Arcanine - Ex Sandstorm - #15", "Arcanine", ""},
		{"Blaine'S Moltres - Gym Heroes - #1", "Blaine's Moltres", ""},
		{"Charizard G Lv.X - Charizard Half Deck - #002 [Japanese]", "Charizard G Lv.x", ""},
		{"Celebi V - Cs3a C-Primordial Arts: Overgrow - #132 [Chinese]", "Celebi V", ""},

		// Plain names without variants
		{"M GENGAR EX", "M Gengar Ex", ""},
		{"BLASTOISE", "Blastoise", ""},
		{"DARK CHARIZARD", "Dark Charizard", ""},
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
