package csvimport

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// psaCategoryToSetName maps known generic PSA categories to real TCG set names.
// PSA uses short category labels (e.g., "GAME") that don't correspond to
// standard set names, causing wrong-card matches during pricing.
var psaCategoryToSetName = map[string]string{
	"GAME":       "Base Set",
	"GAME MOVIE": "Promo", // Ancient Mew — PSA "GAME MOVIE" → "Promo"
}

// resolvePSACategory returns the real set name for a known generic PSA category,
// or the original setName if no mapping exists.
func resolvePSACategory(setName string) string {
	if mapped, ok := psaCategoryToSetName[strings.ToUpper(strings.TrimSpace(setName))]; ok {
		return mapped
	}
	return setName
}

// variantPattern pairs a variant name with its compiled word-boundary regex.
type variantPattern struct {
	name string
	re   *regexp.Regexp
}

// variantPatterns holds variant markers ordered longest-first so "REVERSE HOLO"
// is matched/stripped before "HOLO" and "1ST EDITION" before "1ST".
var variantPatterns = func() []variantPattern {
	variants := constants.VariantKeywords
	patterns := make([]variantPattern, len(variants))
	for i, v := range variants {
		patterns[i] = variantPattern{
			name: v,
			re:   regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(v) + `\b`),
		}
	}
	return patterns
}()

// stripVariantTokens removes variant marker phrases from a set name so they
// aren't duplicated when parseCardMetadataFromTitle appends them via
// extractVariantFromTitle. Uses word-boundary matching to avoid corrupting
// names that contain variant substrings (e.g., "HOLO" in "HOLON").
func stripVariantTokens(setName string) string {
	for _, vp := range variantPatterns {
		setName = vp.re.ReplaceAllString(setName, "")
	}
	// Collapse any spaces left after stripping
	return strings.Join(strings.Fields(setName), " ")
}

// extractCardNameFromPSATitle extracts the card name portion from a PSA listing title.
// Format: "YYYY POKEMON <SET_NAME> <CARD_NUMBER> <CARD_NAME> [DESCRIPTORS] PSA <GRADE>"
// Returns the card name portion (everything between card number and PSA grade),
// or empty string if parsing fails.
func extractCardNameFromPSATitle(title string) string {
	_, cardNum, cardIdx := parsePSAListingTitleWithIndex(title)
	if cardNum == "" || cardIdx < 0 {
		return ""
	}

	cleaned := gradeRegex.ReplaceAllString(title, "")
	cleaned = strings.TrimSpace(cleaned)

	tokens := strings.Fields(cleaned)
	if cardIdx+1 < len(tokens) {
		name := strings.Join(tokens[cardIdx+1:], " ")
		return strings.TrimSpace(name)
	}
	return ""
}

// parseCardMetadataFromTitle extracts card name, card number, set name, and year
// from a PSA listing title and CSV category. This is the fast path used during
// import (no API calls). Cert lookup enrichment happens asynchronously after import.
func parseCardMetadataFromTitle(listingTitle, category string) PSACardMetadata {
	meta := PSACardMetadata{
		SetName:  category,
		CardYear: extractYearFromTitle(listingTitle),
	}

	titleSet, titleNum := ParsePSAListingTitle(listingTitle)
	if titleNum != "" {
		meta.CardNumber = titleNum
	}

	if parsed := extractCardNameFromPSATitle(listingTitle); parsed != "" {
		meta.CardName = parsed
	} else if noNumName, noNumSet := parseNoNumberTitle(listingTitle); noNumName != "" {
		// Fallback for titles without collector numbers (e.g., Ancient Mew, promo cards).
		// Extract card name and set name from the title tokens directly.
		meta.CardName = noNumName
		if noNumSet != "" {
			meta.SetName = noNumSet
		}
	} else {
		meta.CardName = listingTitle
		meta.ParseWarning = "card name extraction failed, using raw listing title"
	}

	// Strip promo collection suffixes from card names — PSA cert Subject
	// often includes collection info (e.g., "UMBREON EX PRISMATIC EVOLUTIONS
	// PREMIUM FIGURE COLLECTION") which pollutes pricing queries.
	meta.CardName = stripCollectionSuffix(meta.CardName)

	// Append variant keywords (1ST EDITION, SHADOWLESS) to card name
	if v := extractVariantFromTitle(listingTitle); v != "" {
		upper := strings.ToUpper(meta.CardName)
		if !strings.Contains(upper, v) {
			meta.CardName = meta.CardName + " " + v
		}
	}

	var setWarning string
	meta.SetName, setWarning = resolveSetName(titleSet, meta.SetName)
	if setWarning != "" && meta.ParseWarning == "" {
		meta.ParseWarning = setWarning
	}

	return meta
}

// resolveSetName applies PSA category resolution and generic set fallback.
// category is the original PSA category (e.g., "GAME"); titleSet is the
// set name parsed from the listing title. Returns the resolved set name
// and an optional warning.
func resolveSetName(titleSet, category string) (string, string) {
	// Apply known PSA category mappings (e.g., "GAME" → "Base Set").
	resolved := resolvePSACategory(category)

	// When set is still generic after mapping, use the parsed title set
	if isGenericSetName(resolved) && titleSet != "" {
		resolved = titleSet
	}

	// Apply mapping again — titleSet may itself be a known PSA category
	// (e.g., "GAME" from listing title → "Base Set")
	resolved = resolvePSACategory(resolved)

	// Determine warning after all resolution steps so a generic titleSet
	// that remains generic after resolvePSACategory is correctly flagged.
	var warning string
	if isGenericSetName(resolved) {
		warning = "set name remains generic after resolution"
	}

	return resolved, warning
}

// stripCollectionSuffix removes promo collection names from card names using
// the pre-partitioned anywhereSuffixes / trailingSuffixes slices.
// PSA cert Subject includes these but they pollute pricing queries.
// E.g., "UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION" → "UMBREON EX"
func stripCollectionSuffix(name string) string {
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

func ExportParseCardMetadataFromTitle(listingTitle, category string) (string, string, string) {
	meta := parseCardMetadataFromTitle(listingTitle, category)
	return meta.CardName, meta.CardNumber, meta.SetName
}

func ExportIsGenericSetName(setName string) bool {
	return isGenericSetName(setName)
}

// extractYearFromTitle extracts the leading 4-digit year from a PSA listing title.
// Returns 0 if no year is found. Example: "2000 POKEMON ROCKET..." → 2000.
func extractYearFromTitle(title string) int {
	tokens := strings.Fields(title)
	if len(tokens) == 0 {
		return 0
	}
	if len(tokens[0]) == 4 {
		if y, err := strconv.Atoi(tokens[0]); err == nil && y >= 1990 && y <= 2039 {
			return y
		}
	}
	return 0
}

// extractVariantFromTitle detects variant keywords in PSA listing titles
// that may not be present in the PSA cert's Variety field.
// Uses word-boundary regexes (variantPatterns) to avoid false positives
// like matching "HOLO" inside "HOLON". Returns the first matched variant
// keyword, or empty string.
func extractVariantFromTitle(title string) string {
	// variantPatterns are ordered longest-first so "REVERSE HOLO" matches
	// before "HOLO" and "1ST EDITION" before "1ST".
	for _, vp := range variantPatterns {
		if vp.re.MatchString(title) {
			return vp.name
		}
	}
	return ""
}
