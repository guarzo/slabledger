package pricecharting

import (
	"regexp"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
)

// Normalization functions for PriceCharting search queries.
//
// normalizeSetName() and normalizeCardName() are PriceCharting-specific normalizers
// (intentionally separate from cardutil) because PriceCharting uses different rules:
// & → "and", era-specific promo simplification, PSA listing title handling.

// pcAbbreviations are PriceCharting-specific abbreviation expansions derived from
// cardutil.BaseAbbreviations. Adds spaced variants ("rev. foil", "rev. holo") that
// only appear in PriceCharting's input after prior processing steps.
var pcAbbreviations = func() [][2]string {
	result := make([][2]string, 0, len(cardutil.BaseAbbreviations)+2)
	for _, abbr := range cardutil.BaseAbbreviations {
		result = append(result, [2]string{abbr.From, abbr.To})
	}
	result = append(result,
		[2]string{"rev. foil", "Reverse Foil"},
		[2]string{"rev. holo", "Reverse Holo"},
	)
	return result
}()

// detectLanguageFromSet returns a PriceCharting language filter (e.g. "japanese")
// if the set name indicates a non-English language. Returns "" for English/unknown.
func detectLanguageFromSet(setName string) string {
	lower := strings.ToLower(setName)
	if strings.HasPrefix(lower, "japanese ") || strings.Contains(lower, " japanese ") {
		return "japanese"
	}
	if strings.HasPrefix(lower, "simplified chinese ") || strings.HasPrefix(lower, "chinese ") {
		return "chinese"
	}
	return ""
}

// normalizeSetName cleans and normalizes set names
func normalizeSetName(setName string) string {
	// PriceCharting converts & to "and" (differs from cardutil which removes it)
	setName = strings.ReplaceAll(setName, "&", "and")

	// Strip "Japanese" prefix early — it's a language indicator, not a set name token.
	// Keeping it would pollute the search query (PriceCharting uses separate console
	// names for Japanese sets, not a "Japanese" keyword in the product name).
	lower := strings.ToLower(setName)
	if strings.HasPrefix(lower, "japanese ") {
		setName = strings.TrimSpace(setName[len("japanese "):])
	}

	// Normalize Card Ladder SVP promo format before shared processing.
	// Must happen early because StripCommonSetPrefixes replaces hyphens.
	// "Svp En-Sv Black Star Promo" → "SVP Black Star Promos"
	// Also handle "SV-P" with hyphen (e.g. "SV-P PROMO") which appears
	// in Japanese set names after the JAPANESE prefix is stripped.
	// ASCII-only: ToLower preserves byte offsets, safe for index transfer.
	lower = strings.ToLower(setName)
	if idx := strings.Index(lower, "sv-p"); idx >= 0 {
		// Normalize "SV-P" → "SVP" before further processing
		setName = setName[:idx] + "SVP" + setName[idx+4:]
	}
	if idx := strings.Index(strings.ToLower(setName), "svp"); idx >= 0 {
		tail := setName[idx+3:]
		tail = strings.TrimSpace(tail)
		lowerTail := strings.ToLower(tail)
		for _, lang := range []string{"en-sv ", "en sv ", "en-jp ", "en jp "} {
			if strings.HasPrefix(lowerTail, lang) {
				tail = strings.TrimSpace(tail[len(lang):])
				break
			}
		}
		if strings.HasSuffix(strings.ToLower(tail), " promo") {
			tail += "s"
		}
		setName = "SVP " + tail
	}

	// Apply shared normalization (strip :, replace -, strip year, strip Pokemon, Chinese)
	setName = cardutil.StripCommonSetPrefixes(setName)

	// Strip PSA-style set code prefixes (e.g. "SWSH45 EN ", "SV06 EN ") before
	// era expansion. Without this, "SWSH45 EN Shining Fates" would expand to
	// "Sword Shield45 EN Shining Fates" instead of just "Shining Fates".
	// Note: "JAPANESE" prefix was already stripped early in this function.
	setName = cardutil.StripPSASetCode(setName)
	lower = strings.ToLower(setName)

	// For era-specific Black Star Promo sets (SWSH, SM), simplify to just "Promo".
	// PriceCharting consolidates all promo eras under a single "Pokemon Promo"
	// console — era-specific tokens like "Sword Shield" dilute search.
	if isEraBlackStarPromo(lower) {
		return "Promo"
	}

	// Expand era abbreviations using the shared EraExpansions map.
	// SVP is guarded against false positive ("svp" should not expand as "sv").
	for abbrev, expansion := range cardutil.EraExpansions {
		prefix := strings.ToLower(abbrev)
		if strings.HasPrefix(lower, prefix) {
			// Guard: "sv" must not match "svp" (SVP is handled separately above)
			if prefix == "sv" && strings.HasPrefix(lower, "svp") {
				continue
			}
			// Guard: don't expand when prefix is followed by a digit (PSA set code like SV4a, SWSH45)
			if nextIdx := len(prefix); nextIdx < len(lower) && lower[nextIdx] >= '0' && lower[nextIdx] <= '9' {
				continue
			}
			setName = expansion + setName[len(abbrev):]
			break
		}
	}

	return strings.TrimSpace(setName)
}

