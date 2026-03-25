package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthRateLimiter_EnforcesLimit tests that the auth rate limiter enforces the request limit
func TestAuthRateLimiter_EnforcesLimit(t *testing.T) {
	limiter := NewAuthRateLimiter(10, time.Second, nil, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Make requests up to and exceeding the limit
	for i := 0; i < 15; i++ {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if i < 10 {
			assert.Equal(t, http.StatusOK, rec.Code, "Request %d should succeed", i+1)
		} else {
			assert.Equal(t, http.StatusTooManyRequests, rec.Code, "Request %d should be rate limited", i+1)
		}
	}
}

// TestAuthRateLimiter_PerIPTracking tests that different IPs have separate rate limits
func TestAuthRateLimiter_PerIPTracking(t *testing.T) {
	limiter := NewAuthRateLimiter(5, time.Second, nil, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ips := []string{"192.168.1.1:12345", "192.168.1.2:12345", "192.168.1.3:12345"}

	// Each IP should have its own separate limit
	for _, ip := range ips {
		// Make 5 requests (the limit) for each IP
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("POST", "/auth/login", nil)
			req.RemoteAddr = ip
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code, "IP %s request %d should succeed", ip, i+1)
		}

		// 6th request should fail for this IP
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTooManyRequests, rec.Code, "IP %s 6th request should be rate limited", ip)
	}
}

// TestAuthRateLimiter_WindowReset tests that the rate limit window resets
func TestAuthRateLimiter_WindowReset(t *testing.T) {
	window := 200 * time.Millisecond
	limiter := NewAuthRateLimiter(3, window, nil, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// 4th request should be rate limited
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	// Wait for window to reset
	time.Sleep(window + 50*time.Millisecond)

	// Request after window reset should succeed
	req = httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "Request after window reset should succeed")
}

// TestAuthRateLimiter_RateLimitHeaders tests that rate limit headers are properly set
func TestAuthRateLimiter_RateLimitHeaders(t *testing.T) {
	limiter := NewAuthRateLimiter(5, time.Second, nil, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Check rate limit headers
	assert.Equal(t, "5", rec.Header().Get("RateLimit-Limit"), "RateLimit-Limit header should be set")
	assert.Equal(t, "4", rec.Header().Get("RateLimit-Remaining"), "RateLimit-Remaining should be limit - 1")
	assert.NotEmpty(t, rec.Header().Get("RateLimit-Reset"), "RateLimit-Reset header should be set")
}

// TestAuthRateLimiter_RetryAfterHeader tests that Retry-After header is set when rate limited
func TestAuthRateLimiter_RetryAfterHeader(t *testing.T) {
	limiter := NewAuthRateLimiter(1, time.Second, nil, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request succeeds
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request should be rate limited
	req = httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"), "Retry-After header should be set when rate limited")
}

// TestAuthRateLimiter_XForwardedFor tests that X-Forwarded-For header is respected
func TestAuthRateLimiter_XForwardedFor(t *testing.T) {
	limiter := NewAuthRateLimiter(2, time.Second, []string{"127.0.0.1"}, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Two requests from same X-Forwarded-For IP
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "10.0.0.1, 192.168.1.1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Third request from same X-Forwarded-For should be rate limited
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 192.168.1.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

// TestAuthRateLimiter_XRealIP tests that X-Real-IP header is respected
func TestAuthRateLimiter_XRealIP(t *testing.T) {
	limiter := NewAuthRateLimiter(2, time.Second, []string{"127.0.0.1"}, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Two requests from same X-Real-IP
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("X-Real-IP", "10.0.0.50")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Third request from same X-Real-IP should be rate limited
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Real-IP", "10.0.0.50")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

// TestAuthRateLimiter_GarbageCollection tests that dormant IPs are cleaned up
func TestAuthRateLimiter_GarbageCollection(t *testing.T) {
	// This test verifies the GC mechanism exists and runs
	// The actual cleanup happens after 15 minutes of dormancy,
	// which is impractical to test. Instead, we verify the GC goroutine
	// is running by checking Close() stops it properly.

	limiter := NewAuthRateLimiter(10, time.Second, nil, mocks.NewMockLogger())

	// Make a few requests to populate the buckets
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify the bucket was created
	limiter.mu.Lock()
	_, exists := limiter.buckets["192.168.1.1"]
	limiter.mu.Unlock()
	require.True(t, exists, "bucket should exist for IP")

	// Close should work without hanging (GC goroutine should stop)
	done := make(chan struct{})
	go func() {
		limiter.Close()
		close(done)
	}()

	select {
	case <-done:
		// Success - Close completed
	case <-time.After(1 * time.Second):
		t.Fatal("Close() did not complete within timeout - GC goroutine may be stuck")
	}
}

// TestAuthRateLimiter_Close tests that Close properly stops the rate limiter
func TestAuthRateLimiter_Close(t *testing.T) {
	limiter := NewAuthRateLimiter(10, time.Second, nil, mocks.NewMockLogger())

	// Close should not panic
	require.NotPanics(t, func() {
		limiter.Close()
	})
}

// TestAuthRateLimiter_MiddlewareAfterClose tests that the middleware gracefully handles
// requests after Close() has been called - it should not panic and should allow requests through
func TestAuthRateLimiter_MiddlewareAfterClose(t *testing.T) {
	limiter := NewAuthRateLimiter(10, time.Second, nil, mocks.NewMockLogger())

	// Track whether the next handler was invoked
	nextHandlerCalled := false
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	// Close the limiter before making a request
	limiter.Close()

	// The middleware should not panic and should allow the request through
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() {
		handler.ServeHTTP(rec, req)
	}, "Middleware should not panic after Close()")

	assert.True(t, nextHandlerCalled, "Next handler should be invoked after Close()")
	assert.Equal(t, http.StatusOK, rec.Code, "Request should succeed after Close()")
}

// TestAuthRateLimiter_AuthMode tests that auth mode applies to all requests
func TestAuthRateLimiter_AuthMode(t *testing.T) {
	limiter := NewAuthRateLimiter(2, time.Second, nil, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Auth mode should rate limit ALL paths, including non-API paths
	paths := []string{"/auth/login", "/", "/static/file.js", "/api/collection/items"}
	for _, path := range paths {
		req := httptest.NewRequest("POST", path, nil)
		req.RemoteAddr = "10.0.0." + path + ":12345" // Unique IP per path
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "First request to %s should succeed", path)

		// Verify rate limit headers are present for all paths
		assert.NotEmpty(t, rec.Header().Get("RateLimit-Limit"), "RateLimit-Limit should be set for %s", path)
	}
}

// TestAuthRateLimiter_AuthMessage tests that auth mode returns the auth-specific error message
func TestAuthRateLimiter_AuthMessage(t *testing.T) {
	limiter := NewAuthRateLimiter(1, time.Second, nil, mocks.NewMockLogger())
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request succeeds
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request should be rate limited with auth message
	req = httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "Too many authentication attempts")
}
