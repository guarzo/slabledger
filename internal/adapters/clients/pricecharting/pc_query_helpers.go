package pricecharting

import (
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
)

// Query building functions for the 6-stage PriceCharting lookup pipeline:
//
//	Stage 1 (tryCache):  OptimizeQuery() — generates normalized cache key
//	Stage 3 (tryAPI):    buildQuery() → buildQueryWithOptions() → BuildAdvancedQuery()
//	Stage 4 (tryFuzzy):  BuildAlternativeQueries() — generates fallback query variants
//	HTTP pre-flight:     OptimizeQueryForDirectLookup() — ensures "pokemon" prefix

// QueryOptions represents advanced search options
type QueryOptions struct {
	Variant       string
	Region        string
	Language      string
	Condition     string
	Grader        string
	ExactMatch    bool
	IncludePromos bool
}

// BuildAdvancedQuery creates an optimized search query with options
func (p *PriceCharting) BuildAdvancedQuery(setName, cardName, number string, options QueryOptions) string {
	return buildQueryWithOptions(setName, cardName, number, options)
}

// buildQuery creates a PriceCharting search query.
// See cardutil package doc for an overview of all three normalization pipelines.
func buildQuery(setName, cardName, number string) string {
	// Extract era prefix before normalization for Black Star Promo number prefixing.
	// PSA uses "SWSH BLACK STAR PROMO" with number "075" but PriceCharting indexes
	// as "SWSH075". Similarly "SM BLACK STAR PROMO" with "SM162" already has prefix.
	eraPrefix := extractBlackStarEraPrefix(setName)

	// Normalize inputs
	setName = normalizeSetName(setName)
	cardName = normalizeCardName(cardName, setName)

	// Build base query
	query := fmt.Sprintf("pokemon %s %s", setName, cardName)

	// Add number if provided (normalize for better API matching).
	// For Chinese sets, map PSA printed numbers to PriceCharting's numbering.
	// Unknown volumes return "" which omits the number from the query.
	if number != "" {
		if cardutil.IsChineseSet(setName) {
			// unknownVol is safe to discard here: when MapChineseNumber returns ""
			// the number is omitted from the query, producing a number-less search.
			// The domain_adapter's resolveExpectedNumber handles the warning path.
			number, _ = cardutil.MapChineseNumber(setName, number)
		}
		if number != "" {
			var normalized string
			if eraPrefix != "" && strings.HasPrefix(strings.ToUpper(number), strings.ToUpper(eraPrefix)) {
				// Number already has the era prefix (e.g., "SWSH075") — use as-is.
				// NormalizeCardNumber would strip the leading zeros ("SWSH075" → "SWSH75")
				// but PriceCharting indexes with them ("SWSH075").
				// Strip denominator for consistency with the else branch.
				if idx := strings.Index(number, "/"); idx > 0 {
					number = number[:idx]
				}
				normalized = number
			} else {
				normalized = cardutil.NormalizeCardNumber(number)
				// For Black Star Promo sets, prepend era prefix to purely numeric numbers.
				// PriceCharting indexes SWSH promos as "SWSH075" not "75".
				if eraPrefix != "" {
					if prefixed := composeEraPrefixedNumber(eraPrefix, number); prefixed != number {
						normalized = prefixed
					}
				}
			}
			if normalized != "" {
				query += fmt.Sprintf(" #%s", normalized)
			}
		}
	}

	return strings.TrimSpace(query)
}

// extractBlackStarEraPrefix returns the era code (e.g., "SWSH") for Black Star Promo
// sets where PriceCharting uses era-prefixed card numbers. Returns "" for non-promo sets
// or eras where the number already includes the prefix (SM promos use "SM162" directly).
//
// Only SWSH is handled because it's the only era where PSA uses bare numbers ("075")
// but PriceCharting indexes with the era prefix ("SWSH075"). Other eras either:
//   - SM: PSA already includes the prefix in the number ("SM162")
//   - XY/BW: PriceCharting doesn't use era-prefixed numbering for these promos
func extractBlackStarEraPrefix(setName string) string {
	upper := strings.ToUpper(setName)
	if !strings.Contains(upper, "BLACK STAR PROMO") {
		return ""
	}
	// SWSH promos: PSA uses "075" but PC uses "SWSH075".
	// Use Contains rather than HasPrefix to handle year-prefixed set names
	// like "2020 SWSH BLACK STAR PROMO".
	if strings.Contains(upper, "SWSH") {
		return "SWSH"
	}
	return ""
}

