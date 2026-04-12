package csvimport

// CollectionSuffix describes a promo collection name that should be stripped
// from card names to avoid polluting pricing queries.
type CollectionSuffix struct {
	// Pattern is the uppercase suffix to match.
	Pattern string
	// TrailingOnly restricts matching to the end of the name. Broader/shorter
	// patterns use this to avoid corrupting card identity mid-string.
	TrailingOnly bool
	// Example documents a real PSA title that triggers this suffix.
	Example string
}

// collectionSuffixRegistry is the single source of truth for collection suffixes.
// Ordered longest-first within each tier so longer patterns match before shorter substrings.
// Use anywhereSuffixes / trailingSuffixes for iteration — they avoid per-loop filtering.
var collectionSuffixRegistry = []CollectionSuffix{
	// Anywhere suffixes — matched anywhere after a space in the name.
	{Pattern: "CROWN ZENITH PREMIUM COLLECTION SEA & SKY", Example: "RAYQUAZA-HOLO CROWN ZENITH PREMIUM COLLECTION SEA & SKY"},
	{Pattern: "PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION", Example: "UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION"},
	{Pattern: "SPECIAL BOX POKEMON CENTER TOHOKU", Example: "TOHOKU'S PIKACHU SPECIAL BOX POKEMON CENTER TOHOKU"},
	{Pattern: "TEAM UP SINGLE PACK BLISTERS", Example: "PIKACHU-HOLO TEAM UP SINGLE PACK BLISTERS"},
	{Pattern: "SD.100 COROCORO COMIC VER.", Example: "SNORLAX SD.100 COROCORO COMIC VER."},
	{Pattern: "SUPER-PREMIUM COLLECTION", Example: "CHARIZARD EX CHARIZARD ex SUPER-PREMIUM COLLECTION"},
	{Pattern: "SUPER PREMIUM COLLECTION", Example: "CHARIZARD EX SUPER PREMIUM COLLECTION"},
	{Pattern: "SPECIAL ILLUSTRATION RARE", Example: "SYLVEON EX SPECIAL ILLUSTRATION RARE"},
	{Pattern: "PREMIUM FIGURE COLLECTION", Example: "UMBREON EX PREMIUM FIGURE COLLECTION"},
	{Pattern: "POKEMON CENTER UNITED KINGDOM", Example: "SPECIAL DELIVERY CHARIZARD-HOLO POKEMON CENTER UNITED KINGDOM"},
	{Pattern: "CLASSIC COLL-BLACK STAR", Example: "BIRTHDAY PIKACHU-HOLO CLASSIC COLL-BLACK STAR"},
	{Pattern: "POKEMON CENTER TOHOKU", Example: "PIKACHU POKEMON CENTER TOHOKU"},
	{Pattern: "SPECIAL BOX PC TOHOKU", Example: "TOHOKU'S PIKACHU SPECIAL BOX PC TOHOKU"},
	{Pattern: "POKEMON CENTER PROMO", Example: "RAYQUAZA SPIRIT LINK POKEMON CENTER PROMO"},
	{Pattern: "SINGLE PACK BLISTERS", Example: "PIKACHU-HOLO SINGLE PACK BLISTERS"},
	{Pattern: "CLASSIC COLLECTION", Example: "BIRTHDAY PIKACHU-HOLO CLASSIC COLLECTION"},
	{Pattern: "PREMIUM COLLECTION", Example: "CHARIZARD EX PREMIUM COLLECTION"},
	{Pattern: "SPECIAL COLLECTION", Example: "PIKACHU SPECIAL COLLECTION"},
	{Pattern: "COROCORO COMIC VER.", Example: "SNORLAX COROCORO COMIC VER."},
	{Pattern: "POKEMON CENTER", Example: "PIKACHU POKEMON CENTER"},

	// Trailing-only suffixes — only stripped when at the end of the name.
	{Pattern: "CROWN ZENITH", TrailingOnly: true, Example: "RAYQUAZA-HOLO CRZ CROWN ZENITH"},
	{Pattern: "SPECIAL ART RARE", TrailingOnly: true, Example: "MEGA GARDEVOIR EX SPECIAL ART RARE"},
}

// anywhereSuffixes and trailingSuffixes are pre-partitioned views of
// collectionSuffixRegistry, computed once at package init. This avoids
// per-call filtering in stripCollectionSuffix.
var anywhereSuffixes, trailingSuffixes = func() ([]CollectionSuffix, []CollectionSuffix) {
	var anywhere, trailing []CollectionSuffix
	for _, cs := range collectionSuffixRegistry {
		if cs.TrailingOnly {
			trailing = append(trailing, cs)
		} else {
			anywhere = append(anywhere, cs)
		}
	}
	return anywhere, trailing
}()
