package cardutil

import (
	"strconv"
	"strings"
)

// ExtractEraPrefix extracts a known era code from a promo set name.
// Only returns a prefix if the set name contains "promo" (case-insensitive)
// and starts with a known era token.
// e.g., "SWSH BLACK STAR PROMO" -> "SWSH", "SM BLACK STAR PROMO" -> "SM"
func ExtractEraPrefix(setName string) string {
	if !strings.Contains(strings.ToLower(setName), "promo") {
		return ""
	}
	for _, token := range strings.Fields(setName) {
		upper := strings.ToUpper(token)
		if KnownEraTokens[upper] {
			return upper
		}
	}
	return ""
}

// StripPSASetCode removes PSA-style set code prefixes like "SWSH45 EN ", "SV06 EN ", "PRE EN "
// from a set name. These internal PSA category codes don't appear in price API set names.
func StripPSASetCode(setName string) string {
	if psaSetCodeRegex.MatchString(setName) {
		return strings.TrimSpace(psaSetCodeRegex.ReplaceAllString(setName, ""))
	}
	return setName
}

// NormalizeSetNameSimple normalizes a Pokemon TCG set name without adding "Pokemon" prefix.
// This is for Pokemon-specific APIs (like PokemonPriceTracker) that already
// know they're dealing with Pokemon cards and use standard TCG set naming.
// Strips "JAPANESE" prefix (use NormalizeSetNameForSearch to preserve it).
//
// Examples:
//   - "151" -> "151"
//   - "Scarlet & Violet" -> "Scarlet Violet" (removes special chars)
//   - "Scarlet & Violet: 151" -> "Scarlet Violet 151"
//   - "2013 Pokemon Black & White Promos" -> "Black White Promos"
//   - "" -> "" (empty stays empty)
func NormalizeSetNameSimple(setName string) string {
	return normalizeSetNameBase(setName, true)
}

// NormalizeSetNameForSearch normalizes a set name for external search APIs
// that distinguish between Japanese and English cards. Unlike NormalizeSetNameSimple,
// this preserves the "JAPANESE" prefix so APIs can filter by language.
func NormalizeSetNameForSearch(setName string) string {
	return normalizeSetNameBase(setName, false)
}

// StripCommonSetPrefixes performs the set name normalization steps shared by
// both the cardutil and PriceCharting pipelines: strip colons, replace hyphens
// with spaces, strip leading year, strip "Pokemon"/"Pokémon" prefix, and
// normalize Chinese PSA set codes.
//
// Callers handle ampersand treatment differently (&->remove vs &->"and") and
// apply their own post-processing (Japanese handling, PSA codes, era expansion).
func StripCommonSetPrefixes(setName string) string {
	if setName == "" {
		return ""
	}

	// Strip : and replace - with space (shared across both pipelines)
	cleaned := strings.ReplaceAll(setName, ":", "")
	cleaned = strings.ReplaceAll(cleaned, "-", " ")

	// Normalize whitespace
	cleaned = strings.Join(strings.Fields(cleaned), " ")

	// Strip leading 4-digit year (PSA categories often start with year, e.g. "2013 ...")
	cleaned = stripLeadingYear(cleaned)

	// Strip "Pokemon"/"Pokémon" prefix
	cleaned = stripPokemonPrefix(cleaned)

	// Normalize Chinese PSA set codes (CN, SIMPLIFIED CHINESE, etc.)
	cleaned, _ = NormalizeChineseSetName(cleaned)

	return cleaned
}

// stripLeadingYear removes a leading 4-digit year followed by a space.
func stripLeadingYear(s string) string {
	if len(s) >= 5 && s[4] == ' ' {
		allDigits := true
		for _, c := range s[:4] {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return strings.TrimSpace(s[5:])
		}
	}
	return s
}

// stripPokemonPrefix removes a leading "Pokemon " or "Pokémon " prefix.
func stripPokemonPrefix(s string) string {
	lower := strings.ToLower(s)
	for _, prefix := range []string{"pokemon ", "pokémon "} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(s[len(prefix):])
		}
	}
	return s
}

// normalizeSetNameBase is the shared implementation for set name normalization.
// When stripJapanese is true, the "JAPANESE" prefix is removed.
func normalizeSetNameBase(setName string, stripJapanese bool) string {
	if setName == "" {
		return ""
	}

	// Cardutil removes ampersands (not "and") before shared processing
	cleaned := strings.ReplaceAll(setName, "&", "")

	// Apply shared normalization steps
	cleaned = StripCommonSetPrefixes(cleaned)

	// Detect "JAPANESE " prefix for both stripping and PSA regex handling.
	lower := strings.ToLower(cleaned)
	hasJapanesePrefix := strings.HasPrefix(lower, "japanese ")

	if stripJapanese && hasJapanesePrefix {
		cleaned = strings.TrimSpace(cleaned[len("japanese "):])
	}

	// Strip PSA-style set code prefixes (e.g. "PRE EN ", "M24 EN ", "SV06 EN ").
	// When input has a leading "Japanese " that won't be stripped, test the
	// portion after it so anchored codes like "JAPANESE SWSH45 EN ..." are
	// still detected and removed.
	psaTarget := cleaned
	if hasJapanesePrefix && !stripJapanese {
		psaTarget = strings.TrimSpace(cleaned[len("japanese "):])
	}
	if psaSetCodeRegex.MatchString(psaTarget) {
		stripped := strings.TrimSpace(psaSetCodeRegex.ReplaceAllString(psaTarget, ""))
		if hasJapanesePrefix && !stripJapanese {
			cleaned = strings.TrimSpace(cleaned[:len("japanese ")] + stripped)
		} else {
			cleaned = stripped
		}
	}

	return cleaned
}

