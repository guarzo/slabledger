package arbitrage

import (
	"testing"
	"time"
)

func TestProjectionCache_HitAndMiss(t *testing.T) {
	c := newProjectionCache(50 * time.Millisecond)

	key := projectionCacheKey{campaignID: "camp-1", purchaseCount: 5}
	result := &MonteCarloComparison{}

	// Cache miss
	if _, ok := c.get(key); ok {
		t.Error("expected cache miss on empty cache")
	}

	// Store and hit
	c.set(key, result)
	got, ok := c.get(key)
	if !ok {
		t.Error("expected cache hit after set")
	}
	if got != result {
		t.Errorf("got different pointer, want same pointer")
	}

	// TTL expiry
	time.Sleep(60 * time.Millisecond)
	if _, ok := c.get(key); ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestProjectionCache_DifferentKeysDontCollide(t *testing.T) {
	c := newProjectionCache(5 * time.Minute)

	key1 := projectionCacheKey{campaignID: "camp-1", purchaseCount: 5}
	key2 := projectionCacheKey{campaignID: "camp-2", purchaseCount: 5}
	key3 := projectionCacheKey{campaignID: "camp-1", purchaseCount: 6}

	c.set(key1, &MonteCarloComparison{})

	if _, ok := c.get(key2); ok {
		t.Error("key2 should be a miss — different campaign")
	}
	if _, ok := c.get(key3); ok {
		t.Error("key3 should be a miss — different purchase count")
	}
	if _, ok := c.get(key1); !ok {
		t.Error("key1 should still be a hit")
	}
}
