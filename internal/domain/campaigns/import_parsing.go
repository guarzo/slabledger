package campaigns

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// isInvalidCardNumber delegates to the shared implementation in constants.
func isInvalidCardNumber(num string) bool {
	return constants.IsInvalidCardNumber(num)
}

// isGenericSetName delegates to the centralized generic set check.
func isGenericSetName(setName string) bool {
	return constants.IsGenericSetName(setName)
}

// psaCategoryToSetName maps known generic PSA categories to real TCG set names.
// PSA uses short category labels (e.g., "GAME") that don't correspond to
// PriceCharting set names, causing wrong-card matches during pricing.
var psaCategoryToSetName = map[string]string{
	"GAME":       "Base Set",
	"GAME MOVIE": "Promo", // Ancient Mew — PSA "GAME MOVIE" → PriceCharting "Pokemon Promo"
}

// resolvePSACategory returns the real set name for a known generic PSA category,
// or the original setName if no mapping exists.
func resolvePSACategory(setName string) string {
	if mapped, ok := psaCategoryToSetName[strings.ToUpper(strings.TrimSpace(setName))]; ok {
		return mapped
	}
	return setName
}

var (
	// Matches #-prefixed card numbers like "#25/25", "#68", "#TG23" anywhere in title
	hashCardNumberRegex = regexp.MustCompile(`#([A-Za-z0-9-]+(?:/[A-Za-z0-9-]+)?)`)
	// Matches bare card numbers like "4/102", "6", or "TG23" immediately before "PSA \d".
	// Must contain at least one digit to avoid matching plain words like "number".
	bareCardNumberRegex = regexp.MustCompile(`(?i)([A-Za-z]*\d[A-Za-z0-9-]*(?:/[A-Za-z0-9-]+)?)\s+PSA\s*\d`)
)

// ExtractCardNumberFromPSATitle extracts a card number from a PSA listing title.
// First looks for #-prefixed numbers (e.g. "#25/25"), then falls back to a number
// immediately before "PSA \d" (e.g. "6 PSA 10").
// Returns empty string if no card number is found.
func ExtractCardNumberFromPSATitle(title string) string {
	if m := hashCardNumberRegex.FindStringSubmatch(title); len(m) >= 2 {
		return m[1]
	}
	if m := bareCardNumberRegex.FindStringSubmatch(title); len(m) >= 2 {
		return m[1]
	}
	return ""
}

var gradeRegex = regexp.MustCompile(`(?i)\bPSA\s*(\d{1,2}(?:\.\d+)?)\b`)

// ExtractGrade attempts to extract a PSA grade from a card title string.
// Returns 0 if no grade is found. Preserves half-grades (e.g. 8.5, 9.5).
// NOTE: This function is PSA-only — it matches the pattern "PSA <grade>".
// For multi-grader support (CGC, BGS, SGC), use ExtractGraderAndGrade instead.
func ExtractGrade(title string) float64 {
	matches := gradeRegex.FindStringSubmatch(title)
	if len(matches) < 2 {
		return 0
	}
	grade, err := strconv.ParseFloat(matches[1], 64)
	if err != nil || grade < 1 || grade > 10 {
		return 0
	}
	return grade
}

// cardNumberTokenRegex matches standalone collector number tokens like "56", "176", "25/25", "TG23", "SWSH123".
// Requires at least one digit to distinguish from pure-alpha set name words.
var cardNumberTokenRegex = regexp.MustCompile(`^[A-Za-z0-9-]*\d[A-Za-z0-9-]*(?:/[A-Za-z0-9-]+)?$`)

// ParsePSAListingTitle extracts set name and card number from a PSA communication
// spreadsheet listing title.
// Format: "YYYY POKEMON <SET_NAME> <CARD_NUMBER> <CARD_NAME> [DESCRIPTORS] PSA <GRADE>"
// Returns (setName, cardNumber) — empty strings if parsing fails.
func ParsePSAListingTitle(title string) (string, string) {
	setName, cardNum, _ := parsePSAListingTitleWithIndex(title)
	return setName, cardNum
}