// MatchesSetOverlap returns true if resultSet shares significant whole tokens
// with expectedSet. Both names are normalized (lowered, punctuation stripped)
// and compared by exact token equality -- not substring containment -- to avoid
// false positives (e.g., "Expedition" inside "Expedition Base").
// Era abbreviations (SV, SWSH, SM, etc.) are expanded to their full names
// so "SV BLACK STAR PROMO" can match "Scarlet & Violet Promos".
// For multi-word expected sets (2+ significant tokens), requires at least 2
// exact token matches so that short set names don't over-match.
func MatchesSetOverlap(resultSet, expectedSet string) bool {
	if expectedSet == "" {
		return true // nothing expected -- auto-pass
	}
	if resultSet == "" {
		return false // expected data but provider returned nothing
	}

	resultTokens := expandEraTokens(normalizeSetTokens(resultSet))
	expectedTokens := expandEraTokens(normalizeSetTokens(expectedSet))

	// Build set of result tokens for O(1) lookup
	resultMap := make(map[string]struct{}, len(resultTokens))
	for _, t := range resultTokens {
		resultMap[t] = struct{}{}
	}

	// Collect significant expected tokens (length >= 3)
	var significant []string
	for _, t := range expectedTokens {
		if len(t) >= 3 {
			significant = append(significant, t)
		}
	}

	matches := 0
	for _, t := range significant {
		if tokenMatchesFuzzy(t, resultMap) {
			matches++
		}
	}

	// Multi-word expected sets require at least 2 matches to reduce false positives
	if len(significant) > 1 {
		return matches >= 2
	}
	return matches > 0
}

// expandEraTokens expands short era abbreviation tokens (e.g. "sv" -> ["scarlet", "violet"])
// using the EraExpansions map. This allows set overlap matching to work when one side
// uses an abbreviation and the other uses the full name.
func expandEraTokens(tokens []string) []string {
	var expanded []string
	for _, t := range tokens {
		upper := strings.ToUpper(t)
		if exp, ok := EraExpansions[upper]; ok {
			expanded = append(expanded, strings.Fields(strings.ToLower(exp))...)
		} else {
			expanded = append(expanded, t)
		}
	}
	return expanded
}

// normalizeSetTokens lowercases s, replaces non-alphanumeric chars with spaces,
// and returns the resulting tokens.
func normalizeSetTokens(s string) []string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// tokenMatchesFuzzy checks if token is present in resultMap, also considering
// singular/plural variants (promo<->promos, island<->islands, etc.).
func tokenMatchesFuzzy(token string, resultMap map[string]struct{}) bool {
	if _, ok := resultMap[token]; ok {
		return true
	}
	// Check plural form: "promo" -> "promos"
	if _, ok := resultMap[token+"s"]; ok {
		return true
	}
	// Check singular form: "promos" -> "promo"
	if strings.HasSuffix(token, "s") && len(token) > 3 {
		if _, ok := resultMap[token[:len(token)-1]]; ok {
			return true
		}
	}
	return false
}

// ExtractSetKeyword returns the most significant keyword from a normalized set name.
// Skips generic words like "set", "cards", "pokemon", etc. Returns the first
// remaining token with length >= 3, or empty string if none found.
func ExtractSetKeyword(normalizedSet string) string {
	for _, tok := range strings.Fields(normalizedSet) {
		if len(tok) < 3 {
			continue
		}
		if genericSetWords[strings.ToLower(tok)] {
			continue
		}
		return tok
	}
	return ""
}

// MissingSetTokens returns significant expected tokens NOT present in the result set.
// Generic words are excluded (see genericSetWords).
// Both sets are normalized (lowered, punctuation stripped) before comparison.
func MissingSetTokens(resultSet, expectedSet string) []string {
	// Note: PSA set codes (svp, en, swsh, etc.) are NOT excluded here.
	// Instead, callers should normalize the expected set via NormalizeSetNameForSearch()
	// before calling MissingSetTokens, which strips PSA codes generically.

	resultTokens := normalizeSetTokens(resultSet)
	resultMap := make(map[string]struct{}, len(resultTokens))
	for _, t := range resultTokens {
		resultMap[t] = struct{}{}
	}

	expectedTokens := normalizeSetTokens(expectedSet)
	var missing []string
	for _, t := range expectedTokens {
		if len(t) < 2 {
			continue
		}
		if genericSetWords[t] {
			continue
		}
		if !tokenMatchesFuzzy(t, resultMap) {
			missing = append(missing, t)
		}
	}
	return missing
}

