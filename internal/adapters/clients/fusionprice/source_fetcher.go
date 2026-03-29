// source_fetcher.go orchestrates concurrent fetches from all available pricing sources.
//
// # Context Flags
//
// Two context-value flags control source selection and fallback behavior:
//
//   - withOnDemand (ctxKeyOnDemand):
//     Marks the call as user-initiated (e.g. via LookupCard).
//     When set, PriceCharting is excluded (already queried upstream with the
//     correct product identity). All secondary sources (CardHedger)
//     remain available; each has its own rate limiter and
//     429-block tracking. Set by FusionPriceProvider.LookupCard.
//
//   - withNoStale (ctxKeyNoStale):
//     Prevents falling back to stale DB prices.
//     Used when canonical card identity has been resolved and stale data stored
//     under a previous name would be incorrect. Set by FusionPriceProvider.LookupCard
//     when PriceCharting returns a different product ID than what was cached.
package fusionprice

import (
	"context"
	"fmt"
	"sync"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// pcFallbackConfidence is the confidence score assigned to PriceCharting grades
// when used as single-source fallback data (secondary sources all failed).
const pcFallbackConfidence = 0.80

// ctxKeyOnDemand is a context key that marks a call as user-initiated (on-demand).
// When set, PriceCharting is excluded (already queried upstream).
type ctxKeyOnDemand struct{}

// withOnDemand returns a context marked as an on-demand (user-initiated) lookup.
func withOnDemand(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyOnDemand{}, true)
}

// isOnDemand returns true if the context was marked as on-demand.
func isOnDemand(ctx context.Context) bool {
	v, ok := ctx.Value(ctxKeyOnDemand{}).(bool)
	return ok && v
}

// ctxKeyNoStale is a context key that prevents stale DB fallback.
// Used when canonical card identity is available and stale data under
// the old name would be incorrect.
type ctxKeyNoStale struct{}

// withNoStale returns a context that suppresses stale DB price fallback.
func withNoStale(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyNoStale{}, true)
}

// isNoStale returns true if the context suppresses stale DB fallback.
func isNoStale(ctx context.Context) bool {
	v, ok := ctx.Value(ctxKeyNoStale{}).(bool)
	return ok && v
}

// sourceResultEntry captures a source result for tracking
type sourceResultEntry struct {
	source  string
	success bool
	errMsg  string
}

// isProviderClientAvailable checks if the client for a given provider exists and is available
func (f *FusionPriceProvider) isProviderClientAvailable(provider string) bool {
	if provider == "pricecharting" {
		return f.priceCharting != nil && f.priceCharting.Available()
	}
	for _, source := range f.secondarySources {
		if source.Name() == provider {
			return source.Available()
		}
	}
	return false
}

// getAvailableSources checks which providers are available (not blocked).
// On-demand calls (via LookupCard) exclude PriceCharting (already queried
// by LookupCard with the correct product identity). All secondary sources
// (CardHedger) remain available — each has its own rate
// limiter and 429-block tracking, so individual source failures don't
// cascade to other sources.
func (f *FusionPriceProvider) getAvailableSources(ctx context.Context) []string {
	onDemand := isOnDemand(ctx)

	// Build list of all provider names: pricecharting + secondary sources
	var providers []string
	if !onDemand {
		providers = append(providers, "pricecharting")
	}
	for _, source := range f.secondarySources {
		providers = append(providers, source.Name())
	}

	if f.apiTracker == nil {
		// No API tracker - all configured sources available (if client exists and is available)
		var sources []string
		for _, provider := range providers {
			if f.isProviderClientAvailable(provider) {
				sources = append(sources, provider)
			}
		}
		return sources
	}

	var sources []string
	for _, provider := range providers {
		// First check if client exists and is available
		if !f.isProviderClientAvailable(provider) {
			continue
		}

		blocked, until, err := f.apiTracker.IsProviderBlocked(ctx, provider)
		if err != nil {
			if f.logger != nil {
				f.logger.Warn(ctx, "failed to check block status",
					observability.String("provider", provider),
					observability.Err(err))
			}
			// Assume available on error (client already verified above)
			sources = append(sources, provider)
			continue
		}
		if blocked {
			if f.logger != nil {
				f.logger.Warn(ctx, "provider blocked, skipping",
					observability.String("provider", provider),
					observability.Time("blocked_until", until))
			}
			continue
		}

		sources = append(sources, provider)
	}

	return sources
}

