package arbitrage

import (
	"testing"
	"time"
)

func TestProjectionCache_HitAndMiss(t *testing.T) {
	c := newProjectionCache(100 * time.Millisecond)

	key := projectionCacheKey{campaignID: "camp-1", purchaseCount: 5, soldCount: 2}
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
	if got == nil {
		t.Error("expected non-nil result on cache hit")
	}

	// TTL expiry — sleep well past the TTL to avoid flakiness under load
	time.Sleep(200 * time.Millisecond)
	if _, ok := c.get(key); ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestProjectionCache_DifferentKeysDontCollide(t *testing.T) {
	c := newProjectionCache(5 * time.Minute)

	key1 := projectionCacheKey{campaignID: "camp-1", purchaseCount: 5, soldCount: 2}
	c.set(key1, &MonteCarloComparison{})

	tests := []struct {
		name    string
		key     projectionCacheKey
		wantHit bool
	}{
		{"different campaign", projectionCacheKey{campaignID: "camp-2", purchaseCount: 5, soldCount: 2}, false},
		{"different purchase count", projectionCacheKey{campaignID: "camp-1", purchaseCount: 6, soldCount: 2}, false},
		{"new sale recorded (soldCount differs)", projectionCacheKey{campaignID: "camp-1", purchaseCount: 5, soldCount: 3}, false},
		{"exact match", key1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := c.get(tt.key)
			if ok != tt.wantHit {
				t.Errorf("cache hit = %v, want %v", ok, tt.wantHit)
			}
		})
	}
}
