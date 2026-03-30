package fusionprice

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// convertToPriceResponse converts fused prices to pricing.Price format
func (f *FusionPriceProvider) convertToPriceResponse(fusedPrices map[string]*fusion.FusedPrice) *pricing.Price {
	price := &pricing.Price{
		Currency: "USD",
		Source:   "fusion",
	}

	// Extract prices by grade
	if psa10 := fusedPrices["psa10"]; psa10 != nil {
		price.Grades.PSA10Cents = mathutil.ToCents(psa10.Value)
		price.Amount = price.Grades.PSA10Cents // Primary price
	}
	if psa9 := fusedPrices["psa9"]; psa9 != nil {
		price.Grades.PSA9Cents = mathutil.ToCents(psa9.Value)
	}
	if psa8 := fusedPrices["psa8"]; psa8 != nil {
		price.Grades.PSA8Cents = mathutil.ToCents(psa8.Value)
	}
	if grade95 := fusedPrices["cgc95"]; grade95 != nil {
		price.Grades.Grade95Cents = mathutil.ToCents(grade95.Value)
	}
	if bgs10 := fusedPrices["bgs10"]; bgs10 != nil {
		price.Grades.BGS10Cents = mathutil.ToCents(bgs10.Value)
	}
	if raw := fusedPrices["raw"]; raw != nil {
		price.Grades.RawCents = mathutil.ToCents(raw.Value)
	}

	// Calculate overall confidence (average of all grades)
	totalConfidence := 0.0
	gradeCount := 0
	var sources []string
	sourceMap := make(map[string]bool)
	totalOutliers := 0
	fusionMethod := ""

	for _, fusedPrice := range fusedPrices {
		if fusedPrice == nil {
			continue
		}
		totalConfidence += fusedPrice.Confidence
		gradeCount++
		totalOutliers += fusedPrice.OutliersFound

		// Capture first non-empty method from grades
		if fusionMethod == "" && fusedPrice.Method != "" {
			fusionMethod = fusedPrice.Method
		}

		// Collect unique sources
		for _, sourceData := range fusedPrice.Sources {
			if !sourceMap[sourceData.Source] {
				sourceMap[sourceData.Source] = true
				sources = append(sources, sourceData.Source)
			}
		}
	}

	if gradeCount > 0 {
		price.Confidence = totalConfidence / float64(gradeCount)
	}

	if fusionMethod == "" {
		fusionMethod = "weighted_median"
	}

	// Build fusion metadata - use unique provider count from sourceMap
	price.FusionMetadata = &pricing.FusionMetadata{
		SourceCount:   len(sourceMap),
		OutliersFound: totalOutliers,
		Method:        fusionMethod,
		Sources:       sources,
	}

	return price
}

func (f *FusionPriceProvider) getCached(ctx context.Context, key string) (*pricing.Price, error) {
	if f.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}

	var price pricing.Price
	found, err := f.cache.Get(ctx, key, &price)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("not found")
	}

	return &price, nil
}

func (f *FusionPriceProvider) setCached(ctx context.Context, key string, price *pricing.Price, ttl time.Duration) {
	if f.cache == nil {
		return
	}

	if err := f.cache.Set(ctx, key, price, ttl); err != nil {
		if f.logger != nil {
			f.logger.Warn(ctx, "Failed to cache fusion price",
				observability.Err(err),
				observability.String("key", key))
		}
	}
}

// persistToDatabase stores fused prices and PriceCharting raw grades to database.
func (f *FusionPriceProvider) persistToDatabase(ctx context.Context, card pricing.Card, price *pricing.Price) {
	if f.priceRepo == nil {
		return
	}

	// Define all grades to persist with their corresponding price values
	gradePrices := []struct {
		grade      string
		priceCents int64
	}{
		{"PSA 10", price.Grades.PSA10Cents},
		{"PSA 9", price.Grades.PSA9Cents},
		{"PSA 8", price.Grades.PSA8Cents},
		{"CGC 9.5", price.Grades.Grade95Cents},
		{"BGS 10", price.Grades.BGS10Cents},
		{"Raw", price.Grades.RawCents},
	}

	now := time.Now()
	for _, gp := range gradePrices {
		if gp.priceCents <= 0 {
			continue
		}

		fusedEntry := &pricing.PriceEntry{
			CardName:              card.Name,
			SetName:               card.Set,
			CardNumber:            card.Number,
			Grade:                 gp.grade,
			PriceCents:            gp.priceCents,
			Confidence:            price.Confidence,
			Source:                "fusion",
			FusionSourceCount:     price.FusionMetadata.SourceCount,
			FusionOutliersRemoved: price.FusionMetadata.OutliersFound,
			FusionMethod:          price.FusionMetadata.Method,
			PriceDate:             now,
		}

		if err := f.priceRepo.StorePrice(ctx, fusedEntry); err != nil {
			if f.logger != nil {
				f.logger.Warn(ctx, "failed to persist fused price",
					observability.Err(err),
					observability.String("card", card.Name),
					observability.String("grade", gp.grade))
			}
		}
	}

	// Also persist PriceCharting's raw grade prices so they survive cache expiry.
	if price.PCGrades != nil {
		pcGrades := []struct {
			grade      string
			priceCents int64
		}{
			{"PSA 10", price.PCGrades.PSA10Cents},
			{"PSA 9", price.PCGrades.PSA9Cents},
			{"PSA 8", price.PCGrades.PSA8Cents},
			{"Raw", price.PCGrades.RawCents},
		}
		for _, gp := range pcGrades {
			if gp.priceCents <= 0 {
				continue
			}
			pcEntry := &pricing.PriceEntry{
				CardName:   card.Name,
				SetName:    card.Set,
				CardNumber: card.Number,
				Grade:      gp.grade,
				PriceCents: gp.priceCents,
				Confidence: 0.90,
				Source:     pricing.SourcePriceCharting,
				PriceDate:  now,
			}
			if err := f.priceRepo.StorePrice(ctx, pcEntry); err != nil {
				if f.logger != nil {
					f.logger.Warn(ctx, "failed to persist pricecharting grade",
						observability.Err(err),
						observability.String("card", card.Name),
						observability.String("grade", gp.grade))
				}
			}
		}
	}
}

