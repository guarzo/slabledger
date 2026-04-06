package resilience

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
)

// Helper function to create a test logger
func testLogger() *telemetry.SlogLogger {
	return telemetry.NewSlogLogger(slog.LevelError, "text")
}

func TestRetryWithBackoff_Success(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	attempts := 0
	operation := func() error {
		attempts++
		return nil
	}

	err := RetryWithBackoff(context.Background(), testLogger(), policy, operation)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryWithBackoff_SuccessAfterRetries(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	attempts := 0
	operation := func() error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("503 service unavailable") // Retryable error
		}
		return nil
	}

	err := RetryWithBackoff(context.Background(), testLogger(), policy, operation)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_MaxRetriesExceeded(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	attempts := 0
	operation := func() error {
		attempts++
		return fmt.Errorf("timeout error") // Always fail with retryable error
	}

	err := RetryWithBackoff(context.Background(), testLogger(), policy, operation)
	if err == nil {
		t.Error("expected error, got nil")
	}

	// Should attempt once + 3 retries = 4 total attempts
	if attempts != 4 {
		t.Errorf("expected 4 attempts (1 + 3 retries), got %d", attempts)
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		// Check that error message indicates retries were exhausted
		if err.Error() == "" {
			t.Error("expected error message")
		}
	}
}

func TestRetryWithBackoff_NonRetryableError(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	attempts := 0
	operation := func() error {
		attempts++
		return fmt.Errorf("404 not found") // Non-retryable error
	}

	err := RetryWithBackoff(context.Background(), testLogger(), policy, operation)
	if err == nil {
		t.Error("expected error, got nil")
	}

	// Should only attempt once for non-retryable errors
	if attempts != 1 {
		t.Errorf("expected 1 attempt for non-retryable error, got %d", attempts)
	}
}

