// Package pricecharting provides a PriceCharting API adapter implementing
// the domain pricing.PriceProvider interface.
package pricecharting

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

type rateLimiter interface {
	WaitContext(ctx context.Context) error
	Stop()
}

// Compile-time interface check - verify PriceCharting implements domain PriceProvider
var _ pricing.PriceProvider = (*PriceCharting)(nil)

// Config groups all PriceCharting client settings for dependency injection.
type Config struct {
	// Token is the PriceCharting API token (required for live lookups).
	Token string

	// RateLimitInterval is the minimum duration between API requests.
	// Default: 2s (30 requests per minute).
	RateLimitInterval time.Duration

	// HTTPClientConfig overrides the default httpx configuration.
	// When nil, sensible defaults are used (30s timeout, tolerant circuit breaker).
	HTTPClientConfig *httpx.Config

	// CircuitBreakerOverrides allows fine-tuning the circuit breaker without
	// replacing the entire HTTPClientConfig.
	CircuitBreakerOverrides *resilience.CircuitBreakerConfig
}

// DefaultConfig returns a Config with sensible production defaults.
func DefaultConfig(token string) Config {
	return Config{
		Token:             token,
		RateLimitInterval: 2 * time.Second,
	}
}

type PriceCharting struct {
	token          string
	cacheManager   *CacheManager
	queryHelper    *QueryHelper
	batchSize      int
	workerPool     int
	rateLimiter    rateLimiter
	requestCount   int64
	cachedRequests int64
	mu             sync.RWMutex
	upcDatabase    *UPCDatabase
	httpClient     *httpx.Client // Unified HTTP client (includes retry + circuit breaker)
	logger         observability.Logger
	hintResolver   pricing.PriceHintResolver
}

// WithHintResolver sets a PriceHintResolver for user-provided price hints.
func WithHintResolver(r pricing.PriceHintResolver) func(*PriceCharting) {
	return func(pc *PriceCharting) {
		pc.hintResolver = r
	}
}

func NewPriceCharting(cfg Config, c cache.Cache, log observability.Logger, opts ...func(*PriceCharting)) (*PriceCharting, error) {
	if log == nil {
		log = observability.NewNoopLogger()
	}

	// Rate limiter - default to 2s between requests (30 req/min)
	interval := cfg.RateLimitInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	rateLimiter := NewTickerRateLimiter(interval)

	// Build httpx config: caller-provided or sensible defaults
	httpCfg := buildHTTPConfig(cfg)
	httpClient := httpx.NewClient(httpCfg)

	pc := &PriceCharting{
		token:        cfg.Token,
		cacheManager: NewCacheManager(c),
		queryHelper:  NewQueryHelper(),
		batchSize:    20, // PriceCharting API batch limit
		workerPool:   5,  // Concurrent workers for batch processing
		rateLimiter:  rateLimiter,
		httpClient:   httpClient,
		logger:       log,
	}

	// Initialize UPC database (in-memory only, no persistence)
	upcDB := NewUPCDatabase()
	upcDB.PopulateCommonMappings() // Load initial mappings
	pc.upcDatabase = upcDB

	// Apply functional options
	for _, opt := range opts {
		opt(pc)
	}

	return pc, nil
}

func (p *PriceCharting) Available() bool {
	return p.token != ""
}

// buildHTTPConfig constructs the httpx.Config from a pricecharting Config.
func buildHTTPConfig(cfg Config) httpx.Config {
	if cfg.HTTPClientConfig != nil {
		httpCfg := *cfg.HTTPClientConfig
		if cfg.CircuitBreakerOverrides != nil {
			applyCBOverrides(&httpCfg.CircuitBreakerConfig, cfg.CircuitBreakerOverrides)
		}
		return httpCfg
	}

	// Sensible defaults for PriceCharting:
	// - Tolerant circuit breaker (primary pricing source)
	// - 30s timeout
	httpCfg := httpx.DefaultConfig("PriceCharting")
	httpCfg.DefaultTimeout = 30 * time.Second
	httpCfg.CircuitBreakerConfig.MinRequests = 20
	httpCfg.CircuitBreakerConfig.FailureRatio = 0.9
	httpCfg.CircuitBreakerConfig.Timeout = 60 * time.Second

	if cfg.CircuitBreakerOverrides != nil {
		applyCBOverrides(&httpCfg.CircuitBreakerConfig, cfg.CircuitBreakerOverrides)
	}
	return httpCfg
}

// applyCBOverrides applies non-zero override values to the circuit breaker config.
func applyCBOverrides(dst *resilience.CircuitBreakerConfig, src *resilience.CircuitBreakerConfig) {
	if src.MaxRequests > 0 {
		dst.MaxRequests = src.MaxRequests
	}
	if src.MinRequests > 0 {
		dst.MinRequests = src.MinRequests
	}
	if src.FailureRatio > 0 {
		dst.FailureRatio = src.FailureRatio
	}
	if src.Timeout > 0 {
		dst.Timeout = src.Timeout
	}
	if src.Interval > 0 {
		dst.Interval = src.Interval
	}
}

// Name implements pricing.PriceProvider interface
func (p *PriceCharting) Name() string {
	return "pricecharting"
}

// GetPrice implements pricing.PriceProvider interface
// Converts domain pricing.Card to domain cards.Card and uses LookupCard
// Returns all grade prices (PSA 9, PSA 10, Grade 9.5, BGS 10, Raw) for risk-adjusted scoring
func (p *PriceCharting) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
	// Convert domain pricing.Card to domain cards.Card
	domainCard := domainCards.Card{
		Name:    card.Name,
		Number:  card.Number,
		SetName: card.Set,
	}

	// Perform lookup using existing LookupCard implementation
	// LookupCard now returns *pricing.Price directly
	price, err := p.LookupCard(ctx, card.Set, domainCard)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) {
			return nil, appErr
		}
		return nil, apperrors.ProviderUnavailable("PriceCharting", err)
	}

	if price == nil {
		return nil, apperrors.ProviderNotFound("PriceCharting", fmt.Sprintf("%s #%s", card.Name, card.Number))
	}

	return price, nil
}

// Close stops the rate limiter and releases resources
func (p *PriceCharting) Close() error {
	if p.rateLimiter != nil {
		p.rateLimiter.Stop()
	}
	return nil
}
