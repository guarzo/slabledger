package pricecharting

import (
	"context"
	"time"
)

// TickerRateLimiter implements RateLimiter using a time.Ticker.
// Unlike golang.org/x/time/rate (token-bucket with bursting), a ticker enforces
// a strict minimum interval between requests. PriceCharting's API requires evenly
// spaced calls (no bursting), which makes the ticker semantics a better fit than
// a token-bucket limiter. Other clients (pokemonprice, cardhedger) use
// golang.org/x/time/rate because their APIs tolerate short bursts.
type TickerRateLimiter struct {
	ticker *time.Ticker
}

// NewTickerRateLimiter creates a new ticker-based rate limiter
func NewTickerRateLimiter(interval time.Duration) *TickerRateLimiter {
	return &TickerRateLimiter{
		ticker: time.NewTicker(interval),
	}
}

// WaitContext blocks until the rate limiter allows the next operation or context is cancelled
func (r *TickerRateLimiter) WaitContext(ctx context.Context) error {
	if r.ticker == nil {
		return nil
	}

	select {
	case <-r.ticker.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop stops the rate limiter and releases resources
func (r *TickerRateLimiter) Stop() {
	if r.ticker != nil {
		r.ticker.Stop()
	}
}
