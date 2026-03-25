package constants

import "testing"

func TestIsGenericSetName(t *testing.T) {
	generic := []string{
		"", "pokemon cards", "Pokemon Cards", "tcg cards", "TCG Cards",
		"cards", "pokemon", "Pokemon", "trading cards", "Trading Cards",
		"other", "Other", "  pokemon  ", " cards ",
	}
	for _, s := range generic {
		if !IsGenericSetName(s) {
			t.Errorf("expected %q to be generic", s)
		}
	}

	specific := []string{
		"Base Set", "Prismatic Evolutions", "Crown Zenith",
		"Scarlet & Violet", "Expedition", "151", "game", "Game", "GAME",
	}
	for _, s := range specific {
		if IsGenericSetName(s) {
			t.Errorf("expected %q to NOT be generic", s)
		}
	}
}
