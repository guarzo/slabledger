package pricecharting

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/platform/cache"
)

// BenchmarkLookupCard benchmarks single card lookup
func BenchmarkLookupCard(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockSingleProductResponse)
	}))
	defer server.Close()

	// Setup cache
	cacheDir := b.TempDir()
	cacheFile := filepath.Join(cacheDir, "bench_cache.json")
	testCache, _ := cache.New(cache.Config{Type: "file", FilePath: cacheFile})

	pc, _ := NewPriceCharting(DefaultConfig("test-token"), testCache, nil)
	card := domainCards.Card{Name: "Pikachu", Number: "025", Set: "surging-sparks", SetName: "Surging Sparks"}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pc.LookupCard(ctx, "Surging Sparks", card)
	}
}

// BenchmarkLookupCardWithCache benchmarks cached lookups
func BenchmarkLookupCardWithCache(b *testing.B) {
	// Setup cache with pre-populated data
	cacheDir := b.TempDir()
	cacheFile := filepath.Join(cacheDir, "bench_cache.json")
	testCache, _ := cache.New(cache.Config{Type: "file", FilePath: cacheFile})

	// Pre-populate cache
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
	for i := 0; i < b.N; i++ {
		_, _ = pc.LookupCard(ctx, "Surging Sparks", card)
	}
}

// BenchmarkQueryOptimization benchmarks query optimization
func BenchmarkQueryOptimization(b *testing.B) {
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			_ = pc.OptimizeQuery(tc.setName, tc.cardName, tc.number)
		}
	}
}

// BenchmarkDeduplication benchmarks query deduplication
func BenchmarkDeduplication(b *testing.B) {
	pc, _ := NewPriceCharting(DefaultConfig("test-token"), nil, nil)

	// Generate queries with duplicates
	queries := []string{
		"pokemon Surging Sparks Pikachu #025",
		"pokemon Surging Sparks Charizard #006",
		"pokemon Surging Sparks Pikachu #025", // Duplicate
		"pokemon Surging Sparks Blastoise #009",
		"pokemon Surging Sparks Charizard #006", // Duplicate
		"pokemon Surging Sparks Venusaur #003",
		"pokemon Surging Sparks Pikachu #025", // Duplicate
	}

	// Pre-populate deduplicator
	for i, q := range queries[:3] {
		pc.cacheManager.queryDedup.Store(q, &PCMatch{
			ID:          fmt.Sprintf("dedup-%d", i),
			ProductName: q,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, q := range queries {
			_ = pc.cacheManager.queryDedup.GetCached(q)
		}
	}
}
