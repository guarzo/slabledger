package inventory

import (
	"strings"
	"unicode/utf8"
)

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

// StripCollectionSuffix removes promo collection names from card names using
// the pre-partitioned anywhereSuffixes / trailingSuffixes slices.
// PSA cert Subject includes these but they pollute pricing queries.
// E.g., "UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION" → "UMBREON EX"
func StripCollectionSuffix(name string) string {
	upper := strings.ToUpper(name)

	// Anywhere suffixes: find the leftmost match across all patterns.
	// We scan all patterns and pick the earliest match position to avoid order-dependent
	// behavior where a later-registered but earlier-occurring pattern would be missed.
	minIdx := -1
	for _, cs := range anywhereSuffixes {
		if idx := strings.Index(upper, " "+cs.Pattern); idx > 0 {
			if minIdx < 0 || idx < minIdx {
				minIdx = idx
			}
		}
	}
	if minIdx > 0 {
		stripped := strings.TrimSpace(name[:minIdx])
		if stripped != "" {
			return stripped
		}
	}

	// Trailing-only suffixes: only strip when at the very end.
	// Loop until no more match so stacked suffixes are fully removed.
	// Recompute upper after each strip to keep byte offsets in sync
	// (strings.ToUpper can change byte length for non-ASCII runes).
	const maxSuffixIterations = 100
	for iter := 0; iter < maxSuffixIterations; iter++ {
		matched := false
		for _, cs := range trailingSuffixes {
			pat := " " + cs.Pattern
			if strings.HasSuffix(upper, pat) {
				runeCount := utf8.RuneCountInString(pat)
				nameRunes := []rune(name)
				stripped := strings.TrimSpace(string(nameRunes[:len(nameRunes)-runeCount]))
				if stripped == "" {
					break
				}
				name = stripped
				upper = strings.ToUpper(name)
				matched = true
				break // restart outer loop with updated name
			}
		}
		if !matched {
			break
		}
	}
	return name
}
