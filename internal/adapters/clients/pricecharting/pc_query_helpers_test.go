package pricecharting

import "testing"

// TestNormalizeCardName_PC tests the PriceCharting-specific card name normalization.
// This function handles PSA listing titles and CL import names differently from
// cardutil.NormalizePurchaseName — see cardutil package doc for pipeline overview.
func TestNormalizeCardName_PC(t *testing.T) {
	tests := []struct {
		name          string
		cardName      string
		normalizedSet string
		want          string
	}{
		{
			"expands rev.foil",
			"MEWTWO-REV.FOIL",
			"Expedition",
			"MEWTWO Reverse Foil",
		},
		{
			"expands sp.delivery",
			"SP.DELIVERY CHARIZARD",
			"Promo",
			"Special Delivery CHARIZARD",
		},
		{
			"expands rev. foil with space",
			"UMBREON REV. FOIL",
			"Prismatic",
			"UMBREON Reverse Foil",
		},
		{
			"strips PSA grade suffix",
			"CHARIZARD PSA 10",
			"Base Set",
			"CHARIZARD",
		},
		{
			"strips embedded card number",
			"MEWTWO #56 PSA 9",
			"Expedition",
			"MEWTWO",
		},
		{
			"strips leading year",
			"2002 Pokemon Expedition Mewtwo",
			"Expedition",
			"Mewtwo",
		},
		{
			"strips set prefix from card name",
			"Pokemon Expedition Mewtwo Reverse Foil",
			"Expedition",
			"Mewtwo Reverse Foil",
		},
		{
			"hyphens become spaces",
			"UMBREON-HOLO",
			"Base Set",
			"UMBREON HOLO",
		},
		{
			"deduplicates PROMO when set contains it",
			"PIKACHU PROMO",
			"Black Star Promos",
			"PIKACHU",
		},
		{
			"preserves PROMO when set does not contain it",
			"PIKACHU PROMO",
			"Expedition",
			"PIKACHU PROMO",
		},
		{
			"empty input",
			"",
			"",
			"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeCardName(tc.cardName, tc.normalizedSet)
			if got != tc.want {
				t.Errorf("normalizeCardName(%q, %q) = %q, want %q", tc.cardName, tc.normalizedSet, got, tc.want)
			}
		})
	}
}
