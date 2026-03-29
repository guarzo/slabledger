package fusionprice

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/cache"
)

// Compile-time interface check
var _ pricing.PriceProvider = (*FusionPriceProvider)(nil)

// DefaultFreshnessDuration is the default maximum age for database prices to be considered fresh.
// Prices older than this threshold will be re-fetched from source APIs.
const DefaultFreshnessDuration = 48 * time.Hour

// FusionPriceProvider implements pricing.PriceProvider using multi-source fusion
type FusionPriceProvider struct {
	engine                 *fusion.FusionEngine
	priceCharting          pricing.PriceProvider
	secondarySources       []fusion.SecondaryPriceSource
	cache                  cache.Cache
	priceRepo              pricing.PriceRepository
	apiTracker             pricing.APITracker
	accessTracker          pricing.AccessTracker
	logger                 observability.Logger
	cardProvider           domainCards.CardProvider // optional card database for validation
	freshnessDuration      time.Duration            // Maximum age for database prices to be considered fresh
	cacheTTL               time.Duration            // In-memory cache TTL for fused prices
	priceChartingTimeout   time.Duration            // Per-request timeout for PriceCharting
	secondarySourceTimeout time.Duration            // Per-request timeout for secondary sources
	inflight               singleflight.Group       // Deduplicates concurrent GetPrice calls for the same card
}