// composeEraPrefixedNumber prepends eraPrefix to a card number, stripping any
// denominator (e.g. "075/165" → "075") while preserving leading zeros.
// Returns the number unchanged when eraPrefix is empty or the numeric core
// is not purely numeric (e.g. already era-prefixed like "SM162").
func composeEraPrefixedNumber(eraPrefix, number string) string {
	if eraPrefix == "" || number == "" {
		return number
	}
	// Strip denominator for both validation and composition.
	core := number
	if idx := strings.Index(core, "/"); idx > 0 {
		core = core[:idx]
	}
	// Validate: the normalized core must be purely numeric.
	normalized := cardutil.NormalizeCardNumber(core)
	if normalized == "" || !isAllDigits(normalized) {
		return number
	}
	// Prepend era prefix to the core (preserving leading zeros).
	return eraPrefix + core
}

// isAllDigits returns true if s is non-empty and contains only ASCII digits.
// Distinct from cardutil.NumericOnly which extracts the first digit run from a mixed string.
func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// isEraBlackStarPromo returns true if the lowercased set name is an era-specific
// Black Star Promo (e.g., "swsh black star promo", "sm black star promo").
// These should be simplified to just "Promo" for PriceCharting queries.
func isEraBlackStarPromo(lowerSetName string) bool {
	if !strings.Contains(lowerSetName, "black star promo") {
		return false
	}
	// Match era prefixes: abbreviated (swsh, sm) and expanded (sword shield, sun moon).
	// Expanded forms appear when input arrives pre-normalized from external sources.
	// SVP is excluded — handled separately by the SVP normalization block.
	for _, prefix := range []string{"swsh", "sm ", "sword shield", "sun moon"} {
		if strings.HasPrefix(lowerSetName, prefix) {
			return true
		}
	}
	return false
}

// ExportBuildQuery exposes buildQuery for integration testing.
func ExportBuildQuery(setName, cardName, number string) string {
	return buildQuery(setName, cardName, number)
}

// buildQueryWithOptions creates a query with advanced options
func buildQueryWithOptions(setName, cardName, number string, options QueryOptions) string {
	query := buildQuery(setName, cardName, number)

	// Add variant filter
	if options.Variant != "" {
		variant := normalizeVariant(options.Variant)
		if variant != "" {
			query += " " + variant
		}
	}

	// Add language filter
	normalizedLanguage := ""
	if options.Language != "" {
		normalizedLanguage = normalizeLanguage(options.Language)
		if normalizedLanguage != "" {
			query += " " + normalizedLanguage
		}
	}

	// Add region filter (when language normalizes away or is empty)
	if options.Region != "" && normalizedLanguage == "" {
		region := normalizeRegion(options.Region)
		if region != "" {
			query += " " + region
		}
	}

	// Add condition filter
	if options.Condition != "" {
		condition := normalizeCondition(options.Condition)
		if condition != "" {
			query += " " + condition
		}
	}

	// Add grader filter
	if options.Grader != "" {
		grader := normalizeGrader(options.Grader)
		if grader != "" {
			query += " " + grader
		}
	}

	// Add exact match operator
	if options.ExactMatch {
		query = "\"" + query + "\""
	}

	return strings.TrimSpace(query)
}

// QueryHelper provides query optimization and fuzzy matching utilities
type QueryHelper struct{}

// NewQueryHelper creates a new query helper
func NewQueryHelper() *QueryHelper {
	return &QueryHelper{}
}

// OptimizeQueryForDirectLookup creates a query optimized for direct API lookup success
func (qh *QueryHelper) OptimizeQueryForDirectLookup(query string) string {
	// Clean up common query issues that cause direct lookup failures
	optimized := strings.TrimSpace(query)

	// Remove extra spaces
	optimized = strings.Join(strings.Fields(optimized), " ")

	// Skip normalization for already-quoted exact-match queries
	if strings.HasPrefix(optimized, "\"") {
		return optimized
	}

	// Ensure Pokemon is at the start for better matching
	if !strings.HasPrefix(strings.ToLower(optimized), "pokemon") {
		optimized = "pokemon " + optimized
	}

	return optimized
}

