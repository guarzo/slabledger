package pricecharting

import (
	"context"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/constants"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/pricing/analysis"
)

// SingleSourceBaselineConfidence is the baseline confidence score assigned to prices
// from a single source (PriceCharting). Multi-source fusion produces higher confidence.
const SingleSourceBaselineConfidence = 0.90

// LookupCard implements pricing.PriceProvider interface
// Converts internal PCMatch to domain Price
func (p *PriceCharting) LookupCard(ctx context.Context, setName string, card domainCards.Card) (*pricing.Price, error) {
	// Call internal lookup
	pcMatch, err := p.lookupCardInternal(ctx, setName, card)
	if err != nil {
		return nil, err
	}

	// Convert to domain model
	return toLookupPrice(pcMatch), nil
}

// GetStats implements pricing.PriceProvider interface
// Converts internal PriceChartingStats to domain ProviderStats
func (p *PriceCharting) GetStats(ctx context.Context) *pricing.ProviderStats {
	// Get internal stats (ctx reserved for future use, e.g., database stats)
	_ = ctx
	pcStats := p.getStatsInternal()
	if pcStats == nil {
		return nil
	}

	// Convert to domain model
	return toDomainProviderStats(pcStats)
}

// toLookupPrice converts internal PCMatch to domain Price
func toLookupPrice(pcMatch *PCMatch) *pricing.Price {
	if pcMatch == nil {
		return nil
	}

	return &pricing.Price{
		// Identification
		ID:          pcMatch.ID,
		ProductName: pcMatch.ProductName,

		// Primary price (PSA 10 for backward compatibility)
		Amount:   int64(pcMatch.PSA10Cents),
		Currency: "USD",
		Source:   pricing.SourcePriceCharting,

		// Graded prices (in cents, converted to int64)
		Grades: pricing.GradedPrices{
			PSA10Cents:   int64(pcMatch.PSA10Cents),
			PSA9Cents:    int64(pcMatch.Grade9Cents),
			PSA8Cents:    int64(pcMatch.PSA8Cents),
			Grade95Cents: int64(pcMatch.Grade95Cents),
			RawCents:     int64(pcMatch.LooseCents), // Map LooseCents to RawCents
			BGS10Cents:   int64(pcMatch.BGS10Cents),
		},

		// UPC
		UPC: pcMatch.UPC,

		// Marketplace data
		Market: &pricing.MarketData{
			ActiveListings:  pcMatch.ActiveListings,
			LowestListing:   int64(pcMatch.LowestListing),
			ListingVelocity: pcMatch.ListingVelocity,
			UnitsSold:       pcMatch.SalesCount,
			SalesLast30d:    pcMatch.Sales30d,
			SalesLast90d:    pcMatch.Sales90d,
			Volatility:      pcMatch.PriceVolatility,
		},

		// Conservative exit prices (p25 percentiles) for margin of safety
		Conservative: &pricing.ConservativePrices{
			PSA10USD: pcMatch.ConservativePSA10USD,
			PSA9USD:  pcMatch.ConservativePSA9USD,
			RawUSD:   pcMatch.OptimisticRawUSD,
		},

		// Full sales distributions (Conservative Exit Prices)
		Distributions: &pricing.Distributions{
			PSA10: pcMatch.PSA10Distribution,
			PSA9:  pcMatch.PSA9Distribution,
			Raw:   pcMatch.RawDistribution,
		},

		// Last sold data by grade
		LastSoldByGrade: pcMatch.LastSoldByGrade,

		// Confidence baseline for single-source
		Confidence: SingleSourceBaselineConfidence,
	}
}

