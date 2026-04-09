package campaigns

import (
	"regexp"
	"strings"
)

// MMSearchTitleFields holds structured fields parsed from a Market Movers SearchTitle.
type MMSearchTitleFields struct {
	PlayerName string // Card name (e.g. "Cynthia's Garchomp ex")
	Year       string // 4-digit year (e.g. "2025")
	Set        string // Set name (e.g. "Scarlet & Violet: Destined Rivals")
	Variation  string // Rarity/variant (e.g. "Special Illustration Rare")
	CardNumber string // Card number with "#" prefix (e.g. "#232/182")
}

// graderGradeSuffixRe matches trailing grader + grade at the end of a SearchTitle.
// Examples: "PSA 10", "BGS 9.5", "CGC 8", "SGC 10", "Raw TCG (Near Mint)".
var graderGradeSuffixRe = regexp.MustCompile(
	`\s+(?:(?:PSA|BGS|CGC|SGC)\s+\d+(?:\.\d)?|Raw\s+TCG\s*\([^)]*\))\s*$`,
)

// yearTokenRe matches a 4-digit year token (1990-2029).
var yearTokenRe = regexp.MustCompile(`\b((?:199|20[012])\d)\b`)

// cardNumberRe matches a card number token starting with "#".
// Matches formats like "#232/182", "#4/102", "#144/S-P", "#150/147".
var cardNumberRe = regexp.MustCompile(`\s+(#\S+)`)

// parseMMSearchTitle extracts structured fields from a Market Movers canonical SearchTitle.
//
// Format: "{PlayerName} {Year} {Set} {Variation} #{CardNumber} {Grader} {Grade}"
// Example: "Cynthia's Garchomp ex 2025 Scarlet & Violet: Destined Rivals Special Illustration Rare #232/182 PSA 10"
//
// Returns zero-value fields for any component that cannot be reliably extracted.
func parseMMSearchTitle(title string) MMSearchTitleFields {
	var fields MMSearchTitleFields
	if title == "" {
		return fields
	}

	remaining := strings.TrimSpace(title)

	// 1. Strip grader+grade from the end.
	remaining = graderGradeSuffixRe.ReplaceAllString(remaining, "")
	remaining = strings.TrimSpace(remaining)

	// 2. Extract card number (#...) — find the last occurrence.
	if loc := cardNumberRe.FindStringIndex(remaining); loc != nil {
		fields.CardNumber = strings.TrimSpace(remaining[loc[0]:loc[1]])
		remaining = strings.TrimSpace(remaining[:loc[0]])
	}

	// 3. Find year — first 4-digit year token.
	if loc := yearTokenRe.FindStringIndex(remaining); loc != nil {
		fields.Year = remaining[loc[0]:loc[1]]
		fields.PlayerName = strings.TrimSpace(remaining[:loc[0]])
		middle := strings.TrimSpace(remaining[loc[1]:])

		// 4. Split middle into Set and Variation.
		fields.Set, fields.Variation = splitSetVariation(middle)
	} else {
		// No year found — put everything in PlayerName.
		fields.PlayerName = strings.TrimSpace(remaining)
	}

	return fields
}

// mmKnownVariations are variation/rarity keywords recognized in MM SearchTitles.
// Ordered longest-first so "1st Edition Holo" matches before "Holo".
var mmKnownVariations = []string{
	"Special Illustration Rare",
	"Illustration Rare",
	"Special Art Rare",
	"Ultra Rare",
	"Hyper Rare",
	"Super Rare",
	"Secret Rare",
	"Art Rare",
	"Full Art",
	"1st Edition Holo",
	"Reverse Holo",
	"Holo",
	"1st Edition",
}

// splitSetVariation splits the middle portion (between year and card number)
// into set name and variation by matching known variation keywords at the end.
//
// Example: "Scarlet & Violet: Destined Rivals Special Illustration Rare"
//
//	→ set="Scarlet & Violet: Destined Rivals", variation="Special Illustration Rare"
//
// If no known variation is found, the entire middle is treated as the set name.
func splitSetVariation(middle string) (set, variation string) {
	if middle == "" {
		return "", ""
	}

	middleLower := strings.ToLower(middle)

	// Try each known variation (longest-first) as a suffix.
	for _, v := range mmKnownVariations {
		vLower := strings.ToLower(v)
		if strings.HasSuffix(middleLower, vLower) {
			setEnd := len(middle) - len(v)
			set = strings.TrimSpace(middle[:setEnd])
			variation = strings.TrimSpace(middle[setEnd:])
			return set, variation
		}
	}

	// No known variation found — everything is the set name.
	// But check for non-standard variation patterns that aren't in the list
	// (e.g. "Pokemon Center Kanazawa Opening" is a variation, not a set).
	return middle, ""
}
