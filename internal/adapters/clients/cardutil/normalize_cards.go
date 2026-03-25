package cardutil

import (
	"strings"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// NormalizeCardName cleans up card names for API lookups by removing
// bracket modifiers ([Reverse Holo], [Jumbo], etc.) and embedded card numbers (#30, #201).
// This is needed because PokeTCG API card names include these suffixes which external
// price APIs don't recognize.
func NormalizeCardName(name string) string {
	// Remove bracket modifiers like [Reverse Holo], [Jumbo]
	cleaned := bracketModifierRegex.ReplaceAllString(name, " ")
	// Remove embedded card numbers like #30, #201
	cleaned = cardNumberRegex.ReplaceAllString(cleaned, "")
	// Normalize whitespace
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.TrimSpace(cleaned)
}

// ExtractCollectorNumber extracts the collector number from a card name.
// Card names often contain embedded collector numbers like "Charizard ex #199" or "Zeraora VMAX #GG42".
// Returns the number without the # prefix, or empty string if not found.
func ExtractCollectorNumber(cardName string) string {
	// First remove bracket modifiers as they can contain numbers
	cleaned := bracketModifierRegex.ReplaceAllString(cardName, " ")
	cleaned = strings.TrimSpace(cleaned)

	matches := collectorNumberExtractRegex.FindStringSubmatch(cleaned)
	if len(matches) >= 2 {
		return matches[1] // Return captured group (number without #)
	}
	return ""
}

// NormalizePurchaseName cleans up PSA-style purchase names for DB/cache lookups.
// Expands abbreviations (-HOLO -> Holo, -REV.FOIL -> Reverse Foil) and replaces
// remaining hyphens with spaces, then applies NormalizeCardName to remove
// bracket modifiers and embedded card numbers.
//
// Examples:
//
//	"DARK GYARADOS-HOLO"  -> "DARK GYARADOS Holo"
//	"MEWTWO-REV.FOIL"     -> "MEWTWO Reverse Foil"
//	"Charizard ex #161"   -> "Charizard ex"
func NormalizePurchaseName(name string) string {
	// Expand PSA abbreviations (case-insensitive).
	lower := strings.ToLower(name)
	for _, pair := range PSAAbbreviations {
		if idx := strings.Index(lower, pair[0]); idx >= 0 {
			name = name[:idx] + pair[1] + name[idx+len(pair[0]):]
			lower = strings.ToLower(name)
		}
	}
	// Replace remaining hyphens with spaces (PSA uses "UMBREON-HOLO" style)
	name = strings.ReplaceAll(name, "-", " ")
	// Apply standard normalization (remove brackets, embedded numbers, collapse whitespace)
	return NormalizeCardName(name)
}

// StripVariantSuffix removes trailing variant descriptors from card names
// so that search APIs (e.g., PokemonPrice) can match the base card name.
// Card number matching handles finding the correct variant.
//
// Examples:
//
//	"MEWTWO Reverse Foil" -> "MEWTWO"
//	"DARK GYARADOS Holo"  -> "DARK GYARADOS"
//	"Holon Phantoms"      -> "Holon Phantoms" (not stripped -- "Holon" is part of name)
func StripVariantSuffix(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range []string{" reverse foil", " reverse holo", " holo"} {
		if strings.HasSuffix(lower, suffix) {
			return strings.TrimSpace(name[:len(name)-len(suffix)])
		}
	}
	return name
}

// IsInvalidCardNumber delegates to the shared implementation in constants.
// These bogus numbers come from legacy PSA-title parsing
// and cause match rejections during price lookups.
func IsInvalidCardNumber(num string) bool {
	return constants.IsInvalidCardNumber(num)
}

// SimplifyForSearch extracts the core TCG card name by removing PSA listing noise.
// Called after NormalizePurchaseName + StripVariantSuffix to produce a concise
// search term for APIs like CardHedger.
//
// Algorithm:
//  1. Deduplicate repeated name+type pattern (e.g. "Charizard ex Charizard ex SUPER PREM COLL" -> "Charizard ex SUPER PREM COLL")
//  2. Truncate after first card type keyword (ex, EX, GX, V, VMAX, VSTAR, BREAK) at word position >= 1
//  3. If no type suffix found, strip trailing noise words (collection/rarity/edition/set leakage tokens)
func SimplifyForSearch(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	words := strings.Fields(name)

	// Step 1: Deduplicate repeated name+type prefix.
	// e.g. ["CHARIZARD","ex","CHARIZARD","ex","SUPER","PREM","COLL"]
	// First 2 words repeat at position 2 -> keep from position 2 onward.
	words = deduplicatePrefix(words)

	// Step 2: Truncate after type suffix at position >= 1.
	if idx := findTypeSuffixIndex(words); idx >= 1 && idx < len(words)-1 {
		// Type suffix found and there are more words after it -- truncate.
		words = words[:idx+1]
		return strings.Join(words, " ")
	}
	// If the type suffix is the last word, nothing to truncate -- fall through to noise stripping.

	// Step 3: Strip trailing noise words (only when no type suffix truncation happened).
	words = stripTrailingNoise(words)

	return strings.Join(words, " ")
}

// deduplicatePrefix detects if the first N words repeat at position N and removes the duplicate.
func deduplicatePrefix(words []string) []string {
	n := len(words)
	for prefixLen := 1; prefixLen <= n/2; prefixLen++ {
		match := true
		for i := 0; i < prefixLen; i++ {
			if !strings.EqualFold(words[i], words[prefixLen+i]) {
				match = false
				break
			}
		}
		if match {
			// Remove the duplicate: keep from position prefixLen onward
			return words[prefixLen:]
		}
	}
	return words
}

// findTypeSuffixIndex returns the index of the first card type keyword at position >= 1.
// Returns -1 if not found.
func findTypeSuffixIndex(words []string) int {
	for i := 1; i < len(words); i++ {
		if cardTypeSuffixes[words[i]] {
			return i
		}
	}
	return -1
}

// stripTrailingNoise removes trailing noise words, keeping at least the first token.
func stripTrailingNoise(words []string) []string {
	for len(words) > 1 {
		last := words[len(words)-1]
		if noiseWords[strings.ToUpper(last)] || noiseWords[last] {
			words = words[:len(words)-1]
		} else {
			break
		}
	}
	return words
}

// NumericOnly strips any leading letter prefix from a normalized card number,
// returning just the digit portion. Used as a fallback comparison when letter
// prefixes differ between sources (e.g., expected "75" vs product "SWSH75").
// Returns empty string if no digits are found.
func NumericOnly(number string) string {
	for i, c := range number {
		if c >= '0' && c <= '9' {
			// Scan forward collecting only contiguous digits
			end := i + 1
			for end < len(number) && number[end] >= '0' && number[end] <= '9' {
				end++
			}
			return number[i:end]
		}
	}
	return ""
}

// NormalizeCardNumber converts Pokemon TCG API format numbers to simple collector numbers.
// Examples:
//   - "006/165" -> "6" (removes leading zeros and denominator)
//   - "GG42/GG70" -> "GG42" (keeps prefix, removes denominator)
//   - "201" -> "201" (already simple)
//   - "SV151" -> "SV151" (keeps as-is)
func NormalizeCardNumber(number string) string {
	if number == "" {
		return ""
	}

	// Remove denominator if present (e.g., "006/165" -> "006")
	if idx := strings.Index(number, "/"); idx > 0 {
		number = number[:idx]
	}

	// Try to parse as integer to remove leading zeros
	// But keep any letter prefix (like "GG" in "GG42")

	// Find where digits start
	digitStart := -1
	for i, c := range number {
		if c >= '0' && c <= '9' {
			digitStart = i
			break
		}
	}

	// Edge case: all-letter inputs are returned unchanged (e.g., "ABC" -> "ABC")
	if digitStart == -1 {
		return number
	}

	prefix := number[:digitStart]
	numPart := number[digitStart:]

	// Remove leading zeros from numeric part
	numPart = strings.TrimLeft(numPart, "0")
	if numPart == "" {
		numPart = "0" // Handle "000" case
	}

	return prefix + numPart
}

// BuildCardMatchQuery normalizes set name and card name, then builds a natural
// language query for the CardHedger card-match endpoint.
func BuildCardMatchQuery(setName, cardName, cardNumber string) string {
	normalizedSet := NormalizeSetNameForSearch(setName)
	simplifiedName := SimplifyForSearch(NormalizePurchaseName(cardName))
	parts := make([]string, 0, 3)
	for _, s := range []string{normalizedSet, simplifiedName, cardNumber} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}
