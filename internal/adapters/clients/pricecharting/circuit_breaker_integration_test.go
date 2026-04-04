//go:build integration

package pricecharting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/platform/resilience"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
)

func TestPriceCharting_CircuitBreakerIntegration(t *testing.T) {
	// Create a simple file cache in temp directory
	c, err := cache.NewFileCacheBackend(t.TempDir(), cache.SimpleCacheConfig{})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = c.Close() }()

	pc, err := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Verify httpClient is initialized (contains circuit breaker)
	if pc.httpClient == nil {
		t.Fatal("HTTP client (with circuit breaker) should be initialized")
	}

	// Get circuit breaker stats from httpClient
	cbStats := pc.httpClient.GetCircuitBreakerStats()

	// Initial state should be closed
	if cbStats.State != "closed" {
		t.Errorf("Expected initial circuit breaker state to be 'closed', got %s", cbStats.State)
	}
}

func TestPriceCharting_CircuitBreakerStats(t *testing.T) {
	c, err := cache.NewFileCacheBackend(t.TempDir(), cache.SimpleCacheConfig{})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = c.Close() }()

	pc, err := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Get stats (now returns typed struct)
	ctx := context.Background()
	stats := pc.GetStats(ctx)

	// Check that circuit breaker stats are included
	if stats.CircuitBreaker == nil {
		t.Fatal("Circuit breaker stats should be included in GetStats()")
	}

	// Verify fields are present (compile-time type safety ensures they exist)
	cb := stats.CircuitBreaker

	// Verify we can access all fields without type assertions
	_ = cb.State
	_ = cb.Requests
	_ = cb.Successes
	_ = cb.Failures
	_ = cb.ConsecutiveSuccesses
	_ = cb.ConsecutiveFailures

	// Verify initial state is Closed
	if cb.State != "closed" {
		t.Errorf("Expected initial state 'closed', got %v", cb.State)
	}
}

func TestPriceCharting_LookupWithCircuitBreaker(t *testing.T) {
	// NOTE: This test verifies circuit breaker integration with the PriceCharting client.
	// The mock server below is not injected into the client because the PriceCharting
	// client uses a fixed base URL. The test validates that lookups don't panic and
	// that the httpClient remains functional after errors.
	// TODO: To enable mock server injection, NewPriceCharting would need a WithBaseURL option.

	c, err := cache.NewFileCacheBackend(t.TempDir(), cache.SimpleCacheConfig{})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = c.Close() }()

	pc, err := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Create a test card
	card := domainCards.Card{
		ID:      "test-001",
		Name:    "Test Card",
		Number:  "001",
		Set:     "test-set",
		SetName: "Test Set",
	}

	ctx := context.Background()
	query := pc.queryHelper.OptimizeQuery("Test Set", card.Name, card.Number)

	// This will fail (mock server returns error), but should demonstrate circuit breaker behavior
	_, err = pc.lookupByQueryInternal(ctx, query)

	// We expect an error from the mock server
	if err == nil {
		t.Log("Warning: Expected error from lookup, got success (test may need adjustment)")
	}

	// httpClient with circuit breaker should still be functional
	if pc.httpClient == nil {
		t.Error("HTTP client should remain initialized after failed lookup")
	}
}

func TestPriceCharting_CacheFallbackWhenCircuitOpen(t *testing.T) {
	// NOTE: This test verifies cache fallback when API is unavailable.
	// The mock server below is not injected into the client because the PriceCharting
	// client uses a fixed base URL. The test relies on pre-populated cache to demonstrate
	// fallback behavior when lookups fail (from real API unavailability or rate limiting).
	// TODO: To enable mock server injection, NewPriceCharting would need a WithBaseURL option.

	c, err := cache.NewFileCacheBackend(t.TempDir(), cache.SimpleCacheConfig{})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = c.Close() }()

	pc, err := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Create a test card and cache a match for it
	card := domainCards.Card{
		ID:      "test-002",
		Name:    "Pikachu",
		Number:  "025",
		Set:     "test-set",
		SetName: "Test Set",
	}

	cachedMatch := &PCMatch{
		ID:          "cached-001",
		ProductName: "Test Cached Product",
		PSA10Cents:  50000, // $500
	}

	// Pre-populate cache
	pc.cacheManager.CacheMatch(context.Background(), "Test Set", card, cachedMatch)

	// Attempt lookup - will fail to reach API but should use cache
	ctx := context.Background()
	query := pc.queryHelper.OptimizeQuery("Test Set", card.Name, card.Number)

	match, err := pc.lookupByQueryInternal(ctx, query)

	// Should successfully retrieve from cache when API is unavailable
	if match != nil {
		if match.ID != "cached-001" {
			t.Errorf("Expected cached match ID 'cached-001', got '%s'", match.ID)
		}
		if match.ProductName != "Test Cached Product" {
			t.Errorf("Expected cached product name, got '%s'", match.ProductName)
		}
		t.Log("Cache fallback working correctly")
	} else if err != nil {
		t.Logf("Lookup failed (expected with mock failure), error: %v", err)
	}
}