// OptimizeQuery improves query accuracy for better matches
func (qh *QueryHelper) OptimizeQuery(setName, cardName, number string) string {
	// Handle "Reverse Holo" in card names - normalize to "Reverse"
	cleanName := cardName
	if strings.Contains(cleanName, "Reverse Holo") {
		cleanName = strings.ReplaceAll(cleanName, " Reverse Holo", " Reverse")
	}

	// Remove common suffixes that cause mismatches
	suffixes := []string{" ex", " gx", " v", " vmax", " vstar"}
	lowerName := strings.ToLower(cleanName)
	for _, suffix := range suffixes {
		if strings.HasSuffix(lowerName, suffix) {
			cleanName = cleanName[:len(cleanName)-len(suffix)]
			break
		}
	}

	// Normalize set name
	setName = strings.ReplaceAll(setName, ":", "")
	setName = strings.ReplaceAll(setName, "-", " ")

	// Build optimized query
	query := fmt.Sprintf("pokemon %s %s #%s", setName, cleanName, number)

	// Add variant indicators if present
	if strings.Contains(strings.ToLower(cardName), "reverse holo") {
		query += " reverse holo"
	} else if strings.Contains(strings.ToLower(cardName), "holo") {
		query += " holo"
	}

	return query
}

// BuildAlternativeQueries generates alternative search queries for fuzzy matching
func (qh *QueryHelper) BuildAlternativeQueries(setName, cardName, number string) []string {
	alternatives := []string{}

	// Try with hyphens preserved (normalizeCardName strips them, but names like "Mewtwo-EX" need hyphens)
	if strings.Contains(cardName, "-") {
		alternatives = append(alternatives, fmt.Sprintf("pokemon %s %s #%s", setName, cardName, number))
	}

	// Try without card type suffix
	suffixes := []string{" ex", " gx", " v", " vmax", " vstar", " EX", " GX", " V", " VMAX", " VSTAR"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(cardName, suffix) {
			cleanName := strings.TrimSuffix(cardName, suffix)
			alternatives = append(alternatives, fmt.Sprintf("pokemon %s %s #%s", setName, cleanName, number))
			break
		}
	}

	// Try with different set name formats
	setVariations := qh.generateSetVariations(setName)
	for _, setVar := range setVariations {
		alternatives = append(alternatives, fmt.Sprintf("pokemon %s %s #%s", setVar, cardName, number))
	}

	// Try without number
	alternatives = append(alternatives, fmt.Sprintf("pokemon %s %s", setName, cardName))

	return alternatives
}

// generateSetVariations creates alternative set name formats
func (qh *QueryHelper) generateSetVariations(setName string) []string {
	variations := []string{}

	// Use shared EraExpansions for era ↔ expansion swaps, plus set-specific abbreviations.
	// EraExpansions covers: SWSH↔Sword Shield, SM↔Sun Moon, SV↔Scarlet Violet, etc.
	for abbrev, expansion := range cardutil.EraExpansions {
		if strings.Contains(setName, expansion) {
			variations = append(variations, strings.Replace(setName, expansion, abbrev, 1))
		} else if strings.Contains(setName, abbrev) {
			variations = append(variations, strings.Replace(setName, abbrev, expansion, 1))
		}
	}
	// Set-specific abbreviations not covered by era expansions
	extraAbbrevMap := map[string]string{
		"Brilliant Stars": "BRS",
		"Astral Radiance": "ASR",
		"Crown Zenith":    "CRZ",
	}
	for full, abbrev := range extraAbbrevMap {
		if strings.Contains(setName, full) {
			variations = append(variations, strings.Replace(setName, full, abbrev, 1))
		} else if strings.Contains(setName, abbrev) {
			variations = append(variations, strings.Replace(setName, abbrev, full, 1))
		}
	}

	// Try with/without hyphens
	if strings.Contains(setName, "-") {
		variations = append(variations, strings.ReplaceAll(setName, "-", " "))
	} else if strings.Contains(setName, " ") {
		variations = append(variations, strings.ReplaceAll(setName, " ", "-"))
	}

	return variations
}