// fetchFromAvailableSources fetches price data only from available (non-blocked) sources.
// Returns grade-keyed price data, all FetchResults from secondary sources (for detail data),
// the PriceCharting result, source result entries, and any error.
func (f *FusionPriceProvider) fetchFromAvailableSources(ctx context.Context, card pricing.Card, availableSources []string, collector *CardSyncCollector) (map[string][]fusion.PriceData, []*fusion.FetchResult, *pricing.Price, []pricing.SourceResult, error) {
	pricesByGrade := make(map[string][]fusion.PriceData)
	var fetchResults []*fusion.FetchResult
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(availableSources))
	resultsChan := make(chan sourceResultEntry, len(availableSources))

	var pcPriceResult *pricing.Price
	var pcPriceMu sync.Mutex

	// Helper to check if source is available
	isAvailable := func(source string) bool {
		for _, s := range availableSources {
			if s == source {
				return true
			}
		}
		return false
	}

	// Fetch from PriceCharting (if not blocked)
	if isAvailable("pricecharting") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			pcCtx, cancel := context.WithTimeout(ctx, f.priceChartingTimeout)
			defer cancel()

			pcPrice, err := f.priceCharting.GetPrice(pcCtx, card)
			latency := time.Since(start)

			// Record API call
			statusCode := 200
			if err != nil {
				statusCode = 500
				errChan <- fmt.Errorf("pricecharting: %w", err)
				resultsChan <- sourceResultEntry{source: "pricecharting", success: false, errMsg: err.Error()}
			} else {
				resultsChan <- sourceResultEntry{source: "pricecharting", success: true}
			}
			f.recordAPICall(ctx, "pricecharting", statusCode, err, latency, &fusion.ResponseMeta{StatusCode: statusCode})

			// Record to collector for unified logging
			collector.RecordSource(SourcePriceCharting, err == nil, err, latency, false)

			if err != nil {
				return
			}

			pcPriceMu.Lock()
			pcPriceResult = pcPrice
			pcPriceMu.Unlock()
			// PriceCharting is used for market data only (sales velocity, listings,
			// conservative exits, last sold). Its prices are NOT fed into the fusion
			// engine — CardHedger provides graded pricing.
		}()
	}

	// Fetch from secondary sources (if available and not blocked)
	for _, source := range f.secondarySources {
		sourceName := source.Name()
		if !isAvailable(sourceName) {
			continue
		}
		wg.Add(1)
		go func(src fusion.SecondaryPriceSource, name string) {
			defer wg.Done()
			start := time.Now()
			srcCtx, cancel := context.WithTimeout(ctx, f.secondarySourceTimeout)
			defer cancel()

			fetchResult, meta, err := src.FetchFusionData(srcCtx, card)
			latency := time.Since(start)

			statusCode := 0
			if meta != nil {
				statusCode = meta.StatusCode
			}
			if err != nil {
				if statusCode == 0 {
					statusCode = 500 // Default to internal server error if no status code
				}
				errChan <- fmt.Errorf("%s: %w", name, err)
				resultsChan <- sourceResultEntry{source: name, success: false, errMsg: err.Error()}
			} else {
				resultsChan <- sourceResultEntry{source: name, success: true}
			}
			f.recordAPICall(ctx, name, statusCode, err, latency, meta)

			// Record to collector for unified logging
			obsSource, ok := observabilitySourceName[name]
			if !ok {
				obsSource = name
			}
			collector.RecordSource(obsSource, err == nil, err, latency, false)

			if err != nil || fetchResult == nil {
				return
			}

			mu.Lock()
			for grade, data := range fetchResult.GradeData {
				pricesByGrade[grade] = append(pricesByGrade[grade], data...)
			}
			fetchResults = append(fetchResults, fetchResult)
			mu.Unlock()
		}(source, sourceName)
	}

	wg.Wait()
	close(errChan)
	close(resultsChan)

	// Collect source results
	var sourceResults []pricing.SourceResult
	for entry := range resultsChan {
		sourceResults = append(sourceResults, pricing.SourceResult{
			Source:  entry.source,
			Success: entry.success,
			Error:   entry.errMsg,
		})
	}

	// Check for errors (but don't fail if one source fails)
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	// If ALL available sources failed, return error
	if len(errs) >= len(availableSources) {
		return nil, nil, nil, sourceResults, apperrors.ProviderUnavailable("fusion", fmt.Errorf("all available sources failed: %v", errs))
	}

	// Fallback: when secondary sources all failed but PriceCharting has graded prices,
	// inject PC grades as single-source fallback data. This mirrors the fallback
	// pattern in LookupCard (fusion_provider.go:476-487).
	if len(pricesByGrade) == 0 && pcPriceResult != nil {
		pcFallback := map[string]int64{
			"PSA 10": pcPriceResult.Grades.PSA10Cents,
			"PSA 9":  pcPriceResult.Grades.PSA9Cents,
			"PSA 8":  pcPriceResult.Grades.PSA8Cents,
			"Raw":    pcPriceResult.Grades.RawCents,
		}
		for grade, cents := range pcFallback {
			if cents <= 0 {
				continue
			}
			pricesByGrade[grade] = []fusion.PriceData{{
				Value:    float64(cents) / 100.0,
				Currency: "USD",
				Source: fusion.DataSource{
					Name:       "pricecharting",
					Confidence: pcFallbackConfidence,
				},
			}}
		}
		if len(pricesByGrade) > 0 && f.logger != nil {
			f.logger.Info(ctx, "using PriceCharting graded prices as fallback (secondary sources failed)",
				observability.String("card", card.Name),
				observability.Int("grades", len(pricesByGrade)))
		}
	}

	// If no data was collected at all (including PC fallback), return error
	if len(pricesByGrade) == 0 {
		return nil, nil, nil, sourceResults, apperrors.ProviderInvalidResponse("fusion", fmt.Errorf("no price data collected from any source"))
	}

	// Detect cross-source price divergence (>3x difference)
	f.detectPriceDivergence(ctx, pricesByGrade, card)

	return pricesByGrade, fetchResults, pcPriceResult, sourceResults, nil
}

