package fusionprice

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertToPriceResponse tests fusion price conversion
func TestConvertToPriceResponse(t *testing.T) {
	// Create test cache
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	// Create mock fused prices
	fusedPrices := map[string]*fusion.FusedPrice{
		"psa10": {
			Value:         150.00,
			Confidence:    0.95,
			SourceCount:   3,
			OutliersFound: 0,
			Method:        "weighted_median",
		},
		"psa9": {
			Value:         80.00,
			Confidence:    0.92,
			SourceCount:   3,
			OutliersFound: 0,
			Method:        "weighted_median",
		},
		"raw": {
			Value:         30.00,
			Confidence:    0.88,
			SourceCount:   2,
			OutliersFound: 1,
			Method:        "weighted_median",
		},
	}

	// Convert to price response
	result := fp.convertToPriceResponse(fusedPrices)

	// Verify conversion
	assert.NotNil(t, result)
	assert.Equal(t, int64(15000), result.Grades.PSA10Cents, "PSA 10 should be $150.00 = 15000 cents")
	assert.Equal(t, int64(8000), result.Grades.PSA9Cents, "PSA 9 should be $80.00 = 8000 cents")
	assert.Equal(t, int64(3000), result.Grades.RawCents, "Raw should be $30.00 = 3000 cents")
	assert.Equal(t, int64(15000), result.Amount, "Amount should equal PSA10")

	// Verify confidence (average of all grades)
	expectedConf := (0.95 + 0.92 + 0.88) / 3.0
	assert.InDelta(t, expectedConf, result.Confidence, 0.01)

	// Verify fusion metadata
	assert.NotNil(t, result.FusionMetadata)
	assert.Equal(t, "weighted_median", result.FusionMetadata.Method)
	assert.Equal(t, 1, result.FusionMetadata.OutliersFound, "Should have 1 outlier from raw price")
}

// TestConvertToPriceResponse_EmptyGrades tests conversion with no grades
func TestConvertToPriceResponse_EmptyGrades(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	fusedPrices := map[string]*fusion.FusedPrice{}

	result := fp.convertToPriceResponse(fusedPrices)

	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.Amount)
	assert.Equal(t, 0.0, result.Confidence)
	assert.NotNil(t, result.FusionMetadata)
	assert.Equal(t, 0, result.FusionMetadata.SourceCount)
}

// TestConvertToPriceResponse_AllGrades tests conversion with all grade types
func TestConvertToPriceResponse_AllGrades(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	fusedPrices := map[string]*fusion.FusedPrice{
		"psa10": {Value: 200.00, Confidence: 0.95},
		"psa9":  {Value: 120.00, Confidence: 0.92},
		"psa8":  {Value: 70.00, Confidence: 0.90},
		"cgc95": {Value: 150.00, Confidence: 0.93},
		"bgs10": {Value: 250.00, Confidence: 0.96},
		"raw":   {Value: 40.00, Confidence: 0.85},
	}

	result := fp.convertToPriceResponse(fusedPrices)

	assert.Equal(t, int64(20000), result.Grades.PSA10Cents)
	assert.Equal(t, int64(12000), result.Grades.PSA9Cents)
	assert.Equal(t, int64(7000), result.Grades.PSA8Cents)
	assert.Equal(t, int64(15000), result.Grades.Grade95Cents)
	assert.Equal(t, int64(25000), result.Grades.BGS10Cents)
	assert.Equal(t, int64(4000), result.Grades.RawCents)
	assert.Equal(t, int64(20000), result.Amount)
}

// TestFusionProvider_Available tests availability check
func TestFusionProvider_Available(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	// Should be available with valid price provider
	assert.True(t, fp.Available())
}

// TestFusionProvider_Name tests provider name
func TestFusionProvider_Name(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	assert.Equal(t, "fusion", fp.Name())
}

// TestFusionProvider_Close tests resource cleanup
func TestFusionProvider_Close(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	err = fp.Close()
	assert.NoError(t, err)
}

// TestFusionProvider_LookupCard tests introspection delegation
func TestFusionProvider_LookupCard(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	card := domainCards.Card{
		Name:   "Charizard",
		Number: "006",
	}

	// LookupCard should delegate to price charting
	// This will fail without real API credentials, but we test the delegation works
	// We don't assert on error since this depends on API availability
	// Just test that the method exists and doesn't panic
	assert.NotPanics(t, func() {
		_, _ = fp.LookupCard(context.Background(), "Base Set", card)
	})
}

// TestFusionProvider_GetStats tests stats delegation
func TestFusionProvider_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	stats := fp.GetStats(context.Background())
	assert.NotNil(t, stats)
}

