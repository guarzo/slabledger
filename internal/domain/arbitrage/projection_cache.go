package arbitrage

import (
	"sync"
	"time"
)

// projectionCacheKey identifies a unique projection computation.
// purchaseCount + soldCount together ensure the cache is invalidated when purchases are
// added/removed OR when a sale is recorded (soldCount changes).
type projectionCacheKey struct {
	campaignID    string
	purchaseCount int
	soldCount     int
}

type projectionCacheEntry struct {
	result    *MonteCarloComparison
	expiresAt time.Time
}

// projectionCache is a simple in-memory TTL cache for Monte Carlo projection results.
// Safe for concurrent use.
type projectionCache struct {
	mu      sync.Mutex
	entries map[projectionCacheKey]projectionCacheEntry
	ttl     time.Duration
}

func newProjectionCache(ttl time.Duration) *projectionCache {
	return &projectionCache{
		entries: make(map[projectionCacheKey]projectionCacheEntry),
		ttl:     ttl,
	}
}

func (c *projectionCache) get(key projectionCacheKey) (*MonteCarloComparison, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.result, true
}

func (c *projectionCache) set(key projectionCacheKey, result *MonteCarloComparison) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = projectionCacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}
