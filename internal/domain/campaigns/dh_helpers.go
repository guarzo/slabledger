package campaigns

import (
	"strings"
	"unicode"
)

// CleanCardNameForDH extracts a clean card name and variant hint from PSA-style
// card titles for DH cert resolution. The cert number is the primary lookup key;
// this just provides disambiguation hints.
func CleanCardNameForDH(raw string) (name, variant string) {
	if raw == "" {
		return "", ""
	}

	s := raw

	if n, ok := parseStructuredName(s); ok {
		return strings.TrimSpace(n), ""
	}

	s = stripArtPrefix(s)

	// Text after the variant is set/product/edition info already in set_name.
	s, variant = extractVariant(s)

	s = stripRaritySuffix(s)
	s = stripEditionSuffix(s)

	name = strings.Join(strings.Fields(toTitleCase(strings.TrimSpace(s))), " ")
	return name, variant
}

// parseStructuredName handles "Name - Set - #Number [Language]" format
// from cert lookups. Returns the name portion and true, or ("", false).
func parseStructuredName(s string) (string, bool) {
	parts := strings.SplitN(s, " - ", 2)
	if len(parts) < 2 {
		return "", false
	}
	// Verify the second part looks like set info (contains #, [, or a known set pattern).
	second := parts[1]
	if strings.Contains(second, "#") || strings.Contains(second, "[") {
		return toTitleCase(strings.TrimSpace(parts[0])), true
	}
	return "", false
}

// stripArtPrefix removes FA/ and FULL ART/ prefixes.
func stripArtPrefix(s string) string {
	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "FULL ART/") {
		return s[len("FULL ART/"):]
	}
	if strings.HasPrefix(upper, "FA/") {
		return s[len("FA/"):]
	}
	return s
}

// variantSuffixes are recognized variant markers. Order matters: longer/more
// specific patterns first to avoid partial matches.
var variantSuffixes = []struct {
	suffix  string
	variant string
}{
	{"-REVERSE HOLO", "Reverse Holo"},
	{"-REVERSE FOIL", "Reverse Holo"},
	{"-REV.FOIL", "Reverse Holo"},
	{"-GOLD STAR", "Gold Star"},
	{"-HOLO", "Holo"},
}

// extractVariant finds the first variant suffix, returns the text before it
// (the card name) and the variant. Text after the suffix is discarded.
func extractVariant(s string) (string, string) {
	upper := strings.ToUpper(s)
	for _, vs := range variantSuffixes {
		if idx := strings.Index(upper, vs.suffix); idx != -1 {
			return strings.TrimSpace(s[:idx]), vs.variant
		}
	}
	return s, ""
}

// raritySuffixes are rarity indicators appended to card names. They aren't part
// of the actual card name and should be stripped before sending to DH.
var raritySuffixes = []string{
	"SPECIAL ILLUSTRATION RARE",
	"SPECIAL ART RARE",
	"MEGA HYPER RARE",
	"MEGA ATTACK RARE",
	"MEGA ULTRA RARE",
	"ILLUSTRATION RARE",
	"ULTRA RARE",
	"HYPER RARE",
	"ART RARE",
}

// stripRaritySuffix removes trailing rarity indicators.
func stripRaritySuffix(s string) string {
	upper := strings.ToUpper(s)
	for _, suffix := range raritySuffixes {
		if strings.HasSuffix(upper, suffix) {
			return strings.TrimSpace(s[:len(s)-len(suffix)])
		}
	}
	return s
}

// editionSuffixes are edition/promo markers that should be removed.
var editionSuffixes = []string{
	"1ST EDITION",
	"1ST ED.",
}

// stripEditionSuffix removes trailing edition markers and any leading dash.
func stripEditionSuffix(s string) string {
	upper := strings.ToUpper(strings.TrimSpace(s))
	for _, suffix := range editionSuffixes {
		if strings.HasSuffix(upper, suffix) {
			result := strings.TrimSpace(s[:len(s)-len(suffix)])
			return strings.TrimRight(result, "- ")
		}
	}
	return s
}

// toTitleCase converts "DRAGONITE" → "Dragonite", handling apostrophes correctly.
func toTitleCase(s string) string {
	lower := strings.ToLower(s)
	runes := []rune(lower)
	capitalizeNext := true
	for i, r := range runes {
		if unicode.IsSpace(r) {
			capitalizeNext = true
		} else if capitalizeNext && r >= 'a' && r <= 'z' {
			runes[i] = r - 'a' + 'A'
			capitalizeNext = false
		} else {
			capitalizeNext = false
		}
	}
	return string(runes)
}
