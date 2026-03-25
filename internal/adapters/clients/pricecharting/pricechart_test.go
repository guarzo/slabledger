package pricecharting

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/testutil"
)

// Mock PriceCharting API responses
var mockSingleProductResponse = map[string]interface{}{
	"status":            "success",
	"id":                "12345",
	"product-name":      "Pokemon Surging Sparks Pikachu #238",
	"loose-price":       850,  // $8.50 in cents
	"graded-price":      1500, // PSA 9
	"box-only-price":    1800, // Grade 9.5
	"manual-only-price": 2500, // PSA 10
	"bgs-10-price":      3000, // BGS 10
}

// Mock response with sales data fields
var _ = map[string]interface{}{
	"status":            "success",
	"id":                "12345",
	"product-name":      "Pokemon Surging Sparks Pikachu #238",
	"loose-price":       850,
	"graded-price":      1500,
	"box-only-price":    1800,
	"manual-only-price": 2500,
	"bgs-10-price":      3000,
	"new-price":         950,  // Sealed product
	"cib-price":         1200, // Complete in box
	"manual-price":      400,  // Manual only
	"box-price":         800,  // Box only
	"sales-volume":      25,   // Recent sales count
	"last-sold-date":    "2024-01-15",
	"retail-buy-price":  600,  // Dealer buy
	"retail-sell-price": 1100, // Dealer sell
	"sales-data": []interface{}{
		// Recent sales
		map[string]interface{}{
			"sale-price": 900,
			"sale-date":  "2024-01-15",
			"grade":      "NM",
			"source":     "eBay",
		},
		map[string]interface{}{
			"sale-price": 2600,
			"sale-date":  "2024-01-14",
			"grade":      "PSA 10",
			"source":     "PWCC",
		},
	},
}

func TestNewPriceCharting(t *testing.T) {
	// Test with token
	testToken := testutil.GetTestPriceChartingToken()
	pc1, err := NewPriceCharting(DefaultConfig(testToken), nil, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc1.Close() }()

	if pc1.token != testToken {
		t.Errorf("expected token %s, got %s", testToken, pc1.token)
	}
	// Cache manager should always be initialized
	if pc1.cacheManager == nil {
		t.Errorf("expected non-nil cache manager")
	}

	// Test with cache
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, _ := cache.New(cache.Config{Type: "file", FilePath: cachePath})

	pc2, err := NewPriceCharting(DefaultConfig(""), testCache, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc2.Close() }()

	if pc2.token != "" {
		t.Errorf("expected empty token")
	}
	if pc2.cacheManager == nil {
		t.Errorf("expected non-nil cache manager")
	}

	// Test error handling for multi-layer cache initialization
	// This would require mocking cache.NewMultiLayerCache, so we just verify no panic
}

func TestPriceCharting_Available(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "with token",
			token:    "valid-token",
			expected: true,
		},
		{
			name:     "empty token",
			token:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc, _ := NewPriceCharting(DefaultConfig(tt.token), nil, nil)
			result := pc.Available()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPriceCharting_LookupCard_NoToken(t *testing.T) {
	// Test behavior when no token is available
	pc, _ := NewPriceCharting(DefaultConfig(""), nil, nil)

	card := domainCards.Card{
		Name:    "Pikachu",
		Number:  "001",
		Set:     "surging-sparks",
		SetName: "Surging Sparks",
	}

	// Since no token, this should fail
	ctx := context.Background()
	match, err := pc.LookupCard(ctx, "Surging Sparks", card)
	if err == nil {
		t.Errorf("expected error when no token provided")
	}
	if match != nil {
		t.Errorf("expected nil match when no token")
	}
}

func TestPriceCharting_LookupCardWithCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Pre-populate cache
	cachedMatch := &PCMatch{
		ID:           "cached-123",
		ProductName:  "Cached Product",
		LooseCents:   1000,
		PSA10Cents:   5000,
		Grade9Cents:  3000,
		Grade95Cents: 4000,
		BGS10Cents:   6000,
	}

	card := domainCards.Card{Name: "Cached Card", Number: "001", Set: "test-set", SetName: "Test Set"}
	key := cache.PriceChartingKey("Test Set", card.Name, card.Number)
	err = testCache.Set(context.Background(), key, cachedMatch, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to cache data: %v", err)
	}

	pc, _ := NewPriceCharting(DefaultConfig(testutil.GetTestPriceChartingToken()), testCache, nil)

	// This should use cached data
	ctx := context.Background()
	match, err := pc.LookupCard(ctx, "Test Set", card)
	if err != nil {
		t.Fatalf("lookup with cache failed: %v", err)
	}

	if match.ID != "cached-123" {
		t.Errorf("expected cached ID cached-123, got %s", match.ID)
	}
	if match.Grades.PSA10Cents != 5000 {
		t.Errorf("expected cached PSA10 price 5000, got %d", match.Grades.PSA10Cents)
	}
}

