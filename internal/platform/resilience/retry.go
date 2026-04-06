package resilience

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/sony/gobreaker"
)

// RetryPolicy defines retry behavior configuration
type RetryPolicy struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
}

// DefaultRetryPolicy returns sensible defaults for retry behavior
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
	}
}

// CircuitBreakerConfig defines circuit breaker configuration
type CircuitBreakerConfig struct {
	Name         string
	MaxRequests  uint32        // Max requests allowed when half-open
	Interval     time.Duration // Interval to reset failure counts
	Timeout      time.Duration // Time to wait before attempting to close
	MinRequests  uint32        // Minimum requests before trip check
	FailureRatio float64       // Ratio of failures to trip the breaker
}

// DefaultCircuitBreakerConfig returns sensible defaults for circuit breaker
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:         name,
		MaxRequests:  5,                 // Allow more requests in half-open state
		Interval:     60 * time.Second,  // Reset failure counts every minute
		Timeout:      120 * time.Second, // Wait 2 minutes before trying again
		MinRequests:  10,                // Need at least 10 requests before considering trip
		FailureRatio: 0.8,               // Only trip if 80% of requests fail
	}
}

// NewCircuitBreaker creates a circuit breaker with the given configuration
func NewCircuitBreaker(config CircuitBreakerConfig, logger observability.Logger) *gobreaker.CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        config.Name,
		MaxRequests: config.MaxRequests,
		Interval:    config.Interval,
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Only trip if we have enough requests and failure ratio is high
			return counts.Requests >= config.MinRequests &&
				float64(counts.TotalFailures)/float64(counts.Requests) >= config.FailureRatio
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Warn(context.Background(), "Circuit breaker state changed",
				observability.String("circuit_breaker", name),
				observability.String("from_state", from.String()),
				observability.String("to_state", to.String()))
		},
		IsSuccessful: isCircuitBreakerSuccess,
	}

	return gobreaker.NewCircuitBreaker(settings)
}

// isCircuitBreakerSuccess determines if an error should be counted as a success
// for circuit breaker purposes. Business logic errors (like "not found") should
// NOT trip the circuit breaker - only infrastructure errors should.
func isCircuitBreakerSuccess(err error) bool {
	if err == nil {
		return true
	}

	// Check for AppError types that represent business logic (not infrastructure) failures
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case apperrors.ErrCodeProviderNotFound:
			// 404 Not Found is a business logic result, not an infrastructure failure
			// The API responded successfully, just the resource doesn't exist
			return true
		case apperrors.ErrCodeProviderInvalidReq:
			// 400 Bad Request means our request was wrong, not that the service is failing
			return true
		}
		// All other AppError types (unavailable, timeout, rate limit, auth) are real failures
		return false
	}

	// Check error message patterns for business logic errors
	// Note: 404 "not found" responses are handled above via ErrCodeProviderNotFound
	errStr := strings.ToLower(err.Error())

	// These are business logic errors, not infrastructure failures:
	// - "no match found" - Card not in provider database
	// - "no product match" - Product search returned no results
	// - "query too short" - Search query validation failed
	if strings.Contains(errStr, "no match found") ||
		strings.Contains(errStr, "no product match") ||
		strings.Contains(errStr, "query too short") {
		return true
	}

	// Default: treat unknown errors as failures (safer for circuit breaker)
	return false
}

// logError adds structured error fields to a log
// Extracts fields from AppError for better log readability
func logError(logger observability.Logger, ctx context.Context, msg string, level slog.Level, err error) {
	if err == nil {
		return
	}

	// Build base fields
	fields := []observability.Field{observability.Err(err)}

	// Try to extract AppError for structured logging
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		fields = append(fields,
			observability.String("error_code", string(appErr.Code)),
			observability.String("error_message", appErr.Message))

		// Add HTTP status if available
		if httpStatus := appErr.HTTPStatus(); httpStatus > 0 {
			fields = append(fields, observability.Int("http_status", httpStatus))
		}

		// Add context fields if present
		if len(appErr.Context) > 0 {
			for key, value := range appErr.Context {
				// Convert to string for simplicity
				fields = append(fields, observability.Field{Key: key, Value: value})
			}
		}
	}

	// Log at appropriate level
	switch level {
	case slog.LevelDebug:
		logger.Debug(ctx, msg, fields...)
	case slog.LevelWarn:
		logger.Warn(ctx, msg, fields...)
	case slog.LevelError:
		logger.Error(ctx, msg, fields...)
	default:
		logger.Info(ctx, msg, fields...)
	}
}

