package campaigns

import (
	"strings"
)

// cleanMMPlayerName extracts the core card name from our raw CardName/CardPlayer,
// stripping embedded variation keywords, set name references, and format suffixes
// so Market Movers can match the card against its database.
//
// Examples:
//
//	"CYNTHIA'S GARCHOMP ex SPECIAL ILLUSTRATION RARE"  → "Cynthia's Garchomp ex"
//	"NIDOKING-HOLO"                                    → "Nidoking"
//	"ENTEI-HOLO EX UNSEEN FORCES"                      → "Entei"
//	"FA/SHAYMIN EX XY COLLECTION PROMO"                → "Shaymin EX"
//	"DARK CHARIZARD-HOLO 1ST EDITION"                  → "Dark Charizard"
//	"SNORLAX-REV.FOIL"                                 → "Snorlax"
//	"PIKACHU-HOLO BLACK STAR PROMOS"                   → "Pikachu"
func cleanMMPlayerName(raw, setName string) string {
	name := raw

	// Strip "FA/" or "FULL ART/" prefix.
	name = stripPrefixes(name)

	// Strip set name embedded in the card name (case-insensitive).
	// Do this before dash/variation stripping so "GIRATINA PROMO-TEAM PLASMA"
	// with set "Pokemon Black & White Promo" can later strip the "PROMO" keyword.
	if setName != "" {
		name = stripSetName(name, setName)
	}

	// Strip dash-suffixes like "-HOLO", "-REV.FOIL", "-GOLD STAR", etc.
	name = stripDashSuffix(name)

	// Strip known variation/rarity keywords from the end.
	name = stripVariationKeywords(name)

	// Clean up and convert to title case.
	name = strings.TrimSpace(name)
	if name == "" {
		return mmTitleCase(raw) // safety: never return empty
	}
	return mmTitleCase(name)
}

// stripPrefixes removes leading "FA/" or "FULL ART/" from the name.
func stripPrefixes(name string) string {
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "FA/") {
		return name[3:]
	}
	if strings.HasPrefix(upper, "FULL ART/") {
		return name[9:]
	}
	return name
}

// stripDashSuffix removes trailing "-SUFFIX" segments where the suffix starts
// with a known format/variation marker (e.g. HOLO, REV.FOIL, GOLD STAR).
// Scans dashes left-to-right to handle names with multiple dashes like
// "UMBREON-GOLD STAR CLASSIC COLL-POP SERIES 5".
func stripDashSuffix(name string) string {
	upper := strings.ToUpper(name)
	// Try each dash position from left to right.
	for i := 1; i < len(upper); i++ {
		if upper[i] != '-' {
			continue
		}
		suffix := strings.TrimSpace(upper[i+1:])
		for _, marker := range mmDashSuffixes {
			if suffix == marker || strings.HasPrefix(suffix, marker+" ") {
				return strings.TrimSpace(name[:i])
			}
		}
	}
	return name
}

// mmDashSuffixes are the known dash-suffix markers to strip.
var mmDashSuffixes = []string{
	"HOLO",
	"REV.FOIL",
	"REVERSE FOIL",
	"GOLD STAR",
	"SECRET",
	"1ST ED",
	"1ST EDITION",
}

// stripSetName removes the set name when it appears as a trailing substring
// in the card name (case-insensitive). Handles abbreviated set references
// by also stripping any remaining trailing words that look like set context.
func stripSetName(name, setName string) string {
	upper := strings.ToUpper(name)
	setUpper := strings.ToUpper(setName)

	// Try exact set name match at the end.
	if idx := strings.LastIndex(upper, setUpper); idx > 0 {
		candidate := strings.TrimSpace(name[:idx])
		if candidate != "" {
			return candidate
		}
	}

	// Try matching individual significant words from set name (3+ chars)
	// to strip trailing set context from the card name.
	setWords := significantWords(setUpper)
	words := strings.Fields(upper)
	// Walk backwards: strip trailing words that appear in the set name.
	cutAt := len(words)
	for i := len(words) - 1; i >= 1; i-- {
		if containsWord(setWords, words[i]) {
			cutAt = i
		} else {
			break
		}
	}
	if cutAt < len(words) {
		originalWords := strings.Fields(name)
		return strings.TrimSpace(strings.Join(originalWords[:cutAt], " "))
	}
	return name
}

// significantWords returns words from s that are 3+ characters, filtering noise.
func significantWords(s string) []string {
	var result []string
	for _, w := range strings.Fields(s) {
		if len(w) >= 3 {
			result = append(result, w)
		}
	}
	return result
}

func containsWord(words []string, word string) bool {
	for _, w := range words {
		if w == word {
			return true
		}
	}
	return false
}

// mmVariationKeywords are known trailing keywords that represent card variations/rarities.
// Checked from longest to shortest to avoid partial matches.
var mmVariationKeywords = []string{
	"SPECIAL ILLUSTRATION RARE",
	"SPECIAL ART RARE",
	"ILLUSTRATION RARE",
	"ULTRA RARE",
	"MEGA HYPER RARE",
	"MEGA ATTACK RARE",
	"MEGA ULTRA RARE",
	"HYPER RARE",
	"ART RARE",
	"FULL ART",
	"PROMO",
	"1ST EDITION",
	"1ST ED.",
	"1ST ED",
	"PRERELEASE",
	"PRERELEASE-STAFF",
}

// stripVariationKeywords removes known variation/rarity keywords from the end,
// or truncates at the first occurrence of a keyword followed by non-name text.
func stripVariationKeywords(name string) string {
	upper := strings.ToUpper(strings.TrimSpace(name))

	// First pass: strip exact suffix matches (repeated for chained keywords).
	changed := true
	for changed {
		changed = false
		for _, kw := range mmVariationKeywords {
			if !strings.HasSuffix(upper, kw) {
				continue
			}
			trimLen := len(upper) - len(kw)
			name = strings.TrimSpace(name[:trimLen])
			upper = strings.ToUpper(name)
			changed = true
			break
		}
	}

	// Second pass: truncate at first standalone keyword occurrence
	// (handles cases like "GIRATINA PROMO-TEAM PLASMA" where PROMO
	// may be joined to following text by a dash).
	words := strings.Fields(upper)
	nameWords := strings.Fields(name)
	for i := 1; i < len(words); i++ {
		for _, kw := range mmVariationKeywords {
			kwFirst := strings.Fields(kw)[0]
			if words[i] == kwFirst || strings.HasPrefix(words[i], kwFirst+"-") {
				candidate := strings.TrimSpace(strings.Join(nameWords[:i], " "))
				if candidate != "" {
					return candidate
				}
			}
		}
	}

	return name
}

// mmTitleCase converts "DARK CHARIZARD" → "Dark Charizard",
// but preserves known Pokemon TCG tokens like "ex", "EX", "GX", "VMAX", etc.
func mmTitleCase(name string) string {
	words := strings.Fields(name)
	for i, w := range words {
		lower := strings.ToLower(w)
		// Preserve Pokemon TCG suffixes that have specific casing.
		switch lower {
		case "ex":
			words[i] = "ex" // lowercase "ex" is the modern SV-era convention
		case "gx":
			words[i] = "GX"
		case "vmax":
			words[i] = "VMAX"
		case "vstar":
			words[i] = "VSTAR"
		case "v":
			words[i] = "V"
		case "lv.x":
			words[i] = "Lv.X"
		default:
			// Title case: first letter upper, rest lower.
			if len(w) > 0 {
				words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
			}
		}
	}
	return strings.Join(words, " ")
}
