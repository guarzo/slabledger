package pricecharting

import (
	"context"
	"fmt"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
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
		hint, err := p.hintResolver.GetHint(ctx, c.Name, setName, c.Number, pricing.SourcePriceCharting)
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