func TestRetryWithBackoff_ContextCancellation(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		BackoffFactor:  2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	operation := func() error {
		attempts++
		if attempts == 2 {
			cancel() // Cancel context on second attempt
		}
		return fmt.Errorf("timeout error") // Retryable error
	}

	err := RetryWithBackoff(ctx, testLogger(), policy, operation)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

func TestRetryWithBackoff_Timing(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		BackoffFactor:  2.0,
	}

	start := time.Now()
	attempts := 0

	operation := func() error {
		attempts++
		return fmt.Errorf("temporary network error") // Retryable error
	}

	_ = RetryWithBackoff(context.Background(), testLogger(), policy, operation)
	elapsed := time.Since(start)

	// Delays with jitter [0.5, 1.0): base delays 100ms, 200ms, 400ms
	// Minimum total: 50ms + 50ms + 50ms = 150ms (worst case: jitter compounds low)
	// Maximum total: 100ms + 200ms + 400ms = 700ms (jitter=1.0)
	expectedMin := 100 * time.Millisecond
	expectedMax := 800 * time.Millisecond // Allow overhead

	if elapsed < expectedMin {
		t.Errorf("elapsed time too short: got %v, want at least %v", elapsed, expectedMin)
	}

	if elapsed > expectedMax {
		t.Errorf("elapsed time too long: got %v, want at most %v", elapsed, expectedMax)
	}

	if attempts != 4 {
		t.Errorf("expected 4 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_ExponentialBackoff(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:     4,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	attemptTimes := []time.Time{}
	operation := func() error {
		attemptTimes = append(attemptTimes, time.Now())
		return fmt.Errorf("503 service unavailable")
	}

	_ = RetryWithBackoff(context.Background(), testLogger(), policy, operation)

	// Verify delays are reasonable (jitter makes exact timing non-deterministic).
	// With jitter in [0.5, 1.0) compounding across iterations, we just verify:
	// 1. Delays are non-zero
	// 2. Total time is reasonable
	// 3. We got the expected number of attempts
	if len(attemptTimes) != 5 {
		t.Errorf("expected 5 attempts, got %d", len(attemptTimes))
	}

	for i := 1; i < len(attemptTimes); i++ {
		delay := attemptTimes[i].Sub(attemptTimes[i-1])
		if delay < 1*time.Millisecond {
			t.Errorf("attempt %d: delay %v too short", i, delay)
		}
		// With MaxBackoff=100ms and jitter up to 1.0, no single delay should exceed 120ms
		if delay > 120*time.Millisecond {
			t.Errorf("attempt %d: delay %v exceeds expected max", i, delay)
		}
	}
}

func TestRetryWithBackoff_MaxBackoff(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     200 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	attemptTimes := []time.Time{}
	operation := func() error {
		attemptTimes = append(attemptTimes, time.Now())
		return fmt.Errorf("connection reset")
	}

	_ = RetryWithBackoff(context.Background(), testLogger(), policy, operation)

	// Verify that backoff doesn't exceed MaxBackoff + overhead (jitter is [0.5, 1.0))
	for i := 2; i < len(attemptTimes); i++ {
		delay := attemptTimes[i].Sub(attemptTimes[i-1])

		// MaxBackoff is 200ms, with jitter up to 1.0x, plus scheduling overhead
		maxAllowed := policy.MaxBackoff + 20*time.Millisecond

		if delay > maxAllowed {
			t.Errorf("attempt %d: delay %v exceeds max allowed %v (MaxBackoff=%v with jitter)",
				i, delay, maxAllowed, policy.MaxBackoff)
		}
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil error", nil, false},
		{"timeout", fmt.Errorf("operation timeout"), true},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"connection refused", fmt.Errorf("connection refused"), true},
		{"EOF", fmt.Errorf("unexpected EOF"), true},
		{"broken pipe", fmt.Errorf("broken pipe"), true},
		{"temporary", fmt.Errorf("temporary network error"), true},
		{"429 rate limit", fmt.Errorf("HTTP 429 too many requests"), false}, // Rate limits should NOT be retried
		{"503 unavailable", fmt.Errorf("503 service unavailable"), true},
		{"504 timeout", fmt.Errorf("504 gateway timeout"), true},
		{"502 bad gateway", fmt.Errorf("502 bad gateway"), true},
		{"404 not found", fmt.Errorf("404 not found"), false},
		{"400 bad request", fmt.Errorf("400 bad request"), false},
		{"generic error", fmt.Errorf("some random error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.retryable {
				t.Errorf("isRetryableError(%v) = %v, want %v",
					tt.err, result, tt.retryable)
			}
		})
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", policy.MaxRetries)
	}

	if policy.InitialBackoff != 1*time.Second {
		t.Errorf("expected InitialBackoff=1s, got %v", policy.InitialBackoff)
	}

	if policy.MaxBackoff != 30*time.Second {
		t.Errorf("expected MaxBackoff=30s, got %v", policy.MaxBackoff)
	}

	if policy.BackoffFactor != 2.0 {
		t.Errorf("expected BackoffFactor=2.0, got %f", policy.BackoffFactor)
	}
}

func TestNewCircuitBreaker(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test-breaker")
	cb := NewCircuitBreaker(config, testLogger())

	if cb == nil {
		t.Fatal("expected circuit breaker, got nil")
	}

	if cb.Name() != "test-breaker" {
		t.Errorf("expected name 'test-breaker', got %s", cb.Name())
	}
}

func TestGetCircuitBreakerStats(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test-stats")
	cb := NewCircuitBreaker(config, testLogger())

	stats := GetCircuitBreakerStats(cb)

	if stats.Name != "test-stats" {
		t.Errorf("expected name 'test-stats', got %s", stats.Name)
	}

	if stats.State != "closed" {
		t.Errorf("expected initial state 'closed', got %s", stats.State)
	}

	if stats.Requests != 0 {
		t.Errorf("expected 0 requests initially, got %d", stats.Requests)
	}
}