// NormalizeChineseSetName normalizes Chinese PSA set code prefixes to a canonical form.
// Handles "SIMPLIFIED CHINESE CBBx C ...", "TRADITIONAL CHINESE CBBx C ...", "CN CBBx C ...",
// and "CHINESE CBBx C ..." patterns. Returns the normalized name and whether it was Chinese.
//
// Examples:
//   - "SIMPLIFIED CHINESE CBB2 C GEM PACK VOL 2" -> "Chinese GEM PACK VOL 2", true
//   - "CN CBB1 C GEM PACK VOL 1" -> "Chinese GEM PACK VOL 1", true
//   - "Sword Shield" -> "Sword Shield", false
func NormalizeChineseSetName(setName string) (string, bool) {
	lower := strings.ToLower(setName)

	// Strip "SIMPLIFIED" / "TRADITIONAL" qualifier, leaving "CHINESE ..."
	if strings.HasPrefix(lower, "simplified chinese ") || strings.HasPrefix(lower, "traditional chinese ") {
		spaceIdx := strings.Index(lower, " ")
		setName = strings.TrimSpace(setName[spaceIdx+1:])
		lower = strings.ToLower(setName)
	}

	if strings.HasPrefix(lower, "chinese ") {
		rest := setName[len("chinese "):]
		if idx := strings.Index(strings.ToUpper(rest), " C "); idx > 0 {
			rest = strings.TrimSpace(rest[idx+3:])
		}
		return "Chinese " + rest, true
	}
	if strings.HasPrefix(lower, "cn ") {
		rest := setName[3:]
		if idx := strings.Index(strings.ToUpper(rest), " C "); idx > 0 {
			rest = strings.TrimSpace(rest[idx+3:])
		}
		return "Chinese " + rest, true
	}

	return setName, false
}

// IsChineseSet returns true if the set name indicates a Chinese card.
// Chinese Gem Pack cards use species-based numbering in PriceCharting
// (e.g., PSA #09 -> PC #709 where 7 = species position).
func IsChineseSet(setName string) bool {
	lower := strings.ToLower(setName)
	return strings.HasPrefix(lower, "cn ") || strings.Contains(lower, "chinese")
}

// IsChineseGemPackSet returns true if the set name matches a known Chinese Gem Pack volume
// (CBB1/Vol 1, CBB2/Vol 2, CBB3/Vol 3). Only these volumes have species-based numbering
// in PriceCharting. Requires the set to also be Chinese to avoid false positives on
// non-Chinese sets that happen to contain "Vol 1" etc.
func IsChineseGemPackSet(setName string) bool {
	if !IsChineseSet(setName) {
		return false
	}
	lower := strings.ToLower(setName)
	return strings.Contains(lower, "cbb1") || strings.Contains(lower, "vol 1") ||
		strings.Contains(lower, "cbb2") || strings.Contains(lower, "vol 2") ||
		strings.Contains(lower, "cbb3") || strings.Contains(lower, "vol 3")
}

// chineseGemPackBases maps known Chinese Gem Pack volume identifiers to their
// PriceCharting species-position base numbers.
// PriceCharting uses species-position-based numbering (species N -> base N*100),
// so only volumes with known card->species mappings work here.
//
// To add a new volume: look up the species-position base on PriceCharting's API
// (search for a card in the new volume and observe its PC number), then add both
// the CBBx and "vol N" keyword entries here with the corresponding base.
//
// Known mappings (from PriceCharting API verification):
//   - CBB1 / Vol 1: Captain Pikachu is species 7 -> base 700
//   - CBB2 / Vol 2: Umbreon is species 6 -> base 600
//   - CBB3 / Vol 3: Gengar is species 3 -> base 300
var chineseGemPackBases = []struct {
	keyword string
	base    int
}{
	{"cbb1", 700}, {"vol 1", 700},
	{"cbb2", 600}, {"vol 2", 600},
	{"cbb3", 300}, {"vol 3", 300},
}

// MapChineseNumber maps PSA printed card numbers to PriceCharting's numbering
// for known Chinese Gem Pack volumes. Cards not covered need price hints.
// Returns ("", true) for unknown volumes to signal callers to log a warning.
func MapChineseNumber(setName, number string) (string, bool) {
	if number == "" {
		return "", false
	}
	// Extract the first contiguous digit sequence to handle formats like "009/015", "#09".
	digits := NumericOnly(number)
	if digits == "" {
		return number, false
	}
	trimmed := strings.TrimLeft(digits, "0")
	if trimmed == "" {
		return number, false
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n <= 0 {
		return number, false
	}

	lower := strings.ToLower(setName)
	for _, entry := range chineseGemPackBases {
		if strings.Contains(lower, entry.keyword) {
			return strconv.Itoa(entry.base + n), false
		}
	}
	// Unknown volume -- can't determine species base. Return empty
	// to trigger number-less search (better than wrong number).
	return "", true
}
