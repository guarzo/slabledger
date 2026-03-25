package pricecharting

import (
	"context"
	"fmt"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/constants"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/cache"
)

// PriorityStrategy calculates cache TTL/priority for a card.
type PriorityStrategy interface {
	CalculatePriority(psa10Cents, bgs10Cents int, recentSalesCount int) pricing.CachePriorityResult
}

// QueryDedup prevents duplicate queries within a batch.
type QueryDedup interface {
	GetCached(query string) *PCMatch
	Store(query string, match *PCMatch)
	Clear()
}

// CacheManager handles all caching operations for PriceCharting lookups
type CacheManager struct {
	cache            cache.Cache
	queryDedup       QueryDedup
	priorityStrategy PriorityStrategy
}

// CacheManagerOption configures a CacheManager.
type CacheManagerOption func(*CacheManager)

// WithPriorityStrategy overrides the default cache priority strategy.
func WithPriorityStrategy(s PriorityStrategy) CacheManagerOption {
	return func(cm *CacheManager) { cm.priorityStrategy = s }
}

// WithQueryDeduplicator overrides the default query deduplicator.
func WithQueryDeduplicator(d QueryDedup) CacheManagerOption {
	return func(cm *CacheManager) { cm.queryDedup = d }
}

// NewCacheManager creates a new cache manager
func NewCacheManager(c cache.Cache, opts ...CacheManagerOption) *CacheManager {
	cm := &CacheManager{
		cache:      c,
		queryDedup: NewQueryDeduplicator(),
		priorityStrategy: &pricing.CachePriorityStrategy{
			HighValueThresholdCents: constants.HighValueThresholdCents,
			HighValueTTL:            constants.HighValueCacheTTL,
			ActiveTradingTTL:        constants.ActiveTradingCacheTTL,
			StableTTL:               constants.StableCacheTTL,
			ActiveSalesThreshold:    constants.ActiveSalesThreshold,
		},
	}
	for _, opt := range opts {
		opt(cm)
	}
	return cm
}

// RawCache returns the underlying cache instance.
func (cm *CacheManager) RawCache() cache.Cache {
	return cm.cache
}

// GetCachedMatch retrieves a cached match for a card
func (cm *CacheManager) GetCachedMatch(ctx context.Context, setName string, card domainCards.Card) (*PCMatch, bool) {
	if cm.cache == nil {
		return nil, false
	}

	key := cache.PriceChartingKey(setName, card.Name, card.Number)
	var match PCMatch
	found, err := cm.cache.Get(ctx, key, &match)
	if err != nil {
		return nil, false
	}
	if !found {
		return nil, false
	}
	return &match, true
}

// GetCachedMatchByQuery retrieves a cached match by query string
func (cm *CacheManager) GetCachedMatchByQuery(query string) (*PCMatch, bool) {
	if cachedMatch := cm.queryDedup.GetCached(query); cachedMatch != nil {
		return cachedMatch, true
	}
	return nil, false
}

// GetCachedUPCMatch retrieves a cached match by UPC
func (cm *CacheManager) GetCachedUPCMatch(ctx context.Context, upc string) (*PCMatch, bool) {
	if cm.cache == nil {
		return nil, false
	}

	cacheKey := fmt.Sprintf("upc:%s", upc)
	var match PCMatch
	found, err := cm.cache.Get(ctx, cacheKey, &match)
	if err != nil {
		return nil, false
	}
	if found {
		return &match, true
	}
	return nil, false
}

// CacheMatch stores a match in the cache with dynamic TTL
func (cm *CacheManager) CacheMatch(ctx context.Context, setName string, card domainCards.Card, match *PCMatch) {
	if cm.cache == nil || match == nil {
		return
	}

	key := cache.PriceChartingKey(setName, card.Name, card.Number)
	priority := cm.priorityStrategy.CalculatePriority(match.PSA10Cents, match.BGS10Cents, len(match.RecentSales))
	_ = cm.cache.Set(ctx, key, match, priority.TTL) //nolint:errcheck // Cache errors are not critical
}

// CacheMatchByQuery stores a match in the query deduplicator
func (cm *CacheManager) CacheMatchByQuery(query string, match *PCMatch) {
	if match != nil {
		cm.queryDedup.Store(query, match)
	}
}

// CacheUPCMatch stores a UPC-based match
func (cm *CacheManager) CacheUPCMatch(ctx context.Context, upc string, match *PCMatch) {
	if cm.cache == nil || match == nil {
		return
	}

	cacheKey := fmt.Sprintf("upc:%s", upc)
	priority := cm.priorityStrategy.CalculatePriority(match.PSA10Cents, match.BGS10Cents, len(match.RecentSales))
	_ = cm.cache.Set(ctx, cacheKey, match, priority.TTL) //nolint:errcheck // Cache errors are not critical
}