// TestHttpGetJSON removed - HTTP client functionality is now tested in httpx.Client tests

func TestPriceCharting_CacheExpiration(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cachePath})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Add data with very short TTL
	cachedMatch := &PCMatch{
		ID:          "temp-123",
		ProductName: "Temporary Product",
		PSA10Cents:  1000,
	}

	card := domainCards.Card{Name: "Temp Card", Number: "001", Set: "temp-set", SetName: "Temp Set"}
	key := cache.PriceChartingKey("Temp Set", card.Name, card.Number)
	err = testCache.Set(context.Background(), key, cachedMatch, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to cache data: %v", err)
	}

	// Wait for expiration
	time.Sleep(5 * time.Millisecond)

	// Verify cache miss after expiration
	var retrievedMatch PCMatch
	found, _ := testCache.Get(context.Background(), key, &retrievedMatch)
	if found {
		t.Errorf("expected cache miss after expiration")
	}
}

func TestPriceCharting_CacheKey(t *testing.T) {
	// Test that cache keys are constructed correctly
	setName := "Surging Sparks"
	cardName := "Pikachu"
	number := "025"

	expectedKey := cache.PriceChartingKey(setName, cardName, number)
	if expectedKey == "" {
		t.Errorf("expected non-empty cache key")
	}

	// Verify key format
	if !strings.Contains(expectedKey, setName) {
		t.Errorf("expected cache key to contain set name")
	}
	if !strings.Contains(expectedKey, cardName) {
		t.Errorf("expected cache key to contain card name")
	}
	if !strings.Contains(expectedKey, number) {
		t.Errorf("expected cache key to contain card number")
	}
}

// Benchmark tests
func BenchmarkParseAPIResponseWithLogger(b *testing.B) {
	data := map[string]interface{}{
		"id":                "12345",
		"product-name":      "Pokemon Surging Sparks Pikachu #238",
		"loose-price":       850,
		"graded-price":      1500,
		"box-only-price":    1800,
		"manual-only-price": 2500,
		"bgs-10-price":      3000,
	}

	jsonBytes, _ := json.Marshal(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseAPIResponseWithLogger(jsonBytes, nil)
	}
}

// Test the query formatting in LookupCard
func TestPriceCharting_QueryFormatting(t *testing.T) {
	tests := []struct {
		name     string
		setName  string
		cardName string
		number   string
		expected string
	}{
		{
			name:     "basic card",
			setName:  "Surging Sparks",
			cardName: "Pikachu",
			number:   "238",
			expected: "pokemon Surging Sparks Pikachu #238",
		},
		{
			name:     "special characters",
			setName:  "Sword & Shield",
			cardName: "Charizard-GX",
			number:   "150",
			expected: "pokemon Sword & Shield Charizard-GX #150", // -GX with hyphen not in suffix list
		},
		{
			name:     "with spaces",
			setName:  "Base Set",
			cardName: "Dark Charizard",
			number:   "4",
			expected: "pokemon Base Set Dark Charizard #4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc, _ := NewPriceCharting(DefaultConfig("test-token"), nil, nil)
			query := pc.OptimizeQuery(tt.setName, tt.cardName, tt.number)

			if query != tt.expected {
				t.Errorf("expected query %q, got %q", tt.expected, query)
			}
		})
	}
}

