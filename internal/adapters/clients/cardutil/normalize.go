// Package cardutil provides shared utilities for card name normalization
// used by multiple price provider clients.
//
// Three normalization pipelines flow through functions in this package:
//
// Pipeline 1 -- PriceCharting Query Building (pricecharting package):
//
//	normalizeSetName(set) -> StripCommonSetPrefixes -> StripPSASetCode -> era expansion
//	normalizeCardName(name, normalizedSet) -> pcAbbreviations -> strip boilerplate -> strip set prefix
//	buildQuery(set, name, num) -> "pokemon <set> <name> #<num>"
//
// Pipeline 2 -- CardHedger Query Building (fusionprice package):
//
//	BuildCardMatchQuery -> NormalizeSetNameForSearch(set) + SimplifyForSearch(NormalizePurchaseName(name)) + number
//	Fallback: truncateAtVariant(name) + eraPrefix + number
//	Fallback: raw PSA listing title (stripped of grade suffix)
//
// Pipeline 3 -- Import Title Parsing (campaigns package):
//
//	parseCardMetadataFromTitle -> ParsePSAListingTitle + extractCardNameFromPSATitle
//	-> stripCollectionSuffix -> extractVariantFromTitle -> resolvePSACategory
package cardutil

import (
	"regexp"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// psaSetCodeRegex matches PSA-style set code prefixes like "PRE EN ", "M24 EN ", "SV06 EN ",
// "SWSH4 EN ", "SWSH45 EN ". These are internal PSA category codes that don't appear
// in price API set names. Upper bound of 6 covers modern multi-char prefixes.
var psaSetCodeRegex = regexp.MustCompile(`^[A-Z0-9]{2,6}\s[A-Z]{2}\s`)

// PSAGradeSuffixRegex matches trailing PSA/BGS/CGC/SGC grade suffixes like "PSA 9", "BGS 10", "PSA 9.5".
var PSAGradeSuffixRegex = regexp.MustCompile(`\s+(?:PSA|BGS|CGC|SGC)\s+\d{1,2}(?:\.\d)?\s*$`)

// BaseAbbreviation is a canonical PSA abbreviation -> expansion pair shared
// across pipelines. NoDashPrefix marks entries that appear without a leading
// dash in PSA titles (e.g., "sp.delivery" vs "-rev.foil").
type BaseAbbreviation struct {
	From, To     string
	NoDashPrefix bool
}

// BaseAbbreviations are the canonical PSA abbreviation -> expansion pairs shared
// across pipelines. Pipeline-specific wrappers (PSAAbbreviations for cardutil,
// pcAbbreviations for PriceCharting) adapt these to their matching context:
// cardutil uses dash-prefixed patterns; PriceCharting uses dashless patterns
// with additional spaced variants.
var BaseAbbreviations = []BaseAbbreviation{
	{From: "sp.delivery", To: "Special Delivery", NoDashPrefix: true},
	{From: "rev.foil", To: "Reverse Foil"},
	{From: "rev.holo", To: "Reverse Holo"},
}

// PSAAbbreviations maps PSA listing abbreviations (dash-prefixed) to expanded
// forms. Used by NormalizePurchaseName for CardHedger/fusion pipeline.
// Derived from BaseAbbreviations with dash prefix, plus -holo which is
// cardutil-specific (PriceCharting relies on hyphen->space conversion instead).
var PSAAbbreviations = func() [][2]string {
	result := make([][2]string, 0, len(BaseAbbreviations)+1)
	for _, abbr := range BaseAbbreviations {
		if abbr.NoDashPrefix {
			result = append(result, [2]string{abbr.From, abbr.To})
		} else {
			// Dash-prefixed abbreviations get " " before the expansion in PSA context
			result = append(result, [2]string{"-" + abbr.From, " " + abbr.To})
		}
	}
	// -holo is cardutil-specific: PriceCharting strips hyphens to spaces later
	result = append(result, [2]string{"-holo", " Holo"})
	return result
}()

// VariantKeywords re-exports domain variant keywords for adapter-layer convenience.
var VariantKeywords = constants.VariantKeywords

// EraExpansions maps abbreviated Pokemon TCG era codes to their full set name
// prefixes. This is the single source of truth for era code <-> expansion pairs.
// Both cardutil (for era detection) and pricecharting (for query building and
// alternative query generation) import from this map.
var EraExpansions = map[string]string{
	"SWSH": "Sword Shield",
	"SM":   "Sun Moon",
	"SV":   "Scarlet Violet",
	"XY":   "XY",
	"BW":   "Black White",
}

// KnownEraTokens maps Pokemon TCG era prefixes used in promo set names.
// Derived from EraExpansions plus SVP (a token but not an expansion prefix).
var KnownEraTokens = func() map[string]bool {
	m := make(map[string]bool, len(EraExpansions)+1)
	for k := range EraExpansions {
		m[k] = true
	}
	m["SVP"] = true // SVP is recognized as an era token but has no expansion
	return m
}()

// Regex patterns for normalizing card names
var (
	// bracketModifierRegex matches bracket modifiers like [Reverse Holo], [Jumbo], [Cosmos Holo], etc.
	bracketModifierRegex = regexp.MustCompile(`\s*\[[^\]]+\]\s*`)
	// cardNumberRegex matches card numbers like #30, #201, #139/195, #GG42, #SV151, #126/XY-P
	// Supports optional letter prefix (for gallery/special cards) and optional denominator.
	// Denominator allows dashes and trailing letters for Japanese-style numbers (e.g., XY-P, S-P).
	cardNumberRegex = regexp.MustCompile(`\s*#[A-Z]*\d+(?:/[A-Za-z0-9-]+)?\s*$`)
	// collectorNumberExtractRegex extracts collector numbers like #199, #30, #GG42 from card names
	// Matches: #123, #GG42, #SV151, #123/195, #126/XY-P
	collectorNumberExtractRegex = regexp.MustCompile(`#([A-Z]*\d+)(?:/[A-Za-z0-9-]+)?$`)
)

// cardTypeSuffixes are TCG card type keywords that mark the end of the core card name.
var cardTypeSuffixes = map[string]bool{
	"ex": true, "EX": true, "GX": true, "V": true,
	"VMAX": true, "VSTAR": true, "BREAK": true,
}

// noiseWords are trailing tokens commonly found in PSA listing titles that
// should be stripped to produce a clean search name.
// Only universal PSA listing noise is included here. Card/set-specific words
// (SOUTHERN, ISLAND, THUNDER, KNUCKLE, etc.) are handled by user price hints
// rather than an ever-growing noise word list.
var noiseWords = map[string]bool{
	// Collection
	"COLL": true, "COLLECTION": true, "PREMIUM": true, "PREM": true,
	"BOX": true, "FIGURE": true, "CENTER": true, "PC": true,
	// Rarity
	"SPECIAL": true, "ILLUSTRATION": true, "RARE": true,
	// Edition/variant
	"1ST": true, "EDITION": true, "Ed.": true, "SHADOWLESS": true, "UNLIMITED": true,
	// Generic labels
	"PROMO": true, "PRE": true, "POKEMON": true,
}

// genericSetWords are words that don't carry set-specific meaning and should
// be skipped when extracting significant set tokens or comparing sets.
// Shared by ExtractSetKeyword and MissingSetTokens to prevent drift.
var genericSetWords = map[string]bool{
	"set": true, "cards": true, "pokemon": true, "japanese": true,
	"promo": true, "promos": true,
	"the": true, "and": true, "of": true, "vs": true,
}
