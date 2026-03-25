package mocks

import (
	"fmt"
	"hash/fnv"
	"time"
)

// MockBehavior defines configurable behavior for all mock providers
type MockBehavior struct {
	// Error handling
	ShouldError   bool  // If true, return error on all calls
	ErrorToReturn error // Specific error to return
	FailAfterN    int   // Fail after N successful calls
	callCount     int   // Internal counter for FailAfterN

	// Performance simulation
	ResponseDelay   time.Duration // Delay before returning
	ShouldTimeout   bool          // If true, simulate timeout
	ShouldRateLimit bool          // If true, simulate rate limit

	// Data behavior
	ReturnEmptyData bool   // If true, return empty results
	ReturnAllCards  bool   // If true, SearchCards returns all cards without filtering (except Limit)
	DataVariant     string // Which variant of test data to return
}

// MockOption is a functional option for configuring mock behavior
type MockOption func(*MockBehavior)

// WithError configures the mock to return an error
func WithError(err error) MockOption {
	return func(b *MockBehavior) {
		b.ShouldError = true
		b.ErrorToReturn = err
	}
}

// WithFailAfterN configures the mock to fail after N successful calls
func WithFailAfterN(n int) MockOption {
	return func(b *MockBehavior) {
		b.FailAfterN = n
	}
}

// WithDelay adds a response delay to simulate network latency
func WithDelay(d time.Duration) MockOption {
	return func(b *MockBehavior) {
		b.ResponseDelay = d
	}
}

// WithTimeout simulates a timeout error
func WithTimeout() MockOption {
	return func(b *MockBehavior) {
		b.ShouldTimeout = true
		b.ShouldError = true
		b.ErrorToReturn = fmt.Errorf("mock: timeout")
	}
}

// WithRateLimit simulates a rate limit error
func WithRateLimit() MockOption {
	return func(b *MockBehavior) {
		b.ShouldRateLimit = true
		b.ShouldError = true
		b.ErrorToReturn = fmt.Errorf("mock: rate limit exceeded")
	}
}

// WithEmptyData returns empty results instead of mock data
func WithEmptyData() MockOption {
	return func(b *MockBehavior) {
		b.ReturnEmptyData = true
	}
}

// WithDataVariant specifies which variant of test data to return
func WithDataVariant(variant string) MockOption {
	return func(b *MockBehavior) {
		b.DataVariant = variant
	}
}

// WithReturnAllCards configures SearchCards to return all cards without filtering
func WithReturnAllCards() MockOption {
	return func(b *MockBehavior) {
		b.ReturnAllCards = true
	}
}

// checkBehavior checks if the mock should return an error based on configuration
func (b *MockBehavior) checkBehavior() error {
	// Simulate delay if configured
	if b.ResponseDelay > 0 {
		time.Sleep(b.ResponseDelay)
	}

	// Check if we should fail
	if b.ShouldError {
		if b.ErrorToReturn != nil {
			return b.ErrorToReturn
		}
		return fmt.Errorf("mock: configured to return error")
	}

	// Check FailAfterN
	if b.FailAfterN > 0 {
		b.callCount++
		if b.callCount > b.FailAfterN {
			return fmt.Errorf("mock: failed after %d calls", b.FailAfterN)
		}
	}

	return nil
}

// simpleHash generates a simple deterministic hash from a string
// Used to generate consistent but varied mock data
func simpleHash(s string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return int(h.Sum32())
}