func TestIsCircuitBreakerSuccess(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		isSuccess bool
	}{
		// Success cases (nil error)
		{"nil error", nil, true},

		// Business logic errors that should NOT trip the circuit (count as success)
		{"no match found (string)", fmt.Errorf("no match found for card: Pikachu"), true},
		{"no product match (string)", fmt.Errorf("no product match"), true},
		{"query too short (string)", fmt.Errorf("no product match - query too short"), true},
		{"AppError ProviderNotFound", apperrors.ProviderNotFound("doubleholo", "Pikachu"), true},
		{"AppError ProviderInvalidReq", apperrors.ProviderInvalidRequest("doubleholo", fmt.Errorf("bad request")), true},

		// Infrastructure errors that SHOULD trip the circuit (count as failure)
		{"AppError ProviderUnavailable", apperrors.ProviderUnavailable("doubleholo", fmt.Errorf("HTTP 503")), false},
		{"AppError ProviderTimeout", apperrors.ProviderTimeout("doubleholo", fmt.Errorf("timeout")), false},
		{"AppError ProviderRateLimited", apperrors.ProviderRateLimited("doubleholo", ""), false},
		{"AppError ProviderAuthFailed", apperrors.ProviderAuthFailed("doubleholo", fmt.Errorf("HTTP 401")), false},
		{"AppError ProviderCircuitOpen", apperrors.ProviderCircuitOpen("doubleholo"), false},
		{"generic error", fmt.Errorf("connection refused"), false},
		{"timeout error", fmt.Errorf("request timeout"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCircuitBreakerSuccess(tt.err)
			if result != tt.isSuccess {
				t.Errorf("isCircuitBreakerSuccess(%v) = %v, want %v",
					tt.err, result, tt.isSuccess)
			}
		})
	}
}

func TestCircuitBreakerDoesNotTripOnBusinessErrors(t *testing.T) {
	// Configure a circuit breaker with low thresholds for testing
	config := CircuitBreakerConfig{
		Name:         "test-business-errors",
		MaxRequests:  1,
		Interval:     60 * time.Second,
		Timeout:      1 * time.Second,
		MinRequests:  3, // Need at least 3 requests
		FailureRatio: 0.5,
	}

	cb := NewCircuitBreaker(config, testLogger())

	// Simulate multiple "no match found" errors (business logic errors)
	// These should NOT trip the circuit breaker
	for i := 0; i < 10; i++ {
		_, _ = cb.Execute(func() (any, error) {
			return nil, fmt.Errorf("no match found for card: Card%d", i)
		})
	}

	stats := GetCircuitBreakerStats(cb)

	// Circuit should still be closed because business errors don't count as failures
	if stats.State != "closed" {
		t.Errorf("Circuit breaker should remain closed after business logic errors, but state is: %s", stats.State)
	}

	// All requests counted but no failures
	if stats.Failures != 0 {
		t.Errorf("Expected 0 failures (business errors counted as success), got %d", stats.Failures)
	}
}

func TestCircuitBreakerTripsOnInfrastructureErrors(t *testing.T) {
	// Configure a circuit breaker with low thresholds for testing
	config := CircuitBreakerConfig{
		Name:         "test-infra-errors",
		MaxRequests:  1,
		Interval:     60 * time.Second,
		Timeout:      1 * time.Second,
		MinRequests:  3, // Need at least 3 requests
		FailureRatio: 0.5,
	}

	cb := NewCircuitBreaker(config, testLogger())

	// Simulate multiple infrastructure errors (these SHOULD trip the circuit)
	for i := 0; i < 10; i++ {
		_, _ = cb.Execute(func() (any, error) {
			return nil, apperrors.ProviderUnavailable("doubleholo", fmt.Errorf("HTTP 503"))
		})
	}

	stats := GetCircuitBreakerStats(cb)

	// Circuit should be open after infrastructure failures
	// Note: gobreaker clears counts when the state changes to open,
	// so we primarily check the state, not the failure count
	if stats.State != "open" {
		t.Errorf("Circuit breaker should be open after infrastructure errors, but state is: %s", stats.State)
	}
}
