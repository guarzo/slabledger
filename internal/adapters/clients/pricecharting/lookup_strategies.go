package pricecharting

import (
	"context"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/constants"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// tryCache attempts to find the card in cache
func (p *PriceCharting) tryCache(ctx context.Context, card domainCards.Card, setName string) (*PCMatch, bool) {
	// Check primary cache
	if match, found := p.cacheManager.GetCachedMatch(ctx, setName, card); found {
		p.incrementCachedRequests()
		return match, true
	}

	// Check query-based cache
	q := p.queryHelper.OptimizeQuery(setName, card.Name, card.Number)
	if match, found := p.cacheManager.GetCachedMatchByQuery(q); found {
		p.incrementCachedRequests()
		return match, true
	}

	return nil, false
}

// tryUPC attempts to lookup card prices using UPC codes
func (p *PriceCharting) tryUPC(ctx context.Context, card domainCards.Card, setName string) (*PCMatch, bool, error) {
	// Skip if UPC database not available
	if p.upcDatabase == nil {
		return nil, false, nil
	}

	// Check if card has UPC information
	upcMappings := p.upcDatabase.FindByCardInfo(setName, card.Number)
	if len(upcMappings) == 0 {
		return nil, false, nil
	}

	// Try UPC-based lookup
	match, err := p.LookupByUPC(ctx, upcMappings[0].UPC)
	if err != nil {
		return nil, false, err
	}

	if match != nil {
		// Enrich with historical data before caching so the cached
		// object has Conservative, Distributions, LastSoldByGrade.
		p.enrichMatch(ctx, match)
		p.cacheManager.CacheMatch(ctx, setName, card, match)
		return match, true, nil
	}

	return nil, false, nil
}

// tryAPI performs direct API lookups using optimized queries.
// Returns (match, true, nil, nil) on success, or (nil, false, fallback, nil)
// when a match passed card-number validation but failed set validation.
// The caller uses the fallback as a last resort if fuzzy also finds nothing.
func (p *PriceCharting) tryAPI(ctx context.Context, card domainCards.Card, setName string) (match *PCMatch, found bool, fallback *PCMatch, err error) {
	// Build optimized query with advanced options
	options := QueryOptions{}

	// Auto-detect language from set name (e.g. "JAPANESE SV-P PROMO" → "japanese").
	// Must happen before normalization strips the prefix.
	if lang := detectLanguageFromSet(setName); lang != "" {
		options.Language = lang
	}

	// Check for variant information in card name
	if strings.Contains(strings.ToLower(card.Name), "1st edition") {
		options.Variant = "1st Edition"
	} else if strings.Contains(strings.ToLower(card.Name), "shadowless") {
		options.Variant = "Shadowless"
	}

	q := p.BuildAdvancedQuery(setName, card.Name, card.Number, options)
	trace := cardutil.TraceFromContext(ctx)
	trace.AddStep("tryAPI:query", fmt.Sprintf("set=%q name=%q num=%q", setName, card.Name, card.Number), q)

	// Rate limiting
	if p.rateLimiter != nil {
		if err := p.rateLimiter.WaitContext(ctx); err != nil {
			return nil, false, nil, fmt.Errorf("rate limit wait cancelled: %w", err)
		}
	}

	apiMatch, lookupErr := p.lookupByQueryInternal(ctx, q)
	p.incrementRequestCount()

	if lookupErr != nil {
		return nil, false, nil, lookupErr
	}

	if apiMatch == nil {
		return nil, false, nil, nil
	}

	// Verify card number to avoid wrong-variant matches (e.g. Pikachu #002 matching Pikachu #5).
	// resolveExpectedNumber handles Chinese set remapping and SWSH era-prefix composition.
	expectedNumber, unknownVol := resolveExpectedNumber(setName, card.Number)
	if unknownVol && p.logger != nil {
		p.logger.Warn(ctx, "unknown Chinese volume, falling back to number-less search",
			observability.String("set", setName),
			observability.String("card", card.Name))
	}
	if expectedNumber != "" && !VerifyProductMatch(ctx, apiMatch.ProductName, expectedNumber) {
		if p.logger != nil {
			p.logger.Info(ctx, "API match rejected: card number mismatch",
				observability.String("card", card.Name),
				observability.String("expected_number", expectedNumber),
				observability.String("product", apiMatch.ProductName),
				observability.String("query", q))
		}
		return nil, false, nil, nil
	}

	// When set name is generic, verify the matched product's console name
	// overlaps with a set hint extracted from the card name to avoid cross-set
	// mismatches. Only check when we can extract a meaningful set hint;
	// comparing the full card name against console name produces false rejections.
	if constants.IsGenericSetName(setName) && apiMatch.ConsoleName != "" {
		if setHint := extractSetHint(card.Name); setHint != "" {
			if !VerifySetOverlap(ctx, apiMatch.ConsoleName, setHint) {
				if p.logger != nil {
					p.logger.Info(ctx, "API match rejected: generic set with no set-hint overlap",
						observability.String("card", card.Name),
						observability.String("setHint", setHint),
						observability.String("set", setName),
						observability.String("console", apiMatch.ConsoleName),
						observability.String("product", apiMatch.ProductName))
				}
				return nil, false, nil, nil
			}
		}
	}

	// For non-generic sets, verify the matched product's console name
	// doesn't have significant missing tokens from the expected set.
	// If it does, return as fallback and continue to fuzzy stage.
	if !constants.IsGenericSetName(setName) && apiMatch.ConsoleName != "" {
		// Normalize expected set to strip PSA codes before comparing.
		normalizedExpected := cardutil.NormalizeSetNameForSearch(setName)

		// If both sides are promo sets, skip token comparison.
		// PriceCharting consolidates all promo eras (SWSH, SM, SV, XY) under
		// a single "Pokemon Promo" console, so era-specific tokens always miss.
		bothPromo := containsWordCI(normalizedExpected, "promo") &&
			containsWordCI(apiMatch.ConsoleName, "promo")

		missing := cardutil.MissingSetTokens(apiMatch.ConsoleName, normalizedExpected)
		if len(missing) > 0 && bothPromo {
			missing = nil // promo match is sufficient with card number verification
		}
		if len(missing) > 0 {
			if p.logger != nil {
				p.logger.Info(ctx, "API match: set token mismatch, saving as fallback",
					observability.String("card", card.Name),
					observability.String("set", setName),
					observability.String("console", apiMatch.ConsoleName),
					observability.String("product", apiMatch.ProductName),
					observability.String("missing_tokens", strings.Join(missing, ",")))
			}
			// Only record as a fallback when card-number validation ran;
			// without a number check the match has no identity anchor.
			if expectedNumber != "" {
				apiMatch.QueryUsed = q
				return nil, false, apiMatch, nil
			}
			return nil, false, nil, nil
		}
	}

	// Set query used for tracking
	apiMatch.QueryUsed = q

	// Enrich with historical data + conservative exits + last sold
	p.enrichMatch(ctx, apiMatch)

	// Cache the match
	p.cacheManager.CacheMatch(ctx, setName, card, apiMatch)
	p.cacheManager.CacheMatchByQuery(q, apiMatch)

	return apiMatch, true, nil, nil
}

// extractSetHint extracts set-related tokens from a card name that can be used
// for set overlap verification. Returns empty string when no reliable set hint
// is found (which signals that the overlap check should be skipped).
func extractSetHint(cardName string) string {
	// Check for parenthetical set hints like "(Base Set)"
	if start := strings.Index(cardName, "("); start >= 0 {
		if end := strings.Index(cardName[start:], ")"); end > 0 {
			return strings.TrimSpace(cardName[start+1 : start+end])
		}
	}

	// Check for known set tokens in the card name
	words := strings.Fields(strings.ToLower(cardName))
	var hints []string
	for _, w := range words {
		if knownSetTokens[w] {
			hints = append(hints, w)
		}
	}
	// Only trust multi-token hints to avoid false positives from common words
	// like "star", "fire", "promo" that appear in card names
	if len(hints) >= 2 {
		return strings.Join(hints, " ")
	}
	return ""
}

// resolveExpectedNumber maps a card number through Chinese set normalization
// and era-prefix composition for PriceCharting product match verification.
// Returns the expected number to verify against, or "" to skip verification.
// unknownVol is true when the Chinese volume is unrecognized, signalling that
// the caller should log a warning and fall back to number-less search.
func resolveExpectedNumber(setName, cardNumber string) (expectedNumber string, unknownVol bool) {
	expectedNumber = cardNumber
	if cardutil.IsChineseSet(setName) {
		mapped, unknown := cardutil.MapChineseNumber(setName, cardNumber)
		expectedNumber = mapped
		if unknown {
			return "", true
		}
	}
	expectedNumber = composeEraPrefixedNumber(extractBlackStarEraPrefix(setName), expectedNumber)
	return expectedNumber, false
}

// tryFuzzy attempts fuzzy matching when exact lookups fail
func (p *PriceCharting) tryFuzzy(ctx context.Context, card domainCards.Card, setName string) (*PCMatch, bool, error) {
	// Build alternative queries
	alternatives := p.queryHelper.BuildAlternativeQueries(setName, card.Name, card.Number)

	var lastErr error
	for _, altQuery := range alternatives {
		// Rate limiting
		if p.rateLimiter != nil {
			if err := p.rateLimiter.WaitContext(ctx); err != nil {
				lastErr = err
				break
			}
		}

		match, err := p.lookupByQueryInternal(ctx, altQuery)
		p.incrementRequestCount()

		if err != nil {
			lastErr = err
			if p.logger != nil {
				p.logger.Debug(ctx, "fuzzy alternative skipped",
					observability.String("query", altQuery),
					observability.String("card", card.Name),
					observability.Err(err))
			}
			continue
		}

		if match != nil {
			// Verify card number — resolveExpectedNumber handles Chinese remapping + era prefix.
			fuzzyExpNum, unknownVol := resolveExpectedNumber(setName, card.Number)
			if unknownVol && p.logger != nil {
				p.logger.Warn(ctx, "unknown Chinese volume in fuzzy path, falling back to number-less search",
					observability.String("set", setName),
					observability.String("card", card.Name))
			}
			if fuzzyExpNum != "" && !VerifyProductMatch(ctx, match.ProductName, fuzzyExpNum) {
				if p.logger != nil {
					p.logger.Info(ctx, "fuzzy match rejected: card number mismatch",
						observability.String("card", card.Name),
						observability.String("expected_number", fuzzyExpNum),
						observability.String("product", match.ProductName),
						observability.String("query", altQuery))
				}
				continue
			}

			// Reject fuzzy matches where the set name doesn't overlap.
			// Normalize expected set to strip PSA codes before comparing
			// (consistent with tryAPI's normalization).
			normalizedSet := cardutil.NormalizeSetNameForSearch(setName)
			if setName != "" && match.ConsoleName != "" && !VerifySetOverlap(ctx, match.ConsoleName, normalizedSet) {
				if p.logger != nil {
					p.logger.Warn(ctx, "fuzzy match rejected: set mismatch",
						observability.String("card", card.Name),
						observability.String("expected_set", setName),
						observability.String("console", match.ConsoleName),
						observability.String("product", match.ProductName))
				}
				continue
			}

			if p.logger != nil {
				p.logger.Info(ctx, "fuzzy match accepted",
					observability.String("original_card", card.Name),
					observability.String("query", altQuery),
					observability.String("product", match.ProductName))
			}

			p.enrichMatch(ctx, match)
			match.QueryUsed = altQuery

			// Cache the match
			p.cacheManager.CacheMatch(ctx, setName, card, match)
			p.cacheManager.CacheMatchByQuery(altQuery, match)

			return match, true, nil
		}
	}

	return nil, false, lastErr
}