// TestFusionProvider_GetCached tests cache operations
func TestFusionProvider_GetCached(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)
	ctx := context.Background()

	// Test cache miss
	price, err := fp.getCached(ctx, "non-existent-key")
	assert.Error(t, err)
	assert.Nil(t, price)

	// Set cache value
	testPrice := &pricing.Price{
		Amount:   10000,
		Currency: "USD",
	}
	fp.setCached(ctx, "test-key", testPrice, 1*time.Hour)

	// Test cache hit
	cachedPrice, err := fp.getCached(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, testPrice.Amount, cachedPrice.Amount)
}

// TestFusionProvider_GetCached_NilCache tests cache operations with nil cache
func TestFusionProvider_GetCached_NilCache(t *testing.T) {
	mockPC := mocks.NewMockPriceProvider()

	fp := &FusionPriceProvider{
		priceCharting: mockPC,
		cache:         nil, // Nil cache
	}
	ctx := context.Background()

	// getCached should return error with nil cache
	price, err := fp.getCached(ctx, "test-key")
	assert.Error(t, err)
	assert.Nil(t, price)

	// setCached should not panic with nil cache
	assert.NotPanics(t, func() {
		fp.setCached(ctx, "test-key", &pricing.Price{}, 1*time.Hour)
	})
}

// TestFusionProvider_ContextCancellation tests context cancellation
func TestFusionProvider_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	mockPC := mocks.NewMockPriceProvider()

	fp := NewFusionProviderWithRepo(mockPC, nil, testCache, nil, nil, nil, observability.NewNoopLogger(), 0, 0, 0, 0)

	testCard := pricing.Card{
		Name:   "Test Card",
		Set:    "Test Set",
		Number: "001",
	}

	// Create context with immediate cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Get price with cancelled context
	price, err := fp.GetPrice(ctx, testCard)

	// Should return error due to cancellation (or no data)
	// We can't guarantee cancellation error since price charting might be cached or immediate
	_ = price
	_ = err
	// Just test that it doesn't panic
	assert.NotPanics(t, func() {
		_, _ = fp.GetPrice(ctx, testCard)
	})
}

// TestComputeRateLimitReset_NilHeaders tests fallback to short backoff
func TestComputeRateLimitReset_NilHeaders(t *testing.T) {
	now := time.Now()
	result := computeRateLimitReset(nil)

	// Should be approximately 90 seconds from now (short backoff for transient rate limits)
	assert.True(t, result.After(now.Add(80*time.Second)), "Should be at least 80 seconds in future")
	assert.True(t, result.Before(now.Add(100*time.Second)), "Should be at most 100 seconds in future")
}

// TestComputeRateLimitReset_EmptyHeaders tests fallback with empty headers
func TestComputeRateLimitReset_EmptyHeaders(t *testing.T) {
	headers := make(http.Header)
	result := computeRateLimitReset(headers)

	// Should use fallback (next UTC midnight or 24h)
	now := time.Now()
	assert.True(t, result.After(now), "Reset time should be in the future")
	assert.True(t, result.Before(now.Add(25*time.Hour)), "Reset time should be within 25 hours")
}

// TestComputeRateLimitReset_XRateLimitReset tests Unix timestamp parsing
func TestComputeRateLimitReset_XRateLimitReset(t *testing.T) {
	now := time.Now()
	expectedReset := now.Add(6 * time.Hour).Unix()

	headers := make(http.Header)
	headers.Set("X-RateLimit-Reset", fmt.Sprintf("%d", expectedReset))

	result := computeRateLimitReset(headers)

	assert.Equal(t, expectedReset, result.Unix(), "Should parse X-RateLimit-Reset header")
}

// TestComputeRateLimitReset_RetryAfterSeconds tests Retry-After in seconds
func TestComputeRateLimitReset_RetryAfterSeconds(t *testing.T) {
	now := time.Now()

	headers := make(http.Header)
	headers.Set("Retry-After", "3600") // 1 hour

	result := computeRateLimitReset(headers)

	// Should be approximately 1 hour in future
	assert.True(t, result.After(now.Add(59*time.Minute)), "Should be at least 59 minutes in future")
	assert.True(t, result.Before(now.Add(61*time.Minute)), "Should be at most 61 minutes in future")
}

// TestComputeRateLimitReset_RetryAfterHTTPDate tests Retry-After as HTTP date
func TestComputeRateLimitReset_RetryAfterHTTPDate(t *testing.T) {
	// Create a future time in UTC that will pass validation (must be after now)
	// Must use UTC because http.TimeFormat always uses GMT timezone
	now := time.Now().UTC()
	expectedReset := now.Add(2 * time.Hour).Truncate(time.Second) // Truncate to second precision

	headers := make(http.Header)
	headers.Set("Retry-After", expectedReset.Format(http.TimeFormat))

	result := computeRateLimitReset(headers)

	// The result should match within a few seconds tolerance to account for test execution time
	assert.InDelta(t, expectedReset.Unix(), result.Unix(), 2, "Should parse HTTP date")
}