// psaGradeSuffix is an alias for the shared PSA grade suffix regex.
var psaGradeSuffix = cardutil.PSAGradeSuffixRegex

// psaListingYear matches a leading 4-digit year followed by a space.
var psaListingYear = regexp.MustCompile(`^\d{4}\s+`)

// embeddedCardNumber matches embedded card numbers like #56, #199/230, #BW93, #TG16/TG30.
// Requires leading whitespace or start-of-string to avoid matching mid-word.
var embeddedCardNumber = regexp.MustCompile(`(?:^|\s+)#[A-Za-z]*\d+(?:/[A-Za-z]*\d+)?\s*`)

// normalizeCardName cleans card names for PriceCharting search queries.
// Handles PSA listing titles ("2002 Pokemon Expedition Mewtwo-Rev.foil #56 PSA 9")
// as well as cleaner names from CL imports ("Mewtwo-Rev.foil").
// normalizedSetName is the already-normalized set name used to strip duplicate
// set-name tokens from PSA listing titles embedded in the card name.
func normalizeCardName(cardName string, normalizedSetName string) string {
	// Expand Card Ladder / PSA abbreviations (case-insensitive).
	// PSA subjects use uppercase (e.g. "MEWTWO-REV.FOIL"), CL uses mixed case.
	// NOTE: PriceCharting uses dashless patterns from cardutil.BaseAbbreviations
	// plus spaced variants ("rev. foil"). It doesn't expand -holo — hyphens are
	// stripped to spaces later, making "HOLO" a standalone word automatically.
	lower := strings.ToLower(cardName)
	for _, pair := range pcAbbreviations {
		for {
			idx := strings.Index(lower, pair[0])
			if idx < 0 {
				break
			}
			cardName = cardName[:idx] + pair[1] + cardName[idx+len(pair[0]):]
			lower = strings.ToLower(cardName)
		}
	}

	// Strip PSA listing title boilerplate:
	// "2002 Pokemon Expedition Mewtwo Reverse Foil #56 PSA 9" → "Mewtwo Reverse Foil"
	cardName = psaGradeSuffix.ReplaceAllString(cardName, "")      // strip trailing "PSA 9"
	cardName = embeddedCardNumber.ReplaceAllString(cardName, " ") // strip "#56"
	cardName = psaListingYear.ReplaceAllString(cardName, "")      // strip leading "2002 "
	// Normalize separators before set-prefix stripping so tokens match normalizeSetName output
	cardName = strings.ReplaceAll(cardName, "&", "and")
	cardName = strings.ReplaceAll(cardName, ":", "")
	cardName = stripSetPrefix(cardName, normalizedSetName) // strip "Pokemon Expedition " prefix

	// Strip only hyphen-separated variant forms so expanded abbreviations are preserved
	cardName = strings.ReplaceAll(cardName, " - Reverse Holo", "")
	cardName = strings.ReplaceAll(cardName, " - Reverse Foil", "")
	// Replace hyphens with spaces (PSA uses "UMBREON-HOLO" style)
	cardName = strings.ReplaceAll(cardName, "-", " ")

	// Strip "PROMO" from card name when the set name already contains it,
	// to avoid double-weighting in the search query. Must happen after
	// hyphens are replaced so "PROMO-REVERSE" → "PROMO REVERSE" makes
	// "PROMO" a separate word that removeWordCI can match.
	if containsWordCI(normalizedSetName, "promo") || containsWordCI(normalizedSetName, "promos") {
		cardName = removeWordCI(cardName, "PROMO")
	}

	// Collapse whitespace
	cardName = strings.Join(strings.Fields(cardName), " ")
	return strings.TrimSpace(cardName)
}

// stripSetPrefix removes a "Pokemon <set-name>" prefix from card names.
// PSA listing titles embed the full set name, e.g. "Pokemon Expedition Mewtwo".
// Uses the already-normalized set name to determine which words are set tokens
// rather than maintaining a static dictionary.
func stripSetPrefix(name string, normalizedSetName string) string {
	lower := strings.ToLower(name)
	// Strip "Pokemon"/"Pokémon" prefix
	for _, prefix := range []string{"pokemon ", "pokémon "} {
		if strings.HasPrefix(lower, prefix) {
			rest := name[len(prefix):]
			return stripSetWords(rest, normalizedSetName)
		}
	}
	return name
}