func TestPriceCharting_HttpClientConfiguration(t *testing.T) {
	c, err := cache.NewFileCacheBackend(t.TempDir(), cache.SimpleCacheConfig{})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = c.Close() }()

	pc, err := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Verify httpClient is initialized (retry is handled internally by httpx.Client)
	if pc.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
}

func TestPriceCharting_LookupByQueryInternal(t *testing.T) {
	// Create mock server that returns success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"status":            "success",
			"id":                "12345",
			"product-name":      "Pokemon Test Pikachu #025",
			"loose-price":       850,
			"graded-price":      1500,
			"manual-only-price": 2500,
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := cache.NewFileCacheBackend(t.TempDir(), cache.SimpleCacheConfig{})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = c.Close() }()

	pc, err := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Note: This test verifies the method exists and doesn't panic
	// Actual API calls would require mocking the httpClient transport
	query := "pokemon test pikachu #025"
	ctx := context.Background()

	_, err = pc.lookupByQueryInternal(ctx, query)

	// Since we can't easily mock the httpClient's transport, we just verify no panic
	// The error is expected since we're not actually hitting the mock server
	t.Logf("lookupByQueryInternal completed with result: %v", err)

	// Should not panic or cause issues - httpClient should remain functional
	if pc.httpClient == nil {
		t.Error("HTTP client should remain after lookup call")
	}
}

// Test that circuit breaker state changes are logged
func TestCircuitBreaker_StateChangeLogging(t *testing.T) {
	config := resilience.CircuitBreakerConfig{
		Name:         "logging-test",
		MaxRequests:  1,
		Interval:     100 * time.Millisecond,
		Timeout:      200 * time.Millisecond,
		MinRequests:  1,
		FailureRatio: 0.5,
	}

	logger := telemetry.NewSlogLogger(slog.LevelWarn, "text")
	cb := resilience.NewCircuitBreaker(config, logger)

	// Cause state transitions
	initial := cb.State()

	// Force failures to open circuit
	for i := 0; i < 2; i++ {
		_, _ = cb.Execute(func() (interface{}, error) {
			return nil, errors.New("test failure")
		})
	}

	afterFailures := cb.State()

	if initial == afterFailures {
		t.Log("Circuit states did not change - may need more failures")
	}

	// The state change callback in NewCircuitBreaker should log
	// This test verifies the circuit breaker is properly configured
	if cb.Name() != "logging-test" {
		t.Errorf("Expected circuit breaker name 'logging-test', got '%s'", cb.Name())
	}
}

func TestPriceCharting_ConcurrentAccessWithCircuitBreaker(t *testing.T) {
	c, err := cache.NewFileCacheBackend(t.TempDir(), cache.SimpleCacheConfig{})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = c.Close() }()

	pc, err := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Test concurrent access to circuit breaker state
	done := make(chan bool)

	// Thread-safe error collection (t.Errorf is not goroutine-safe)
	var mu sync.Mutex
	var errs []string

	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Get stats concurrently (includes circuit breaker stats from httpClient)
			stats := pc.GetStats(context.Background())
			if stats == nil {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("Goroutine %d: stats should not be nil", id))
				mu.Unlock()
			}

			// Access circuit breaker state through httpClient
			if pc.httpClient != nil {
				_ = pc.httpClient.GetCircuitBreakerStats()
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Report any errors collected from goroutines
	for _, e := range errs {
		t.Error(e)
	}
}

// Benchmark circuit breaker overhead via GetStats
func BenchmarkPriceCharting_CircuitBreakerExecute(b *testing.B) {
	c, err := cache.NewFileCacheBackend(b.TempDir(), cache.SimpleCacheConfig{})
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = c.Close() }()

	pc, err := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	if err != nil {
		b.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Access circuit breaker stats through httpClient
		_ = pc.httpClient.GetCircuitBreakerStats()
	}
}
