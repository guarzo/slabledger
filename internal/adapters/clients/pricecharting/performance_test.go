package pricecharting

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/platform/cache"
)

// isRaceEnabled detects if the race detector is enabled
func isRaceEnabled() bool {
	return raceEnabled
}

// Performance baselines and regression thresholds
const (
	// Cache performance thresholds
	maxCacheHitLatency   = 5 * time.Millisecond   // Cache hits should be <5ms
	maxQueryOptimization = 2000 * time.Nanosecond // Query optimization <2μs (realistic threshold)
)

// TestCacheHitPerformance tests cache hit performance with regression detection
func TestCacheHitPerformance(t *testing.T) {
	// Setup cache with pre-populated data
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "perf_cache.json")
	testCache, err := cache.New(cache.Config{Type: "file", FilePath: cacheFile})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Pre-populate cache
	key := cache.PriceChartingKey("Surging Sparks", "Pikachu", "025")
	err = testCache.Set(context.Background(), key, &PCMatch{
		ID:          "12345",
		ProductName: "Pokemon Surging Sparks Pikachu #025",
		LooseCents:  850,
		PSA10Cents:  2500,
	}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to set cache: %v", err)
	}

	pc, err := NewPriceCharting(DefaultConfig("test-token"), testCache, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}

	card := domainCards.Card{Name: "Pikachu", Number: "025", Set: "surging-sparks", SetName: "Surging Sparks"}

	ctx := context.Background()
	// Warm up cache
	_, _ = pc.LookupCard(ctx, "Surging Sparks", card)

	// Measure cache hit performance
	const iterations = 100
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, err := pc.LookupCard(ctx, "Surging Sparks", card)
		if err != nil {
			t.Fatalf("Lookup failed: %v", err)
		}
	}
	elapsed := time.Since(start)
	avgLatency := elapsed / iterations

	t.Logf("Cache hit latency: %v avg over %d lookups", avgLatency, iterations)

	if avgLatency > maxCacheHitLatency {
		t.Errorf("Performance regression: cache hit latency %v exceeds threshold %v",
			avgLatency, maxCacheHitLatency)
	} else {
		t.Logf("✓ Cache performance OK: %v (threshold: %v, headroom: %.1f%%)",
			avgLatency, maxCacheHitLatency,
			float64(maxCacheHitLatency-avgLatency)/float64(maxCacheHitLatency)*100)
	}
}

// TestMemoryUsageScaling tests memory scaling with dataset size
// TestQueryOptimizationPerformance tests query string optimization speed
func TestQueryOptimizationPerformance(t *testing.T) {
	// Skip this test when race detector is enabled - it adds too much overhead
	// Race detector can slow down execution 2-20x
	if isRaceEnabled() {
		t.Skip("Skipping performance test with race detector enabled")
	}

	pc, _ := NewPriceCharting(DefaultConfig("test-token"), nil, nil)

	testCases := []struct {
		setName  string
		cardName string
		number   string
	}{
		{"Surging Sparks", "Pikachu", "025"},
		{"Surging Sparks", "Pikachu ex", "025"},
		{"Surging Sparks", "Charizard VMAX", "006"},
		{"Sword & Shield: Base Set", "Zacian V", "138"},
		{"Pokemon GO", "Mewtwo VSTAR", "079"},
	}

	const iterations = 1000
	start := time.Now()
	for i := 0; i < iterations; i++ {
		for _, tc := range testCases {
			_ = pc.OptimizeQuery(tc.setName, tc.cardName, tc.number)
		}
	}
	elapsed := time.Since(start)
	avgPerOp := elapsed / (iterations * time.Duration(len(testCases)))

	t.Logf("Query optimization: %v avg per operation (%d ops)", avgPerOp, iterations*len(testCases))

	if avgPerOp > maxQueryOptimization {
		t.Errorf("Query optimization regression: %v exceeds threshold %v",
			avgPerOp, maxQueryOptimization)
	} else {
		t.Logf("✓ Query optimization OK: %v (threshold: %v)", avgPerOp, maxQueryOptimization)
	}
}

