package pricecharting

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/platform/cache"
)

func TestLookupCardInternal(t *testing.T) {
	// Create a test cache
	c, _ := cache.NewFileCacheBackend(filepath.Join(t.TempDir(), "test_cache.json"), cache.SimpleCacheConfig{
		FlushInterval: 100 * time.Millisecond,
		MaxEntries:    1000,
	})
	defer func() { _ = c.Close() }()

	// Create PriceCharting instance
	pc, err := NewPriceCharting(DefaultConfig("test"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Test card
	testCard := domainCards.Card{
		Name:    "Pikachu",
		Number:  "025",
		Set:     "base-set",
		SetName: "Base Set",
	}
	setName := "Base Set"

	// Pre-populate cache with a test match
	testMatch := &PCMatch{
		ProductName: "Pokemon Base Set Pikachu #025",
	}
	pc.cacheManager.CacheMatch(context.Background(), setName, testCard, testMatch)

	// Test 1: Cache lookup should find the match
	t.Run("CacheLookupFindsMatch", func(t *testing.T) {
		match, found := pc.tryCache(context.Background(), testCard, setName)

		if !found {
			t.Error("Cache lookup should have found the match")
		}
		if match == nil {
			t.Error("Match should not be nil")
		}
		if match != nil && match.ProductName != testMatch.ProductName {
			t.Errorf("Expected product name %s, got %s", testMatch.ProductName, match.ProductName)
		}
	})

	// Test 2: Lookup chain should execute in order (cache first)
	t.Run("LookupChainExecutesInOrder", func(t *testing.T) {
		ctx := context.Background()
		match, err := pc.lookupCardInternal(ctx, setName, testCard)
		if err != nil {
			t.Errorf("Lookup failed: %v", err)
		}
		if match == nil {
			t.Error("Lookup should have found a match")
		}
		if match != nil && match.ProductName != testMatch.ProductName {
			t.Errorf("Expected product name %s, got %s", testMatch.ProductName, match.ProductName)
		}
	})

	// Test 3: UPC lookup skips when no UPC database
	t.Run("UPCLookupSkipsWhenNoDatabase", func(t *testing.T) {
		pc.upcDatabase = nil // Ensure no UPC database

		differentCard := domainCards.Card{
			Name:    "Charizard",
			Number:  "006",
			Set:     "base-set",
			SetName: "Base Set",
		}

		match, found, err := pc.tryUPC(context.Background(), differentCard, setName)

		if err != nil {
			t.Errorf("UPC lookup should not error when no database: %v", err)
		}
		if found {
			t.Error("UPC lookup should return false when no database")
		}
		if match != nil {
			t.Error("Match should be nil when UPC lookup skips")
		}
	})

	// Test 4: LookupCard uses the lookup chain
	t.Run("LookupCardUsesInternalChain", func(t *testing.T) {
		ctx := context.Background()
		match, err := pc.LookupCard(ctx, setName, testCard)
		if err != nil {
			t.Errorf("LookupCard failed: %v", err)
		}
		if match == nil {
			t.Error("LookupCard should have found a match")
		}
		if match != nil && match.ProductName != testMatch.ProductName {
			t.Errorf("Expected product name %s, got %s", testMatch.ProductName, match.ProductName)
		}
	})

}

func TestLookupMethods(t *testing.T) {
	// Create a test cache
	c, _ := cache.NewFileCacheBackend(filepath.Join(t.TempDir(), "test_cache.json"), cache.SimpleCacheConfig{
		FlushInterval: 100 * time.Millisecond,
		MaxEntries:    1000,
	})
	defer func() { _ = c.Close() }()

	// Create PriceCharting instance
	pc, err := NewPriceCharting(DefaultConfig("test"), c, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Test card
	testCard := domainCards.Card{
		Name:    "Pikachu",
		Number:  "025",
		Set:     "base-set",
		SetName: "Base Set",
	}
	setName := "Base Set"

	// Test cache method
	t.Run("TryCacheMethod", func(t *testing.T) {
		// Should not find anything initially
		match, found := pc.tryCache(context.Background(), testCard, setName)
		if found {
			t.Error("Should not find match in empty cache")
		}
		if match != nil {
			t.Error("Match should be nil when not found")
		}

		// Add to cache
		testMatch := &PCMatch{
			ProductName: "Pokemon Base Set Pikachu #025",
		}
		pc.cacheManager.CacheMatch(context.Background(), setName, testCard, testMatch)

		// Should find it now
		match, found = pc.tryCache(context.Background(), testCard, setName)
		if !found {
			t.Error("Should find match in cache")
		}
		if match == nil {
			t.Error("Match should not be nil")
		}
	})

	// Test UPC method
	t.Run("TryUPCMethod", func(t *testing.T) {
		// Should return false when no UPC database
		pc.upcDatabase = nil
		match, found, err := pc.tryUPC(context.Background(), testCard, setName)
		if err != nil {
			t.Errorf("Should not error: %v", err)
		}
		if found {
			t.Error("Should not find match without UPC database")
		}
		if match != nil {
			t.Error("Match should be nil")
		}

		// Re-initialize UPC database
		pc.upcDatabase = NewUPCDatabase()

		// Should return false when no UPC mapping
		match, found, err = pc.tryUPC(context.Background(), testCard, setName)
		if err != nil {
			t.Errorf("Should not error: %v", err)
		}
		if found {
			t.Error("Should not find match without UPC mapping")
		}
		if match != nil {
			t.Error("Match should be nil")
		}
	})

}