// recordAPICall tracks API calls to providers.
// meta is optional and can be nil - if provided and status is 429, pre-parsed rate limit reset will be used.
func (f *FusionPriceProvider) recordAPICall(ctx context.Context, provider string, statusCode int, err error, latency time.Duration, meta *fusion.ResponseMeta) {
	if f.apiTracker == nil {
		return
	}

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	call := &pricing.APICallRecord{
		Provider:   provider,
		StatusCode: statusCode,
		Error:      errMsg,
		LatencyMS:  latency.Milliseconds(),
		Timestamp:  time.Now(),
	}

	if recordErr := f.apiTracker.RecordAPICall(ctx, call); recordErr != nil {
		// Log but don't return - still need to handle 429 rate limiting below
		if f.logger != nil {
			f.logger.Debug(ctx, "failed to record API call", observability.Err(recordErr), observability.String("provider", provider))
		}
	}

	// If 429 (rate limited), update rate limit status (best-effort, non-critical)
	if statusCode == 429 {
		var resetTime time.Time
		if meta != nil && meta.RateLimitReset != nil {
			resetTime = *meta.RateLimitReset
		} else {
			resetTime = computeRateLimitReset(nil)
		}
		if err := f.apiTracker.UpdateRateLimit(ctx, provider, resetTime); err != nil && f.logger != nil {
			f.logger.Debug(ctx, "failed to update rate limit status", observability.Err(err), observability.String("provider", provider))
		}
	}
}

// computeRateLimitReset determines when the rate limit resets based on HTTP headers.
// Checks absolute Unix timestamp headers, RateLimit-Reset (RFC 9110), and Retry-After.
// When a 429 response lacks these headers, uses a fixed 90-second backoff targeting
// transient per-second/per-minute limits rather than the previous midnight/24h fallback.
func computeRateLimitReset(headers http.Header) time.Time {
	now := time.Now()

	if headers != nil {
		// Try absolute Unix timestamp headers
		for _, headerName := range []string{
			"X-RateLimit-Reset",
			"X-Rate-Limit-Reset",
		} {
			if resetStr := headers.Get(headerName); resetStr != "" {
				if resetUnix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
					resetTime := time.Unix(resetUnix, 0)
					if resetTime.After(now) && resetTime.Before(now.Add(7*24*time.Hour)) {
						return resetTime
					}
				}
			}
		}

		// RateLimit-Reset is delta-seconds per RFC 9110
		if resetStr := headers.Get("RateLimit-Reset"); resetStr != "" {
			if seconds, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
				if seconds > 0 && seconds <= 7*24*60*60 {
					return now.Add(time.Duration(seconds) * time.Second)
				}
			}
		}

		// Try Retry-After header (seconds to wait or HTTP-date)
		if retryAfter := headers.Get("Retry-After"); retryAfter != "" {
			// Try parsing as seconds
			if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
				if seconds > 0 && seconds <= 7*24*60*60 { // Cap at 7 days
					return now.Add(time.Duration(seconds) * time.Second)
				}
			}
			// Try parsing as HTTP-date (RFC 1123)
			if resetTime, err := http.ParseTime(retryAfter); err == nil {
				if resetTime.After(now) && resetTime.Before(now.Add(7*24*time.Hour)) {
					return resetTime
				}
			}
		}
	}

	// Fallback: short backoff. Without a Retry-After header, the 429 is likely
	// a transient per-second/per-minute limit (not a daily quota exhaustion).
	// Block for 90 seconds to let the rate limit window reset, then retry.
	return now.Add(90 * time.Second)
}