// NewFusionProviderWithRepo creates a new fusion provider with database support.
// The freshnessDuration parameter specifies the maximum age for database prices to be
// considered fresh. If zero or negative, DefaultFreshnessDuration (48 hours) is used.
// The secondarySources parameter accepts any implementations of SecondaryPriceSource.
// Optional cacheTTL, pcTimeout, and secondaryTimeout override defaults (4h, 30s, 20s).
func NewFusionProviderWithRepo(
	pcProvider pricing.PriceProvider,
	secondarySources []fusion.SecondaryPriceSource,
	appCache cache.Cache,
	priceRepo pricing.PriceRepository,
	apiTracker pricing.APITracker,
	accessTracker pricing.AccessTracker,
	log observability.Logger,
	freshnessDuration time.Duration,
	cacheTTL time.Duration,
	pcTimeout time.Duration,
	secondaryTimeout time.Duration,
	opts ...FusionOption,
) *FusionPriceProvider {
	// Configure fusion engine with custom weights
	fusionConfig := fusion.DefaultFusionConfig()
	fusionConfig.SourceWeights["cardhedger"] = 0.85 // Multi-platform price estimates

	// Fall back to defaults for zero values
	if freshnessDuration <= 0 {
		freshnessDuration = DefaultFreshnessDuration
	}
	if cacheTTL <= 0 {
		cacheTTL = 4 * time.Hour
	}
	if pcTimeout <= 0 {
		pcTimeout = 30 * time.Second
	}
	if secondaryTimeout <= 0 {
		secondaryTimeout = 20 * time.Second
	}

	f := &FusionPriceProvider{
		engine:                 fusion.NewFusionEngine(fusionConfig, log),
		priceCharting:          pcProvider,
		secondarySources:       secondarySources,
		cache:                  appCache,
		priceRepo:              priceRepo,
		apiTracker:             apiTracker,
		accessTracker:          accessTracker,
		logger:                 log,
		freshnessDuration:      freshnessDuration,
		cacheTTL:               cacheTTL,
		priceChartingTimeout:   pcTimeout,
		secondarySourceTimeout: secondaryTimeout,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// FusionOption configures optional FusionPriceProvider dependencies.
type FusionOption func(*FusionPriceProvider)

// WithCardProvider sets the card database provider for cross-validation.
func WithCardProvider(cp domainCards.CardProvider) FusionOption {
	return func(f *FusionPriceProvider) { f.cardProvider = cp }
}

// GetPrice implements pricing.PriceProvider interface
func (f *FusionPriceProvider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
	// Normalize PSA-style card names (e.g. "DARK GYARADOS-HOLO" → "DARK GYARADOS Holo")
	// so DB/cache lookups match data stored under the normalized name.
	card.Name = cardutil.NormalizePurchaseName(card.Name)

	// Create sync collector for unified logging
	collector := NewCardSyncCollector(f.logger, card.Name, card.Number, card.Set)
	defer collector.Complete(ctx)

	// Record card access for priority refresh (best-effort, non-critical).
	// Safe to ignore: access tracking is for analytics/optimization only;
	// failure does not affect price retrieval or core flow.
	if f.accessTracker != nil {
		//nolint:errcheck // intentionally ignored - analytics only, non-critical
		f.accessTracker.RecordCardAccess(ctx, card.Name, card.Set, "analysis")
	}

	// 1. Check database for recent prices (within freshness threshold).
	if f.priceRepo != nil {
		dbPrice, age := f.getFromDatabase(ctx, card)
		if dbPrice != nil {
			collector.RecordCacheHit(age)
			return dbPrice, nil
		}
	}

	// 2. Check in-memory cache (for very recent fetches)
	cacheKey := fmt.Sprintf("fusion:%s:%s:%s", card.Set, card.Name, card.Number)
	if cached, err := f.getCached(ctx, cacheKey); err == nil && cached != nil {
		collector.RecordCacheHit(0) // Memory cache doesn't track age
		return cached, nil
	}

	// 3. Deduplicate concurrent requests for the same card.
	// singleflight coalesces in-flight fetches so two schedulers hitting the
	// same card simultaneously share a single set of API calls.
	// The closure always returns (result, nil) — errors are inside gpr.err.
	// Include request-mode flags in the key so callers with different
	// onDemand/noStale settings get separate flights (they use different
	// source sets and fallback behavior).
	onDemand := isOnDemand(ctx)
	noStale := isNoStale(ctx)
	flightKey := fmt.Sprintf("%s|%s|%s|od=%t|ns=%t", card.Set, card.Name, card.Number, onDemand, noStale)
	v, err, shared := f.inflight.Do(flightKey, func() (any, error) {
		return f.getPriceFromSources(ctx, card, collector, cacheKey), nil
	})
	if err != nil {
		fusionErr := apperrors.ProviderInvalidResponse("fusion", fmt.Errorf("singleflight error: %w", err))
		collector.RecordError(fusionErr)
		return nil, fusionErr
	}
	gpr, ok := v.(*getPriceResult)
	if !ok || gpr == nil {
		fusionErr := apperrors.ProviderInvalidResponse("fusion", fmt.Errorf("unexpected nil from singleflight"))
		collector.RecordError(fusionErr)
		return nil, fusionErr
	}
	if gpr.err != nil {
		collector.RecordError(gpr.err)
		return nil, gpr.err
	}

	result := gpr.price

	if shared {
		// This caller's request was deduplicated — the first caller's
		// collector already has the full source-level breakdown.
		collector.RecordSingleFlightHit()
	} else {
		// First caller: record final prices for summary logging.
		// Guard against nil FusionMetadata (e.g. stale DB fallback).
		sourceCount := 0
		if result.FusionMetadata != nil {
			sourceCount = result.FusionMetadata.SourceCount
		}
		collector.RecordPrices(PriceResult{
			PSA10: mathutil.ToDollars(result.Grades.PSA10Cents),
			PSA9:  mathutil.ToDollars(result.Grades.PSA9Cents),
			PSA8:  mathutil.ToDollars(result.Grades.PSA8Cents),
			CGC95: mathutil.ToDollars(result.Grades.Grade95Cents),
			BGS10: mathutil.ToDollars(result.Grades.BGS10Cents),
			Raw:   mathutil.ToDollars(result.Grades.RawCents),
		}, result.Confidence, sourceCount)
	}

	return result, nil
}

// getPriceResult wraps the return values from getPriceFromSources for singleflight.
type getPriceResult struct {
	price *pricing.Price
	err   error
}

// getPriceFromSources fetches prices from available sources, fuses them, and persists the result.
// Extracted from GetPrice to allow singleflight deduplication of concurrent requests.
func (f *FusionPriceProvider) getPriceFromSources(ctx context.Context, card pricing.Card, collector *CardSyncCollector, cacheKey string) *getPriceResult {
	// Check if providers are blocked
	availableSources := f.getAvailableSources(ctx)
	if len(availableSources) == 0 {
		// Return stale database price if available (skip when noStale is set,
		// e.g. canonical card lookups where stale data under old name is wrong)
		if f.priceRepo != nil && !isNoStale(ctx) {
			stalePrice, staleAge := f.getStalePrice(ctx, card)
			if stalePrice != nil {
				collector.RecordCacheHit(staleAge)
				return &getPriceResult{price: stalePrice}
			}
		}
		return &getPriceResult{err: apperrors.ProviderUnavailable("fusion", fmt.Errorf("all price providers blocked or unavailable"))}
	}

	// Fetch from available sources only (collector records source results)
	pricesByGrade, fetchResults, pcPrice, sourceResults, err := f.fetchFromAvailableSources(ctx, card, availableSources, collector)
	if err != nil {
		return &getPriceResult{err: err}
	}

	// Fuse prices for each grade
	fusedPrices := make(map[string]*fusion.FusedPrice)
	for grade, prices := range pricesByGrade {
		if len(prices) == 0 {
			continue
		}

		fusedPrice, err := f.engine.FusePrices(ctx, prices)
		if err != nil {
			// Log the error with context before continuing
			if f.logger != nil {
				f.logger.Warn(ctx, "fusion failed for grade",
					observability.String("grade", grade),
					observability.String("card", card.Name),
					observability.Int("source_count", len(prices)),
					observability.Err(err))
			}
			continue
		}

		// Apply confidence penalty when sources diverge significantly (>3x)
		if len(prices) >= 2 {
			minVal, maxVal := prices[0].Value, prices[0].Value
			for _, p := range prices[1:] {
				minVal = min(minVal, p.Value)
				maxVal = max(maxVal, p.Value)
			}
			if minVal > 0 && maxVal/minVal > divergenceThreshold {
				fusedPrice.Confidence *= 0.7
			}
		}

		fusedPrices[grade] = fusedPrice
	}

	// Ensure we have at least one successful fusion
	if len(fusedPrices) == 0 {
		return &getPriceResult{err: apperrors.ProviderInvalidResponse("fusion", fmt.Errorf("fusion failed for all grades"))}
	}

	// Convert to pricing.Price format
	result := f.convertToPriceResponse(fusedPrices)

	// Add source results to fusion metadata
	if result.FusionMetadata != nil {
		result.FusionMetadata.SourceResults = sourceResults
	}

	// Add PriceCharting-specific data (last sold, conservative exits, sales velocity)
	if pcPrice != nil {
		result.LastSoldByGrade = pcPrice.LastSoldByGrade
		result.PCGrades = &pcPrice.Grades
		if pcPrice.Conservative != nil {
			result.Conservative = pcPrice.Conservative
		}
		if pcPrice.Market != nil {
			if result.Market == nil {
				result.Market = &pricing.MarketData{}
			}
			result.Market.SalesLast30d = pcPrice.Market.SalesLast30d
			result.Market.SalesLast90d = pcPrice.Market.SalesLast90d
			result.Market.ActiveListings = pcPrice.Market.ActiveListings
			result.Market.LowestListing = pcPrice.Market.LowestListing
			result.Market.ListingVelocity = pcPrice.Market.ListingVelocity
			result.Market.Volatility = pcPrice.Market.Volatility
		}
	}

	// Attach per-grade detail data and source names.
	f.attachSourceDetails(result, fetchResults, pcPrice)

	// Persist to database
	if f.priceRepo != nil {
		f.persistToDatabase(ctx, card, result)
	}

	// Cache result in memory
	f.setCached(ctx, cacheKey, result, f.cacheTTL)

	// Cache details separately with TTL matching DB freshness, so grade details
	// survive between main cache expiry (4h) and DB freshness window (48h)
	f.setCached(ctx, detailsCacheKey(card), result, f.freshnessDuration)

	return &getPriceResult{price: result}
}

// Available returns true if the provider is ready
func (f *FusionPriceProvider) Available() bool {
	return f.priceCharting != nil && f.priceCharting.Available()
}

// Name returns the provider identifier
func (f *FusionPriceProvider) Name() string {
	return "fusion"
}

// Close releases resources
func (f *FusionPriceProvider) Close() error {
	if f.priceCharting != nil {
		return f.priceCharting.Close()
	}
	return nil
}

// attachSourceDetails extracts per-grade detail data from FetchResults returned by
// secondary sources and populates result.GradeDetails, result.Velocity, and result.Sources.
// This approach eliminates shared mutable state -- all detail data flows through return values.
func (f *FusionPriceProvider) attachSourceDetails(result *pricing.Price, results []*fusion.FetchResult, pcPrice *pricing.Price) {
	var ebayDetails map[string]*pricing.EbayGradeDetail
	var estimateDetails map[string]*pricing.EstimateGradeDetail
	for _, fr := range results {
		// Merge eBay details from all sources (preserves data from multiple sources)
		if fr.EbayDetails != nil {
			if ebayDetails == nil {
				ebayDetails = make(map[string]*pricing.EbayGradeDetail)
			}
			for k, v := range fr.EbayDetails {
				ebayDetails[k] = v
			}
		}
		// First-wins for velocity (intentional: primary source takes precedence)
		if fr.Velocity != nil && result.Velocity == nil {
			result.Velocity = fr.Velocity
		}
		// Merge estimate details from all sources
		if fr.EstimateDetails != nil {
			if estimateDetails == nil {
				estimateDetails = make(map[string]*pricing.EstimateGradeDetail)
			}
			for k, v := range fr.EstimateDetails {
				estimateDetails[k] = v
			}
		}
	}

	// Build GradeDetails from the union of all grades across eBay and estimate sources
	result.GradeDetails = make(map[string]*pricing.GradeDetail)
	gradeSet := make(map[string]struct{})
	for grade := range ebayDetails {
		gradeSet[grade] = struct{}{}
	}
	for grade := range estimateDetails {
		gradeSet[grade] = struct{}{}
	}
	for grade := range gradeSet {
		detail := &pricing.GradeDetail{}
		if ebayDetails != nil {
			detail.Ebay = ebayDetails[grade]
		}
		if estimateDetails != nil {
			detail.Estimate = estimateDetails[grade]
		}
		if detail.Ebay != nil || detail.Estimate != nil {
			result.GradeDetails[grade] = detail
		}
	}

	// Collect source names
	result.Sources = []string{}
	if result.FusionMetadata != nil {
		for _, sr := range result.FusionMetadata.SourceResults {
			if sr.Success {
				result.Sources = append(result.Sources, sr.Source)
			}
		}
	}
	if pcPrice != nil {
		result.Sources = append(result.Sources, "pricecharting")
	}
}

// GetStats implements pricing.PriceProvider interface
// Delegates to PriceCharting provider for statistics
// The context parameter enables request cancellation and timeout propagation.
func (f *FusionPriceProvider) GetStats(ctx context.Context) *pricing.ProviderStats {
	if f.priceCharting == nil {
		return nil
	}
	return f.priceCharting.GetStats(ctx)
}