// toDomainProviderStats converts internal PriceChartingStats to domain ProviderStats
func toDomainProviderStats(pcStats *PriceChartingStats) *pricing.ProviderStats {
	if pcStats == nil {
		return nil
	}

	stats := &pricing.ProviderStats{
		APIRequests:    pcStats.APIRequests,
		CachedRequests: pcStats.CachedRequests,
		TotalRequests:  pcStats.TotalRequests,
		CacheHitRate:   pcStats.CacheHitRate,
		Reduction:      pcStats.Reduction,
	}

	// Convert circuit breaker stats if available
	if pcStats.CircuitBreaker != nil {
		stats.CircuitBreaker = &pricing.CircuitBreakerStats{
			State:                pcStats.CircuitBreaker.State,
			Requests:             pcStats.CircuitBreaker.Requests,
			Successes:            pcStats.CircuitBreaker.Successes,
			Failures:             pcStats.CircuitBreaker.Failures,
			ConsecutiveSuccesses: pcStats.CircuitBreaker.ConsecutiveSuccesses,
			ConsecutiveFailures:  pcStats.CircuitBreaker.ConsecutiveFailures,
		}
	}

	return stats
}

// lookupCardInternal is the internal implementation that returns concrete PCMatch
// This is used by internal code that needs the full PCMatch structure
func (p *PriceCharting) lookupCardInternal(ctx context.Context, setName string, c domainCards.Card) (*PCMatch, error) {
	// Inject normalization trace for debugging the query construction chain.
	ctx = cardutil.ContextWithTrace(ctx)
	defer p.logTrace(ctx, c.Name, setName)

	// Strategy 0: Check for user-provided price hint (highest priority)
	if p.hintResolver != nil {
		hint, err := p.hintResolver.GetHint(ctx, c.Name, setName, c.Number, "pricecharting")
		if err == nil && hint != "" {
			match, err := p.LookupByProductID(ctx, hint)
			if err == nil && match != nil {
				p.enrichMatch(ctx, match)
				p.cacheManager.CacheMatch(ctx, setName, c, match)
				return match, nil
			}
			if p.logger != nil {
				p.logger.Warn(ctx, "price hint lookup failed, falling back",
					observability.String("card", c.Name),
					observability.String("hint_id", hint),
					observability.Err(err))
			}
		}
	}

	// Strategy 1: Try cache first (fastest, no API calls)
	if match, found := p.tryCache(ctx, c, setName); found {
		return match, nil
	}

	var lastErr error

	// consoleMismatchFallback captures a match that passed card-number validation
	// but failed set validation. Used as a last resort if fuzzy also finds nothing.
	var consoleMismatchFallback *PCMatch

	// Strategy 2: Try UPC lookup (highest confidence when available)
	if match, found, err := p.tryUPC(ctx, c, setName); err != nil {
		lastErr = err
		if p.logger != nil {
			p.logger.Warn(ctx, "UPC lookup failed",
				observability.Err(err), observability.String("card", c.Name))
		}
	} else if found {
		return match, nil
	}

	// Strategy 3: Try API direct lookup
	if match, found, apiFallback, err := p.tryAPI(ctx, c, setName); err != nil {
		lastErr = err
		if p.logger != nil {
			p.logger.Warn(ctx, "API lookup failed",
				observability.Err(err), observability.String("card", c.Name))
		}
	} else if found {
		return match, nil
	} else if apiFallback != nil {
		consoleMismatchFallback = apiFallback
	}

	// Strategy 4: Try fuzzy matching (fallback with alternative queries)
	if match, found, err := p.tryFuzzy(ctx, c, setName); err != nil {
		lastErr = err
		if p.logger != nil {
			p.logger.Warn(ctx, "fuzzy lookup failed",
				observability.Err(err), observability.String("card", c.Name))
		}
	} else if found {
		return match, nil
	}

	// Last resort: use the console mismatch fallback if available.
	// This is a match with the right card number but wrong set — better than no price.
	// Do NOT cache under the requested setName: the fallback's set doesn't match,
	// and caching it would pollute future lookups for the correct set.
	if consoleMismatchFallback != nil {
		if p.logger != nil {
			p.logger.Info(ctx, "using console mismatch fallback",
				observability.String("card", c.Name),
				observability.String("set", setName),
				observability.String("product", consoleMismatchFallback.ProductName))
		}
		p.enrichMatch(ctx, consoleMismatchFallback)
		return consoleMismatchFallback, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, apperrors.ProviderNotFound("PriceCharting", c.Name)
}

// logTrace emits the normalization trace at Debug level if one exists on the context.
func (p *PriceCharting) logTrace(ctx context.Context, cardName, setName string) {
	cardutil.LogNormalizationTrace(ctx, p.logger, cardName, setName)
}

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
		// Cache the result
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

	// Perform API lookup with retry logic
	apiMatch, lookupErr := p.lookupByQueryWithRetry(ctx, q)
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

// saleRecordsFromRecentSales converts adapter SaleData to domain SaleRecord.
func saleRecordsFromRecentSales(sales []SaleData) []pricing.SaleRecord {
	records := make([]pricing.SaleRecord, len(sales))
	for i, sale := range sales {
		records[i] = pricing.SaleRecord{
			PriceCents: sale.PriceCents,
			Date:       sale.Date,
			Grade:      sale.Grade,
		}
	}
	return records
}

// applyConservativeExits uses domain pricing statistics to compute conservative exit prices.
func (p *PriceCharting) applyConservativeExits(ctx context.Context, match *PCMatch) {
	if match == nil || len(match.RecentSales) == 0 {
		return
	}

	records := saleRecordsFromRecentSales(match.RecentSales)

	exits := analysis.CalculateConservativeExits(records, analysis.MinSalesThreshold, "pricecharting")
	if exits == nil {
		return
	}

	match.ConservativePSA10USD = exits.ConservativePSA10USD
	match.ConservativePSA9USD = exits.ConservativePSA9USD
	match.OptimisticRawUSD = exits.OptimisticRawUSD
	match.PSA10Distribution = exits.PSA10Distribution
	match.PSA9Distribution = exits.PSA9Distribution
	match.RawDistribution = exits.RawDistribution

	if p.logger != nil && exits.PSA10Distribution != nil {
		p.logger.Debug(ctx, "calculated conservative exit prices",
			observability.String("product", match.ProductName),
			observability.Float64("psa10_p25_usd", exits.ConservativePSA10USD),
		)
	}
}

// applyLastSoldByGrade uses domain pricing statistics to extract last sold data per grade.
func (p *PriceCharting) applyLastSoldByGrade(_ context.Context, match *PCMatch) {
	if match == nil || len(match.RecentSales) == 0 {
		return
	}

	match.LastSoldByGrade = analysis.CalculateLastSoldByGrade(saleRecordsFromRecentSales(match.RecentSales))
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

// enrichMatch applies conservative exit prices and last-sold-by-grade
// to a matched result. Shared by tryAPI and tryFuzzy.
func (p *PriceCharting) enrichMatch(ctx context.Context, match *PCMatch) {
	p.applyConservativeExits(ctx, match)
	p.applyLastSoldByGrade(ctx, match)
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

		match, err := p.lookupByQueryWithRetry(ctx, altQuery)
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

			// Reject fuzzy matches where the set name doesn't overlap
			if setName != "" && match.ConsoleName != "" && !VerifySetOverlap(ctx, match.ConsoleName, setName) {
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

// getStatsInternal is the internal implementation that returns concrete PriceChartingStats
func (p *PriceCharting) getStatsInternal() *PriceChartingStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	totalRequests := p.requestCount + p.cachedRequests
	cacheHitRate := float64(0)
	if totalRequests > 0 {
		cacheHitRate = float64(p.cachedRequests) / float64(totalRequests) * 100
	}

	// Calculate reduction percentage with zero-division guard
	reduction := "0.00%"
	if totalRequests > 0 {
		reduction = fmt.Sprintf("%.2f%%", float64(p.cachedRequests)/float64(totalRequests)*100)
	}

	stats := &PriceChartingStats{
		APIRequests:    p.requestCount,
		CachedRequests: p.cachedRequests,
		TotalRequests:  totalRequests,
		CacheHitRate:   fmt.Sprintf("%.2f%%", cacheHitRate),
		Reduction:      reduction,
	}

	// Add circuit breaker stats from httpClient
	if p.httpClient != nil {
		cbStats := p.httpClient.GetCircuitBreakerStats()
		stats.CircuitBreaker = &CircuitBreakerData{
			State:                cbStats.State,
			Requests:             cbStats.Requests,
			Successes:            cbStats.Successes,
			Failures:             cbStats.Failures,
			ConsecutiveSuccesses: cbStats.ConsecutiveSuccesses,
			ConsecutiveFailures:  cbStats.ConsecutiveFailures,
		}
	}

	return stats
}
