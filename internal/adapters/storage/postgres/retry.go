package postgres

import (
	"context"
	"time"
)

// retryConfig controls the exponential-backoff loop used by postgres.Open for
// its initial ping. Sleep is injectable for tests.
type retryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Sleep        func(time.Duration)
}

// retryWithBackoff calls fn up to cfg.MaxAttempts times, doubling the delay
// between attempts and capping at cfg.MaxDelay. It returns nil on the first
// successful call, or the last error after exhaustion. Honors ctx cancellation
// between attempts.
func retryWithBackoff(ctx context.Context, cfg retryConfig, fn func(context.Context) error) error {
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	delay := cfg.InitialDelay
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}
		if err := fn(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == cfg.MaxAttempts {
			break
		}
		sleep(delay)
		delay *= 2
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}
	return lastErr
}