// divergenceThreshold is the ratio above which two prices are considered divergent.
const divergenceThreshold = 3.0

// detectPriceDivergence logs a warning when prices from different sources for
// the same grade diverge by more than 3x. This is an audit signal — the data
// still enters fusion, but the confidence penalty is applied by the caller.
func (f *FusionPriceProvider) detectPriceDivergence(ctx context.Context, pricesByGrade map[string][]fusion.PriceData, card pricing.Card) {
	if f.logger == nil {
		return
	}
	for grade, prices := range pricesByGrade {
		if len(prices) < 2 {
			continue
		}
		minVal, maxVal := prices[0].Value, prices[0].Value
		minSrc, maxSrc := prices[0].Source.Name, prices[0].Source.Name
		for _, p := range prices[1:] {
			if p.Value < minVal {
				minVal = p.Value
				minSrc = p.Source.Name
			}
			if p.Value > maxVal {
				maxVal = p.Value
				maxSrc = p.Source.Name
			}
		}
		if minVal > 0 && maxVal/minVal > divergenceThreshold {
			f.logger.Warn(ctx, "cross-source price divergence detected",
				observability.String("card", card.Name),
				observability.String("grade", grade),
				observability.Float64("low_price", minVal),
				observability.String("low_source", minSrc),
				observability.Float64("high_price", maxVal),
				observability.String("high_source", maxSrc),
				observability.Float64("ratio", maxVal/minVal))
		}
	}
}