// isRetryableError determines if an error should trigger a retry.
// It first checks for structured AppError codes via HasErrorCode, then
// falls back to string matching for non-AppError cases.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check structured error codes first (preferred over string matching)
	// Circuit breaker errors should NOT be retried - they're protecting the service
	if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderCircuitOpen) {
		return false
	}

	// Rate limit errors should NOT be retried - retrying makes it worse
	if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit) {
		return false
	}

	// Provider timeout errors are retryable
	if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderTimeout) {
		return true
	}

	// Provider unavailable errors are retryable
	if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderUnavailable) {
		return true
	}

	// Network errors are retryable
	if apperrors.HasErrorCode(err, apperrors.ErrCodeNetworkTimeout) ||
		apperrors.HasErrorCode(err, apperrors.ErrCodeNetworkUnavailable) {
		return true
	}

	// Fall back to string matching for non-AppError cases
	errStr := strings.ToLower(err.Error())

	// Explicitly guard HTTP 429 to avoid accidental retries via generic checks
	// (e.g., a 429 message containing "temporary" would otherwise match below)
	if strings.Contains(errStr, "429") {
		return false
	}

	// Network-level errors that should be retried
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "eof") ||
		strings.Contains(errStr, "broken pipe") {
		return true
	}

	// HTTP status codes that should be retried
	if strings.Contains(errStr, "503") || // Service unavailable
		strings.Contains(errStr, "504") || // Gateway timeout
		strings.Contains(errStr, "502") { // Bad gateway
		return true
	}

	return false
}

// RetryWithBackoff executes a function with retry and exponential backoff
func RetryWithBackoff(ctx context.Context, logger observability.Logger, policy RetryPolicy, operation func() error) error {
	var lastErr error
	backoff := policy.InitialBackoff

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		// Wait before retry (skip for first attempt)
		if attempt > 0 {
			logger.Info(ctx, "Retrying operation",
				observability.Int("attempt", attempt+1),
				observability.Int("max_retries", policy.MaxRetries),
				observability.Duration("backoff", backoff))

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
			}

			// Exponential backoff with jitter to avoid thundering herd
			backoff = time.Duration(float64(backoff) * policy.BackoffFactor)
			if backoff > policy.MaxBackoff {
				backoff = policy.MaxBackoff
			}
			// Apply jitter: multiply by random factor in [0.5, 1.0)
			// This decorrelates retry storms while keeping delays within MaxBackoff
			backoff = time.Duration(float64(backoff) * (0.5 + rand.Float64()*0.5))
		}

		// Execute operation
		err := operation()
		if err == nil {
			if attempt > 0 {
				logger.Info(ctx, "Operation succeeded after retry",
					observability.Int("attempt", attempt+1))
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			logError(logger, ctx, "Non-retryable error encountered", slog.LevelDebug, err)
			return fmt.Errorf("non-retryable error: %w", err)
		}

		if attempt < policy.MaxRetries {
			logError(logger, ctx, "Retryable error encountered", slog.LevelWarn, err)
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", policy.MaxRetries, lastErr)
}

// CircuitBreakerStats represents circuit breaker statistics
type CircuitBreakerStats struct {
	Name                 string
	State                string
	Requests             uint32
	Successes            uint32
	Failures             uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// GetCircuitBreakerStats returns statistics from a circuit breaker
func GetCircuitBreakerStats(cb *gobreaker.CircuitBreaker) CircuitBreakerStats {
	state := cb.State()
	counts := cb.Counts()

	return CircuitBreakerStats{
		Name:                 cb.Name(),
		State:                state.String(),
		Requests:             counts.Requests,
		Successes:            counts.TotalSuccesses,
		Failures:             counts.TotalFailures,
		ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
		ConsecutiveFailures:  counts.ConsecutiveFailures,
	}
}