// TestLookupBatch tests batch lookup functionality
// TestQueryOptimization tests the query optimization functionality
func TestQueryOptimization(t *testing.T) {
	tests := []struct {
		name          string
		setName       string
		cardName      string
		number        string
		expectedQuery string
	}{
		{
			name:          "basic card",
			setName:       "Surging Sparks",
			cardName:      "Pikachu",
			number:        "025",
			expectedQuery: "pokemon Surging Sparks Pikachu #025",
		},
		{
			name:          "card with ex suffix",
			setName:       "Surging Sparks",
			cardName:      "Pikachu ex",
			number:        "025",
			expectedQuery: "pokemon Surging Sparks Pikachu #025",
		},
		{
			name:          "card with vmax suffix",
			setName:       "Surging Sparks",
			cardName:      "Charizard VMAX",
			number:        "006",
			expectedQuery: "pokemon Surging Sparks Charizard #006",
		},
		{
			name:          "reverse holo variant",
			setName:       "Surging Sparks",
			cardName:      "Pikachu Reverse Holo",
			number:        "025",
			expectedQuery: "pokemon Surging Sparks Pikachu Reverse #025 reverse holo",
		},
		{
			name:          "set with colon",
			setName:       "Sword & Shield: Base Set",
			cardName:      "Zacian",
			number:        "138",
			expectedQuery: "pokemon Sword & Shield Base Set Zacian #138",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc, _ := NewPriceCharting(DefaultConfig("test-token"), nil, nil)
			query := pc.OptimizeQuery(tt.setName, tt.cardName, tt.number)

			if query != tt.expectedQuery {
				t.Errorf("expected query %q, got %q", tt.expectedQuery, query)
			}
		})
	}
}

// TestCachePriority tests cache priority calculation
func TestCachePriority(t *testing.T) {
	tests := []struct {
		name             string
		match            *PCMatch
		expectedPriority int
		expectedVolatile bool
	}{
		{
			name: "high value card",
			match: &PCMatch{
				PSA10Cents: 15000, // $150
			},
			expectedPriority: 3,
			expectedVolatile: true,
		},
		{
			name: "actively traded card",
			match: &PCMatch{
				PSA10Cents: 5000,
				RecentSales: []SaleData{
					{}, {}, {}, {}, {}, {}, // 6 sales
				},
			},
			expectedPriority: 2,
			expectedVolatile: false,
		},
		{
			name: "low value stable card",
			match: &PCMatch{
				PSA10Cents:  500,
				RecentSales: []SaleData{{}, {}},
			},
			expectedPriority: 1,
			expectedVolatile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc, _ := NewPriceCharting(DefaultConfig("test-token"), nil, nil)
			result := pc.cacheManager.priorityStrategy.CalculatePriority(tt.match.PSA10Cents, tt.match.BGS10Cents, len(tt.match.RecentSales))

			if result.Priority != tt.expectedPriority {
				t.Errorf("expected priority %d, got %d", tt.expectedPriority, result.Priority)
			}

			if result.Volatile != tt.expectedVolatile {
				t.Errorf("expected volatile %v, got %v", tt.expectedVolatile, result.Volatile)
			}
		})
	}
}

// TestGetStats tests API statistics tracking
func TestGetStats(t *testing.T) {
	pc, _ := NewPriceCharting(DefaultConfig("test-token"), nil, nil)

	// Simulate some requests
	pc.incrementRequestCount()
	pc.incrementRequestCount()
	pc.incrementCachedRequests()
	pc.incrementCachedRequests()
	pc.incrementCachedRequests()

	ctx := context.Background()
	stats := pc.GetStats(ctx)

	if stats.APIRequests != int64(2) {
		t.Errorf("expected 2 API requests, got %v", stats.APIRequests)
	}

	if stats.CachedRequests != int64(3) {
		t.Errorf("expected 3 cached requests, got %v", stats.CachedRequests)
	}

	if stats.TotalRequests != int64(5) {
		t.Errorf("expected 5 total requests, got %v", stats.TotalRequests)
	}

	// Check cache hit rate
	if !strings.Contains(stats.CacheHitRate, "60.00%") {
		t.Errorf("expected 60%% cache hit rate, got %v", stats.CacheHitRate)
	}
}
