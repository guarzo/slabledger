package fusionprice

import (
	"context"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
)

// CardValidationResult captures the outcome of cross-validating a resolved card
// against the canonical card database.
type CardValidationResult struct {
	Valid         bool
	CanonicalCard *domainCards.Card
	Reason        string
}

// ValidateCardResolution checks whether a PriceCharting-resolved card identity
// (name, number, set) matches a known card in the CardProvider database.
//
// Logic:
//  1. Search for cards matching the resolved name + set + number.
//  2. If top result has an exact number match + name overlap → valid.
//  3. If a number was provided but nothing matches in the set → invalid (wrong card).
//  4. If no number available → accept a name-only match with lower confidence.
func ValidateCardResolution(ctx context.Context, cardProv domainCards.CardProvider, resolvedName, resolvedNumber, setName string) CardValidationResult {
	if cardProv == nil || !cardProv.Available() {
		return CardValidationResult{Valid: true, Reason: "card provider unavailable, skipping validation"}
	}

	// Normalize the resolved name for search — PriceCharting product names include
	// bracket modifiers and embedded numbers (e.g., "Mewtwo [Reverse Holo] #56")
	// that won't substring-match TCGdex card names (just "Mewtwo").
	cleanName := cardutil.NormalizeCardName(resolvedName)

	// Don't pass set name to search criteria — PSA-style set names
	// (e.g., "SVP EN-SV BLACK STAR PROMO") won't match TCGdex set names.
	// We validate set membership via number matching + name overlap instead.
	normalizedNumber := cardutil.NormalizeCardNumber(resolvedNumber)
	criteria := domainCards.SearchCriteria{
		CardName:   cleanName,
		CardNumber: normalizedNumber,
		Limit:      10,
	}

	results, _, err := cardProv.SearchCards(ctx, criteria)
	if err != nil {
		return CardValidationResult{Valid: true, Reason: "validation skipped: card provider error"}
	}
	if len(results) == 0 {
		// No results from card database — can't disprove the card's existence.
		// Many Japanese, promo, and older sets are not in TCGdex.
		return CardValidationResult{Valid: true, Reason: "no card database results for resolved card"}
	}

	// Post-filter by set name overlap when set name is available.
	// If no results match the set, skip validation rather than matching
	// against cards from the wrong set.
	if setName != "" {
		var setFiltered []domainCards.Card
		for _, c := range results {
			if cardutil.MatchesSetOverlap(c.SetName, setName) {
				setFiltered = append(setFiltered, c)
			}
		}
		if len(setFiltered) > 0 {
			results = setFiltered
		} else {
			// No set overlap — can't validate, assume valid to avoid false rejection
			return CardValidationResult{Valid: true, Reason: "no card database results matching set"}
		}
	}

	// Check the top results for a match
	for i := range results {
		card := &results[i]
		resultNum := cardutil.NormalizeCardNumber(card.Number)

		if normalizedNumber != "" {
			// Number-based validation
			if resultNum != normalizedNumber {
				continue
			}
			// Number matches — check for name overlap
			if nameOverlaps(resolvedName, card.Name) {
				return CardValidationResult{
					Valid:         true,
					CanonicalCard: card,
					Reason:        "number + name match in card database",
				}
			}
		} else {
			// Name-only validation (no collector number)
			if nameOverlaps(resolvedName, card.Name) {
				return CardValidationResult{
					Valid:         true,
					CanonicalCard: card,
					Reason:        "name match in card database (no number available)",
				}
			}
		}
	}

	// No exact match found, but absence of evidence is not evidence of absence.
	// TCGdex doesn't cover all sets/variants. Accept the PriceCharting result.
	return CardValidationResult{Valid: true, Reason: "no exact match in card database, accepting PriceCharting result"}
}

// ResolveCardIdentity searches TCGdex for a canonical card identity.
// Returns a canonical Card if a single unambiguous match is found:
//   - If cardNumber is provided, requires an exact number match within the set.
//   - If no cardNumber, requires exactly one name+set match (multiple → nil).
//
// Returns nil when no match is found or the result is ambiguous.
func ResolveCardIdentity(ctx context.Context, cardProv domainCards.CardProvider, cardName, cardNumber, setName string) *domainCards.Card {
	if cardProv == nil || !cardProv.Available() {
		return nil
	}

	// Normalize card name for search — raw names from PSA imports
	// (e.g., "MEWTWO-REV.FOIL") won't substring-match TCGdex names.
	cleanName := cardutil.NormalizeCardName(cardName)

	// Don't pass set name — PSA-style set names won't match TCGdex names.
	// Use set overlap as a post-filter instead.
	criteria := domainCards.SearchCriteria{
		CardName:   cleanName,
		CardNumber: cardNumber,
		Limit:      10,
	}

	results, _, err := cardProv.SearchCards(ctx, criteria)
	if err != nil || len(results) == 0 {
		return nil
	}

	// Post-filter by set name overlap when available.
	// When set name is provided and NO results overlap, return nil rather than
	// keeping unfiltered results — the set mismatch is a strong signal that
	// the card isn't in this database (e.g., Japanese cards not in TCGdex).
	if setName != "" {
		var setFiltered []domainCards.Card
		for _, c := range results {
			if cardutil.MatchesSetOverlap(c.SetName, setName) {
				setFiltered = append(setFiltered, c)
			}
		}
		if len(setFiltered) > 0 {
			results = setFiltered
		} else {
			return nil // No set overlap → card not in database for this set
		}
	}

	normalizedNum := cardutil.NormalizeCardNumber(cardNumber)

	if normalizedNum != "" {
		// Card number provided → require exact number match
		for i := range results {
			card := &results[i]
			if cardutil.NormalizeCardNumber(card.Number) == normalizedNum && nameOverlaps(cardName, card.Name) {
				return card
			}
		}
		return nil
	}

	// No card number → require exactly one name match to avoid ambiguity
	var matched []*domainCards.Card
	for i := range results {
		card := &results[i]
		if nameOverlaps(cardName, card.Name) {
			matched = append(matched, card)
		}
	}
	if len(matched) == 1 {
		return matched[0]
	}
	return nil // ambiguous or no match
}

// nameOverlaps checks whether the resolved name shares significant tokens
// with the canonical card name. Uses case-insensitive exact token matching
// (not substring) to avoid false positives like "Mew" matching "Mewtwo".
func nameOverlaps(resolved, canonical string) bool {
	resolvedTokens := normalizeNameTokens(resolved)
	canonicalTokens := normalizeNameTokens(canonical)

	// Build set of canonical tokens for exact matching
	canonicalSet := make(map[string]struct{}, len(canonicalTokens))
	for _, t := range canonicalTokens {
		canonicalSet[t] = struct{}{}
	}

	matches := 0
	for _, token := range resolvedTokens {
		if len(token) < 3 {
			continue
		}
		if _, ok := canonicalSet[token]; ok {
			matches++
		}
	}
	return matches > 0
}

// normalizeNameTokens lowercases s, strips punctuation, and splits into tokens.
func normalizeNameTokens(s string) []string {
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
