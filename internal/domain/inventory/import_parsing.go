package inventory

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

func isInvalidCardNumber(num string) bool {
	return constants.IsInvalidCardNumber(num)
}

// IsGenericSetName checks if a set name is generic (i.e., too vague for accurate pricing).
// Generic sets like "TCG Cards" are skipped to avoid capturing incorrect pricing data.
func IsGenericSetName(setName string) bool {
	return constants.IsGenericSetName(setName)
}

// Deprecated: use IsGenericSetName instead.
func isGenericSetName(setName string) bool {
	return IsGenericSetName(setName)
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