// knownSetTokens are common Pokemon TCG set-name words used as fallback when
// the normalized set name doesn't match the embedded prefix (e.g. set is "TCG Cards"
// but card name contains "Scarlet Violet Prismatic Evolutions ...").
var knownSetTokens = map[string]bool{
	"expedition": true, "base": true, "jungle": true, "fossil": true,
	"rocket": true, "neo": true, "gym": true, "discovery": true,
	"revelation": true, "genesis": true, "destiny": true,
	"aquapolis": true, "skyridge": true, "legendary": true,
	"collection": true, "series": true, "promos": true, "promo": true,
	"diamond": true, "pearl": true, "platinum": true,
	"heartgold": true, "soulsilver": true,
	"evolutions": true, "celebrations": true, "classic": true,
	"hidden": true, "fates": true, "legends": true, "crown": true, "zenith": true,
	"prismatic": true, "sv": true, "swsh": true, "sm": true, "xy": true, "bw": true,
	"black": true, "white": true, "scarlet": true, "violet": true,
	"sun": true, "moon": true, "sword": true, "shield": true, "star": true,
	"shining": true, "ruby": true, "sapphire": true, "emerald": true,
	"fire": true, "red": true, "leaf": true, "green": true, "team": true,
}

// stripSetWords removes leading words from s that match tokens in the set name.
// Uses ordered prefix matching first, then falls back to at most one knownSetTokens match.
func stripSetWords(s string, normalizedSetName string) string {
	normalizedTokens := strings.Fields(normalizedSetName)
	words := strings.Fields(s)

	// Try ordered prefix match against the set name tokens
	i := stripLeadingSequence(words, normalizedTokens)

	// If no ordered match, fall back to known set tokens (at most one)
	if i == 0 && len(words) > 0 && knownSetTokens[strings.ToLower(words[0])] {
		i = 1
	}

	if i == 0 || i >= len(words) {
		return s // Nothing to strip or would strip everything
	}
	return strings.Join(words[i:], " ")
}

// stripLeadingSequence returns the count of leading words consumed by matching
// the orderedTokens in sequence. Words must match the tokens in order.
func stripLeadingSequence(words []string, orderedTokens []string) int {
	i := 0
	for i < len(words) && i < len(orderedTokens) {
		if strings.EqualFold(words[i], orderedTokens[i]) {
			i++
		} else {
			break
		}
	}
	return i
}

// normalizeVariant maps common variants to PriceCharting format
func normalizeVariant(variant string) string {
	switch strings.ToLower(variant) {
	case "1st edition", "first edition":
		return "1st edition"
	case "shadowless":
		return "shadowless"
	case "unlimited":
		return "unlimited"
	case "reverse holo", "reverse":
		return "reverse holo"
	case "holo":
		return "holo"
	case "staff", "staff promo":
		return "staff"
	case "prerelease":
		return "prerelease"
	default:
		return variant
	}
}

// normalizeLanguage maps language codes to PriceCharting format
func normalizeLanguage(language string) string {
	switch strings.ToLower(language) {
	case "japanese", "jp", "japan":
		return "japanese"
	case "korean", "kr", "korea":
		return "korean"
	case "french", "fr":
		return "french"
	case "german", "de":
		return "german"
	case "spanish", "es":
		return "spanish"
	case "italian", "it":
		return "italian"
	case "english", "en", "usa", "us":
		return "" // English is default, no filter needed
	default:
		return ""
	}
}

// normalizeRegion maps region codes to PriceCharting format
func normalizeRegion(region string) string {
	switch strings.ToLower(region) {
	case "japan", "japanese", "jp":
		return "japanese"
	case "europe", "european", "eu":
		return "european"
	case "korea", "korean", "kr":
		return "korean"
	case "usa", "us", "english", "en":
		return "" // USA is default, no filter needed
	default:
		return ""
	}
}

// normalizeCondition maps condition names to PriceCharting format
func normalizeCondition(condition string) string {
	switch strings.ToLower(condition) {
	case "mint", "m":
		return "mint"
	case "near mint", "nm":
		return "near mint"
	case "excellent", "ex":
		return "excellent"
	case "good", "gd":
		return "good"
	case "poor", "pr":
		return "poor"
	case "graded":
		return "graded"
	default:
		return ""
	}
}

// normalizeGrader maps grader names to PriceCharting format
func normalizeGrader(grader string) string {
	switch strings.ToUpper(grader) {
	case "PSA":
		return "PSA"
	case "BGS", "BECKETT":
		return "BGS"
	case "CGC":
		return "CGC"
	case "SGC":
		return "SGC"
	default:
		return ""
	}
}

// containsWordCI returns true if s contains word as a whole word (case-insensitive).
func containsWordCI(s, word string) bool {
	for _, w := range strings.Fields(s) {
		if strings.EqualFold(w, word) {
			return true
		}
	}
	return false
}

// removeWordCI removes all whole-word occurrences of word from s (case-insensitive).
func removeWordCI(s, word string) string {
	words := strings.Fields(s)
	var result []string
	for _, w := range words {
		if !strings.EqualFold(w, word) {
			result = append(result, w)
		}
	}
	return strings.Join(result, " ")
}