// TestComputeRateLimitReset_HeaderPriority tests header precedence
func TestComputeRateLimitReset_HeaderPriority(t *testing.T) {
	now := time.Now()
	expectedReset := now.Add(4 * time.Hour).Unix()

	headers := make(http.Header)
	// X-RateLimit-Reset takes priority over Retry-After
	headers.Set("X-RateLimit-Reset", fmt.Sprintf("%d", expectedReset))
	headers.Set("Retry-After", "7200") // 2 hours - should be ignored

	result := computeRateLimitReset(headers)

	assert.Equal(t, expectedReset, result.Unix(), "X-RateLimit-Reset should take priority")
}

// TestComputeRateLimitReset_InvalidHeaders tests fallback on invalid headers
func TestComputeRateLimitReset_InvalidHeaders(t *testing.T) {
	now := time.Now()

	headers := make(http.Header)
	headers.Set("X-RateLimit-Reset", "not-a-number")
	headers.Set("Retry-After", "also-invalid")

	result := computeRateLimitReset(headers)

	// Should use fallback
	assert.True(t, result.After(now), "Reset time should be in the future")
	assert.True(t, result.Before(now.Add(25*time.Hour)), "Reset time should be within 25 hours")
}

// TestComputeRateLimitReset_PastTimestamp tests rejection of past timestamps
func TestComputeRateLimitReset_PastTimestamp(t *testing.T) {
	now := time.Now()
	pastTime := now.Add(-1 * time.Hour).Unix()

	headers := make(http.Header)
	headers.Set("X-RateLimit-Reset", fmt.Sprintf("%d", pastTime))

	result := computeRateLimitReset(headers)

	// Should use fallback because timestamp is in the past
	assert.True(t, result.After(now), "Reset time should be in the future")
}

// TestComputeRateLimitReset_FarFutureTimestamp tests rejection of far future timestamps
func TestComputeRateLimitReset_FarFutureTimestamp(t *testing.T) {
	now := time.Now()
	farFuture := now.Add(30 * 24 * time.Hour).Unix() // 30 days

	headers := make(http.Header)
	headers.Set("X-RateLimit-Reset", fmt.Sprintf("%d", farFuture))

	result := computeRateLimitReset(headers)

	// Should use fallback because timestamp is too far in future (> 7 days)
	assert.True(t, result.After(now), "Reset time should be in the future")
	assert.True(t, result.Before(now.Add(8*24*time.Hour)), "Reset time should be within 8 days")
}

// TestFusionProvider_AttachSourceDetails tests the extracted attachSourceDetails method.
// Detail data is passed via FetchResult values (no shared mutable state on adapters).
func TestFusionProvider_AttachSourceDetails(t *testing.T) {
	// Build FetchResults representing data from eBay and CardHedger
	ebayResult := &fusion.FetchResult{
		EbayDetails: map[string]*pricing.EbayGradeDetail{
			"raw":   {PriceCents: 48700, Confidence: "low", SalesCount: 63, Trend: "down"},
			"psa8":  {PriceCents: 117499, Confidence: "high", SalesCount: 26, Trend: "up"},
			"psa9":  {PriceCents: 250000, Confidence: "low", Trend: "down"},
			"psa10": {PriceCents: 1487500, SalesCount: 1},
		},
		Velocity: &pricing.SalesVelocity{
			DailyAverage:  1.07,
			WeeklyAverage: 7.44,
			MonthlyTotal:  32,
		},
	}
	chResult := &fusion.FetchResult{
		EstimateDetails: map[string]*pricing.EstimateGradeDetail{
			"raw":   {PriceCents: 52500, Confidence: 0.85},
			"psa8":  {PriceCents: 320050, Confidence: 0.85},
			"psa9":  {PriceCents: 850000, Confidence: 0.85},
			"psa10": {PriceCents: 1699999, Confidence: 0.85},
		},
	}

	fp := &FusionPriceProvider{}

	// Build a result with FusionMetadata containing source results
	result := &pricing.Price{
		FusionMetadata: &pricing.FusionMetadata{
			SourceResults: []pricing.SourceResult{
				{Source: "cardhedger", Success: true},
			},
		},
	}

	// Use a non-nil pcPrice to include "pricecharting" in Sources
	pcPrice := &pricing.Price{}

	fp.attachSourceDetails(result, []*fusion.FetchResult{ebayResult, chResult}, pcPrice)

	// Verify GradeDetails
	require.NotNil(t, result.GradeDetails, "expected GradeDetails to be non-nil")
	for _, grade := range []string{"raw", "psa8", "psa9", "psa10"} {
		gd := result.GradeDetails[grade]
		require.NotNilf(t, gd, "expected GradeDetails[%q] to be non-nil", grade)
		assert.NotNilf(t, gd.Ebay, "expected GradeDetails[%q].Ebay to be non-nil", grade)
		assert.NotNilf(t, gd.Estimate, "expected GradeDetails[%q].Estimate to be non-nil", grade)
	}

	// Verify specific values
	assert.Equal(t, int64(1487500), result.GradeDetails["psa10"].Ebay.PriceCents, "psa10 Ebay PriceCents")
	assert.Equal(t, int64(1699999), result.GradeDetails["psa10"].Estimate.PriceCents, "psa10 Estimate PriceCents")
	assert.Equal(t, int64(117499), result.GradeDetails["psa8"].Ebay.PriceCents, "psa8 Ebay PriceCents")
	assert.Equal(t, int64(320050), result.GradeDetails["psa8"].Estimate.PriceCents, "psa8 Estimate PriceCents")

	// Verify Velocity
	require.NotNil(t, result.Velocity, "expected Velocity to be non-nil")
	assert.Equal(t, 1.07, result.Velocity.DailyAverage)
	assert.Equal(t, 7.44, result.Velocity.WeeklyAverage)
	assert.Equal(t, 32, result.Velocity.MonthlyTotal)

	// Verify Sources contains cardhedger and pricecharting
	assert.Contains(t, result.Sources, "cardhedger", "Sources should contain cardhedger")
	assert.Contains(t, result.Sources, "pricecharting", "Sources should contain pricecharting")
}