// parsePSAListingTitleWithIndex extracts set name, card number, and the token index
// of the card number from a PSA listing title. The token index is relative to the
// grade-stripped, tokenized title. Returns ("", "", -1) if parsing fails.
func parsePSAListingTitleWithIndex(title string) (string, string, int) {
	// Strip trailing PSA grade
	cleaned := gradeRegex.ReplaceAllString(title, "")
	cleaned = strings.TrimSpace(cleaned)

	tokens := strings.Fields(cleaned)
	if len(tokens) < 3 {
		return "", "", -1
	}

	start := 0
	// Skip leading 4-digit year
	if len(tokens[0]) == 4 {
		if _, err := strconv.Atoi(tokens[0]); err == nil {
			start++
		}
	}
	// Skip "POKEMON" keyword
	if start < len(tokens) && strings.EqualFold(tokens[start], "POKEMON") {
		start++
	}

	// Search right-to-left for the last numeric token followed by an alphabetic token.
	// Iterating backwards ensures later collector numbers (e.g., "199/165") win over
	// earlier numeric set names (e.g., "151" in "Pokemon 151 Pikachu 199/165").
	// Skip variant keywords like "1ST" (from "1ST EDITION") and 4-digit years.
	for i := len(tokens) - 2; i >= start; i-- {
		if cardNumberTokenRegex.MatchString(tokens[i]) && len(tokens[i+1]) > 0 && isLetter(tokens[i+1][0]) {
			if isInvalidCardNumber(tokens[i]) {
				continue
			}
			// Reject tokens that look like Pokémon names with trailing digits
			// (e.g. "PORYGON2") — real collector numbers either start with a
			// digit or have a short alpha prefix (≤4 chars, e.g. "TG23", "BW93").
			if !looksLikeCollectorNumber(tokens[i]) {
				continue
			}
			setName := stripVariantTokens(strings.Join(tokens[start:i], " "))
			return strings.TrimSpace(setName), tokens[i], i
		}
	}

	// Fallback: check the last token as a candidate card number.
	// Handles titles like "TG23/TG30 PSA 10" where the number is the final token
	// after grade stripping. Use the token directly since cardNumberTokenRegex
	// already validates the pattern (ExtractCardNumberFromPSATitle requires
	// "#" prefix or trailing "PSA \d" which aren't present on bare tokens).
	if last := tokens[len(tokens)-1]; cardNumberTokenRegex.MatchString(last) && !isInvalidCardNumber(last) && looksLikeCollectorNumber(last) {
		setName := stripVariantTokens(strings.Join(tokens[start:len(tokens)-1], " "))
		return strings.TrimSpace(setName), last, len(tokens) - 1
	}

	return "", "", -1
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isLetterOrDigit(b byte) bool {
	return isLetter(b) || (b >= '0' && b <= '9')
}

// maxCollectorNumberAlphaPrefix is the maximum number of leading alphabetic
// characters allowed before the first digit in a collector number token.
// Real collector numbers have short prefixes like "TG" (2), "BW" (2), "SWSH" (4).
// Pokémon names like "PORYGON2" (7 alpha chars) exceed this limit and are rejected.
const maxCollectorNumberAlphaPrefix = 4

// looksLikeCollectorNumber returns true when a token resembles a collector
// number rather than a Pokémon name that happens to contain digits.
// Collector numbers either start with a digit ("123", "25/25") or have a
// short alphabetic prefix of at most maxCollectorNumberAlphaPrefix characters
// (e.g. "TG23", "BW93", "SWSH123").
// Pokémon names like "PORYGON2" have long alpha prefixes and are rejected.
func looksLikeCollectorNumber(token string) bool {
	if len(token) == 0 {
		return false
	}
	if token[0] >= '0' && token[0] <= '9' {
		return true
	}
	// Count leading alpha characters before the first digit
	for i := 0; i < len(token); i++ {
		if token[i] >= '0' && token[i] <= '9' {
			return i <= maxCollectorNumberAlphaPrefix
		}
	}
	return false
}

// noNumberCardPatterns lists known Pokémon names that appear in PSA titles
// without collector numbers. Each entry is matched case-insensitively as a
// contiguous phrase in the grade-stripped token list.
var noNumberCardPatterns = []string{
	"ANCIENT MEW",
	"DARK MEWTWO",
}

// noNumberStopWords are tokens that mark the end of the card name in a
// no-number title. Everything after the card name pattern until a stop word
// is appended to the card name (e.g., "POKKEN TOURNAMENT" for Dark Mewtwo).
var noNumberStopWords = map[string]bool{
	"POKEMON": true, "PSA": true, "JAPAN": true, "JAPANESE": true,
}

// parseNoNumberTitle handles PSA titles that have no collector number at all
// (e.g., "2000 POKEMON GAME MOVIE ANCIENT MEW POKEMON 2000 MOVIE PSA 9").
// Returns (cardName, setName) extracted from the title tokens.
// Returns ("", "") if the title can't be parsed.
func parseNoNumberTitle(title string) (string, string) {
	// Strip trailing PSA grade
	cleaned := gradeRegex.ReplaceAllString(title, "")
	cleaned = strings.TrimSpace(cleaned)
	upper := strings.ToUpper(cleaned)

	for _, pattern := range noNumberCardPatterns {
		idx := strings.Index(upper, pattern)
		if idx < 0 {
			continue
		}
		// Require word boundaries to avoid substring matches (e.g., "MEW" inside "MEWTWO").
		if idx > 0 && isLetterOrDigit(upper[idx-1]) {
			continue
		}
		if end := idx + len(pattern); end < len(upper) && isLetterOrDigit(upper[end]) {
			continue
		}

		cardName := cleaned[idx : idx+len(pattern)]

		// Everything before the card name pattern (after year + POKEMON) is the set.
		prefix := strings.TrimSpace(cleaned[:idx])
		tokens := strings.Fields(prefix)
		start := 0
		if len(tokens) > 0 {
			if _, err := strconv.Atoi(tokens[0]); err == nil && len(tokens[0]) == 4 {
				start++
			}
		}
		if start < len(tokens) && strings.EqualFold(tokens[start], "POKEMON") {
			start++
		}
		setName := ""
		if start < len(tokens) {
			setName = strings.Join(tokens[start:], " ")
		}

		// Append descriptor tokens after the pattern until a stop word or end.
		// E.g., "DARK MEWTWO POKKEN TOURNAMENT" — "POKKEN TOURNAMENT" is part of the name.
		afterIdx := idx + len(pattern)
		if afterIdx < len(upper) {
			afterTokens := strings.Fields(cleaned[afterIdx:])
			for _, t := range afterTokens {
				if noNumberStopWords[strings.ToUpper(t)] {
					break
				}
				cardName += " " + t
			}
		}

		return strings.TrimSpace(cardName), strings.TrimSpace(setName)
	}
	return "", ""
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
	// Use parsePSAListingTitleWithIndex to find the card number and its token position
	_, cardNum, cardIdx := parsePSAListingTitleWithIndex(title)
	if cardNum == "" || cardIdx < 0 {
		return ""
	}

	// Strip PSA grade suffix
	cleaned := gradeRegex.ReplaceAllString(title, "")
	cleaned = strings.TrimSpace(cleaned)

	// Use the parsed token index to slice everything after the card number
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

	// Parse listing title for set name and card number
	titleSet, titleNum := ParsePSAListingTitle(listingTitle)
	if titleNum != "" {
		meta.CardNumber = titleNum
	}

	// Extract card name from listing title (everything after card number)
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
	for {
		matched := false
		for _, cs := range trailingSuffixes {
			pat := " " + cs.Pattern
			if strings.HasSuffix(upper, pat) {
				cutLen := len(pat)
				stripped := strings.TrimSpace(name[:len(name)-cutLen])
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

// ExportParseCardMetadataFromTitle is a test-only exported wrapper used by
// integration tests in internal/integration/. Not for production use.
func ExportParseCardMetadataFromTitle(listingTitle, category string) (string, string, string) {
	meta := parseCardMetadataFromTitle(listingTitle, category)
	return meta.CardName, meta.CardNumber, meta.SetName
}

// ExportIsGenericSetName is a test-only exported wrapper used by
// integration tests in internal/integration/. Not for production use.
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
