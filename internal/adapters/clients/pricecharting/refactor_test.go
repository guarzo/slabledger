package pricecharting

import (
	"context"
	"testing"
	"time"
)

// TestPriceCharting_Close tests the Close method
func TestPriceCharting_Close(t *testing.T) {
	pc, err := NewPriceCharting(DefaultConfig("test-token"), nil, nil)
	if err != nil {
		t.Fatalf("Failed to create PriceCharting: %v", err)
	}

	// Verify rate limiter is set
	if pc.rateLimiter == nil {
		t.Fatal("Expected non-nil rate limiter")
	}

	// Close should not return error
	if err := pc.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Calling Close multiple times should be safe
	if err := pc.Close(); err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}

// TestPriceCharting_ErrorHandling tests error propagation
func TestPriceCharting_ErrorHandling(t *testing.T) {
	// Test with nil cache - should succeed
	pc, err := NewPriceCharting(DefaultConfig("test-token"), nil, nil)
	if err != nil {
		t.Fatalf("Expected no error with nil cache, got: %v", err)
	}
	defer func() { _ = pc.Close() }()

	// Cache manager should be initialized even with nil cache
	if pc.cacheManager == nil {
		t.Error("Expected non-nil cache manager")
	}
}

// TestTickerRateLimiter tests the rate limiter implementation
func TestTickerRateLimiter(t *testing.T) {
	interval := 50 * time.Millisecond
	limiter := NewTickerRateLimiter(interval)

	if limiter == nil {
		t.Fatal("Expected non-nil limiter")
	}

	// Test WaitContext - should block for interval
	start := time.Now()
	ctx := context.Background()
	_ = limiter.WaitContext(ctx)
	duration := time.Since(start)

	// Should take at least the interval time (with some tolerance)
	if duration < interval/2 {
		t.Errorf("Wait() returned too quickly: %v (expected ~%v)", duration, interval)
	}

	// Test Stop
	limiter.Stop()

	// Calling Stop multiple times should be safe
	limiter.Stop()
}

// TestInterfaceImplementations verifies interfaces are properly implemented
func TestInterfaceImplementations(t *testing.T) {
	// Verify TickerRateLimiter implements rateLimiter
	var _ rateLimiter = (*TickerRateLimiter)(nil)
}

// TestPriceCharting_ResourceCleanup tests proper resource cleanup
func TestPriceCharting_ResourceCleanup(t *testing.T) {
	// Create multiple instances and close them
	for i := 0; i < 10; i++ {
		pc, err := NewPriceCharting(DefaultConfig("test-token"), nil, nil)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to create PriceCharting: %v", i, err)
		}

		if err := pc.Close(); err != nil {
			t.Errorf("Iteration %d: Close() failed: %v", i, err)
		}
	}
}