// gradedPCProvider is a test mock that returns graded prices from GetPrice.
type gradedPCProvider struct {
	grades pricing.GradedPrices
}

func (g *gradedPCProvider) GetPrice(_ context.Context, _ pricing.Card) (*pricing.Price, error) {
	return &pricing.Price{
		Amount:   15000,
		Currency: "USD",
		Source:   pricing.SourcePriceCharting,
		Grades:   g.grades,
	}, nil
}
func (g *gradedPCProvider) LookupCard(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
	return nil, fmt.Errorf("not implemented")
}
func (g *gradedPCProvider) Available() bool { return true }
func (g *gradedPCProvider) Name() string    { return "gradedPCProvider" }
func (g *gradedPCProvider) Close() error    { return nil }
func (g *gradedPCProvider) GetStats(_ context.Context) *pricing.ProviderStats {
	return &pricing.ProviderStats{}
}

// TestFetchFromAvailableSources_PCGradesFallback verifies that when secondary sources
// fail but PriceCharting succeeds with graded prices, those PC grades are injected
// as fallback data into pricesByGrade.
func TestFetchFromAvailableSources_PCGradesFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	require.NoError(t, err)
	defer testCache.Close()

	// Create a PC provider that returns graded prices
	pcProvider := &gradedPCProvider{
		grades: pricing.GradedPrices{
			PSA10Cents: 15000, // $150.00
			PSA9Cents:  8000,  // $80.00
			PSA8Cents:  4000,  // $40.00
			RawCents:   2000,  // $20.00
		},
	}

	// Build FusionProvider with NO secondary sources
	fp := NewFusionProviderWithRepo(
		pcProvider,
		nil, // no secondary sources
		testCache,
		nil, nil, nil,
		observability.NewNoopLogger(),
		0, 0, 0, 0,
	)

	card := pricing.Card{Name: "Charizard", Set: "Base Set", Number: "4"}
	collector := NewCardSyncCollector(observability.NewNoopLogger(), card.Name, card.Number, card.Set)

	// Call with only pricecharting available — no secondary sources exist.
	// PC succeeds with graded prices, so the fallback should inject them.
	pricesByGrade, _, pcResult, sourceResults, fetchErr := fp.fetchFromAvailableSources(
		context.Background(), card, []string{"pricecharting"}, collector,
	)

	// Should NOT error — PC grades fallback should have populated pricesByGrade
	require.NoError(t, fetchErr, "should not error when PC grades fallback is available")
	require.NotNil(t, pcResult, "pcResult should be returned")

	// Verify PC source was recorded as successful
	require.Len(t, sourceResults, 1)
	assert.Equal(t, "pricecharting", sourceResults[0].Source)
	assert.True(t, sourceResults[0].Success)

	// Verify fallback grades were injected
	assert.Len(t, pricesByGrade, 4, "should have 4 grades from PC fallback")

	expectedGrades := map[string]float64{
		"PSA 10": 150.00,
		"PSA 9":  80.00,
		"PSA 8":  40.00,
		"Raw":    20.00,
	}
	for grade, expectedValue := range expectedGrades {
		prices, ok := pricesByGrade[grade]
		require.True(t, ok, "should have grade %q", grade)
		require.Len(t, prices, 1, "should have 1 price for grade %q", grade)
		assert.InDelta(t, expectedValue, prices[0].Value, 0.01, "grade %q value", grade)
		assert.Equal(t, "pricecharting", prices[0].Source.Name, "grade %q source", grade)
	}
}