// TestConcurrentCacheAccess tests cache performance under concurrent load
func TestConcurrentCacheAccess(t *testing.T) {
	// Setup cache
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "concurrent_cache.json")
	testCache, _ := cache.New(cache.Config{Type: "file", FilePath: cacheFile})

	// Pre-populate cache
	setName := "Concurrent Set"
	const numCards = 100
	for i := 0; i < numCards; i++ {
		key := cache.PriceChartingKey(setName, fmt.Sprintf("Card%d", i), fmt.Sprintf("%03d", i))
		_ = testCache.Set(context.Background(), key, &PCMatch{
			ID:          fmt.Sprintf("card-%d", i),
			ProductName: fmt.Sprintf("Card%d #%03d", i, i),
			LooseCents:  100,
			PSA10Cents:  1000,
		}, 1*time.Hour)
	}

	pc, _ := NewPriceCharting(DefaultConfig("test-token"), testCache, nil)

	// Concurrent access test
	const numGoroutines = 20
	const lookupsPerGoroutine = 50

	start := time.Now()
	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			for i := 0; i < lookupsPerGoroutine; i++ {
				cardIdx := (id*lookupsPerGoroutine + i) % numCards
				card := domainCards.Card{
					Name:    fmt.Sprintf("Card%d", cardIdx),
					Number:  fmt.Sprintf("%03d", cardIdx),
					Set:     setName,
					SetName: setName,
				}
				_, _ = pc.LookupCard(context.Background(), setName, card)
			}
			done <- true
		}(g)
	}

	// Wait for completion
	for g := 0; g < numGoroutines; g++ {
		<-done
	}
	elapsed := time.Since(start)

	totalOps := numGoroutines * lookupsPerGoroutine
	avgTimePerOp := elapsed / time.Duration(totalOps)

	t.Logf("Concurrent cache access: %d ops across %d goroutines in %v",
		totalOps, numGoroutines, elapsed)
	t.Logf("Average time per operation: %v", avgTimePerOp)

	// Should complete in reasonable time
	maxConcurrentTime := 5 * time.Second
	if elapsed > maxConcurrentTime {
		t.Errorf("Concurrent access took %v, expected <%v", elapsed, maxConcurrentTime)
	} else {
		t.Log("✓ Concurrent access completed within threshold")
	}
}

// BenchmarkCacheHitWithThreshold benchmarks cache hits with regression detection
func BenchmarkCacheHitWithThreshold(b *testing.B) {
	// Setup cache
	cacheDir := b.TempDir()
	cacheFile := filepath.Join(cacheDir, "bench_cache.json")
	testCache, _ := cache.New(cache.Config{Type: "file", FilePath: cacheFile})

	// Pre-populate
	key := cache.PriceChartingKey("Surging Sparks", "Pikachu", "025")
	_ = testCache.Set(context.Background(), key, &PCMatch{
		ID:          "12345",
		ProductName: "Pokemon Surging Sparks Pikachu #025",
		LooseCents:  850,
		PSA10Cents:  2500,
	}, 1*time.Hour)

	pc, _ := NewPriceCharting(DefaultConfig("test-token"), testCache, nil)
	card := domainCards.Card{Name: "Pikachu", Number: "025", Set: "surging-sparks", SetName: "Surging Sparks"}
	ctx := context.Background()

	b.ResetTimer()

	var totalDuration time.Duration
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, _ = pc.LookupCard(ctx, "Surging Sparks", card)
		totalDuration += time.Since(start)
	}

	avgDuration := totalDuration / time.Duration(b.N)

	b.ReportMetric(float64(avgDuration.Microseconds()), "μs/op")
	b.ReportMetric(float64(maxCacheHitLatency.Microseconds()), "threshold_μs")

	if avgDuration > maxCacheHitLatency {
		b.Logf("⚠ Performance regression: %v > %v", avgDuration, maxCacheHitLatency)
	} else {
		b.Logf("✓ Performance OK: %v (threshold: %v)", avgDuration, maxCacheHitLatency)
	}
}
