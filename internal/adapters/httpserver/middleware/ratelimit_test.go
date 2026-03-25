package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// TestRateLimiter_BasicFunctionality tests basic rate limiting behavior
func TestRateLimiter_BasicFunctionality(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute, false, mocks.NewMockLogger())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 3 requests should succeed
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i, rr.Code)
		}

		// Check rate limit headers
		limit := rr.Header().Get("RateLimit-Limit")
		if limit != "3" {
			t.Errorf("Request %d: expected RateLimit-Limit=3, got %s", i, limit)
		}

		remaining := rr.Header().Get("RateLimit-Remaining")
		expected := strconv.Itoa(3 - i)
		if remaining != expected {
			t.Errorf("Request %d: expected RateLimit-Remaining=%s, got %s", i, expected, remaining)
		}
	}

	// 4th request should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", rr.Code)
	}

	// Check Retry-After header is present
	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("Expected Retry-After header to be set")
	}

	remaining := rr.Header().Get("RateLimit-Remaining")
	if remaining != "0" {
		t.Errorf("Expected RateLimit-Remaining=0, got %s", remaining)
	}
}

// TestRateLimiter_WindowReset tests that the rate limit window resets correctly
func TestRateLimiter_WindowReset(t *testing.T) {
	window := 500 * time.Millisecond
	rl := NewRateLimiter(2, window, false, mocks.NewMockLogger())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Make first request and record the reset time
	req1 := httptest.NewRequest("GET", "/api/test", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	t.Logf("Req 1: Status=%d, Limit=%s, Remaining=%s", rr1.Code, rr1.Header().Get("RateLimit-Limit"), rr1.Header().Get("RateLimit-Remaining"))
	if rr1.Code != http.StatusOK {
		t.Fatalf("First request should succeed, got status %d", rr1.Code)
	}

	// Make second request immediately
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.RemoteAddr = "192.168.1.1:12345"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	t.Logf("Req 2: Status=%d, Limit=%s, Remaining=%s", rr2.Code, rr2.Header().Get("RateLimit-Limit"), rr2.Header().Get("RateLimit-Remaining"))
	if rr2.Code != http.StatusOK {
		t.Fatalf("Second request should succeed, got status %d", rr2.Code)
	}

	// Third request should fail (limit exceeded)
	req3 := httptest.NewRequest("GET", "/api/test", nil)
	req3.RemoteAddr = "192.168.1.1:12345"
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	t.Logf("Req 3: Status=%d, Limit=%s, Remaining=%s", rr3.Code, rr3.Header().Get("RateLimit-Limit"), rr3.Header().Get("RateLimit-Remaining"))
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("Third request should be rate limited, got status %d", rr3.Code)
	}

	// Wait for window to reset
	time.Sleep(window + 100*time.Millisecond)

	// Should succeed now after window reset
	req4 := httptest.NewRequest("GET", "/api/test", nil)
	req4.RemoteAddr = "192.168.1.1:12345"
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req4)
	t.Logf("Req 4 (after reset): Status=%d, Limit=%s, Remaining=%s", rr4.Code, rr4.Header().Get("RateLimit-Limit"), rr4.Header().Get("RateLimit-Remaining"))
	if rr4.Code != http.StatusOK {
		t.Errorf("After window reset, expected status 200, got %d", rr4.Code)
	}
}

// TestRateLimiter_DifferentIPs tests that different IPs have separate rate limits
func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute, false, mocks.NewMockLogger())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ips := []string{"192.168.1.1:12345", "192.168.1.2:12345", "192.168.1.3:12345"}

	// Each IP should have its own limit
	for _, ip := range ips {
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "/api/test", nil)
			req.RemoteAddr = ip
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("IP %s request %d should succeed, got status %d", ip, i, rr.Code)
			}
		}

		// 3rd request should fail for this IP
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("IP %s 3rd request should be rate limited, got status %d", ip, rr.Code)
		}
	}
}

// TestRateLimiter_ProxyHeaders tests X-Forwarded-For and X-Real-IP support
func TestRateLimiter_ProxyHeaders(t *testing.T) {
	tests := []struct {
		name          string
		trustProxy    bool
		remoteAddr    string
		xForwardedFor string
		xRealIP       string
		expectedIP    string
	}{
		{
			name:          "TrustProxy=false, ignores headers",
			trustProxy:    false,
			remoteAddr:    "192.168.1.1:12345",
			xForwardedFor: "1.2.3.4",
			xRealIP:       "5.6.7.8",
			expectedIP:    "192.168.1.1",
		},
		{
			name:          "TrustProxy=true without trusted proxies, ignores XFF and X-Real-IP, uses RemoteAddr",
			trustProxy:    true,
			remoteAddr:    "192.168.1.1:12345",
			xForwardedFor: "1.2.3.4, 5.6.7.8",
			xRealIP:       "9.10.11.12",
			expectedIP:    "192.168.1.1",
		},
		{
			name:          "TrustProxy=true without trusted proxies, ignores X-Real-IP, uses RemoteAddr",
			trustProxy:    true,
			remoteAddr:    "192.168.1.1:12345",
			xForwardedFor: "",
			xRealIP:       "9.10.11.12",
			expectedIP:    "192.168.1.1",
		},
		{
			name:          "TrustProxy=true, falls back to RemoteAddr",
			trustProxy:    true,
			remoteAddr:    "192.168.1.1:12345",
			xForwardedFor: "",
			xRealIP:       "",
			expectedIP:    "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(10, time.Minute, tt.trustProxy, mocks.NewMockLogger())
			defer rl.Close()

			req := httptest.NewRequest("GET", "/api/test", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			ip := rl.clientIP(req, tt.trustProxy)
			if ip != tt.expectedIP {
				t.Errorf("Expected IP %s, got %s", tt.expectedIP, ip)
			}
		})
	}
}

// TestRateLimiter_TrustedProxies tests the trusted proxy IP extraction (spoofing prevention)
func TestRateLimiter_TrustedProxies(t *testing.T) {
	tests := []struct {
		name           string
		trustedProxies []string
		remoteAddr     string
		xForwardedFor  string
		expectedIP     string
	}{
		{
			name:           "Single trusted proxy, extracts client IP",
			trustedProxies: []string{"10.0.0.1"},
			remoteAddr:     "10.0.0.1:12345",
			xForwardedFor:  "203.0.113.50, 10.0.0.1",
			expectedIP:     "203.0.113.50",
		},
		{
			name:           "Multiple trusted proxies in chain",
			trustedProxies: []string{"10.0.0.1", "10.0.0.2"},
			remoteAddr:     "10.0.0.1:12345",
			xForwardedFor:  "203.0.113.50, 10.0.0.2, 10.0.0.1",
			expectedIP:     "203.0.113.50",
		},
		{
			name:           "CIDR trusted proxy range",
			trustedProxies: []string{"10.0.0.0/8"},
			remoteAddr:     "10.1.2.3:12345",
			xForwardedFor:  "203.0.113.50, 10.100.200.50, 10.1.2.3",
			expectedIP:     "203.0.113.50",
		},
		{
			name:           "Spoofed IP ignored - rightmost untrusted wins",
			trustedProxies: []string{"10.0.0.1"},
			remoteAddr:     "10.0.0.1:12345",
			xForwardedFor:  "1.1.1.1, 203.0.113.50, 10.0.0.1",
			expectedIP:     "203.0.113.50", // Not the spoofed 1.1.1.1
		},
		{
			name:           "All IPs trusted, falls back to RemoteAddr",
			trustedProxies: []string{"10.0.0.0/8"},
			remoteAddr:     "192.168.1.1:12345",
			xForwardedFor:  "10.0.0.1, 10.0.0.2",
			expectedIP:     "192.168.1.1",
		},
		{
			name:           "Empty X-Forwarded-For with trusted proxies",
			trustedProxies: []string{"10.0.0.1"},
			remoteAddr:     "192.168.1.1:12345",
			xForwardedFor:  "",
			expectedIP:     "192.168.1.1",
		},
		{
			name:           "Mixed CIDR and exact IP in trusted list",
			trustedProxies: []string{"10.0.0.0/8", "172.16.0.1"},
			remoteAddr:     "172.16.0.1:12345",
			xForwardedFor:  "203.0.113.50, 10.50.100.200, 172.16.0.1",
			expectedIP:     "203.0.113.50",
		},
		{
			name:           "Single IP chain with untrusted proxy",
			trustedProxies: []string{"10.0.0.1"},
			remoteAddr:     "10.0.0.1:12345",
			xForwardedFor:  "203.0.113.50",
			expectedIP:     "203.0.113.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiterWithTrustedProxies(10, time.Minute, true, tt.trustedProxies, mocks.NewMockLogger())
			defer rl.Close()

			req := httptest.NewRequest("GET", "/api/test", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}

			ip := rl.clientIP(req, true)
			if ip != tt.expectedIP {
				t.Errorf("Expected IP %s, got %s", tt.expectedIP, ip)
			}
		})
	}
}

// TestRateLimiter_IsTrustedProxy tests the isTrustedProxy helper
func TestRateLimiter_IsTrustedProxy(t *testing.T) {
	rl := NewRateLimiterWithTrustedProxies(10, time.Minute, true, []string{
		"10.0.0.1",       // Exact IP
		"192.168.0.0/24", // Class C network
		"172.16.0.0/12",  // Class B private range
	}, mocks.NewMockLogger())
	defer rl.Close()

	tests := []struct {
		ip       string
		expected bool
	}{
		{"10.0.0.1", true},       // Exact match
		{"10.0.0.2", false},      // Not in list
		{"192.168.0.1", true},    // In /24 CIDR
		{"192.168.0.255", true},  // In /24 CIDR (last IP)
		{"192.168.1.1", false},   // Outside /24 CIDR
		{"172.16.0.1", true},     // In /12 CIDR
		{"172.31.255.255", true}, // In /12 CIDR (last IP)
		{"172.32.0.1", false},    // Outside /12 CIDR
		{"invalid", false},       // Invalid IP
		{"", false},              // Empty string
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := rl.isTrustedProxy(tt.ip)
			if result != tt.expected {
				t.Errorf("isTrustedProxy(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

// TestRateLimiter_SpoofingPrevention tests that IP spoofing is prevented
func TestRateLimiter_SpoofingPrevention(t *testing.T) {
	// Scenario: Attacker tries to bypass rate limiting by spoofing X-Forwarded-For
	// The attacker adds "1.1.1.1" at the start of X-Forwarded-For to appear as a different client

	trustedProxies := []string{"10.0.0.1", "10.0.0.2"} // Load balancer and proxy
	rl := NewRateLimiterWithTrustedProxies(2, time.Minute, true, trustedProxies, mocks.NewMockLogger())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Real client IP is 203.0.113.50
	// Attacker tries to spoof by adding fake IPs at the start
	makeRequest := func(spoofedPrefix string) int {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "10.0.0.1:12345" // Request comes from load balancer
		// Attacker prepends fake IPs, but real client (203.0.113.50) was added by first trusted proxy
		xff := spoofedPrefix + "203.0.113.50, 10.0.0.2, 10.0.0.1"
		req.Header.Set("X-Forwarded-For", xff)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	// First two requests should succeed (limit is 2)
	// Even though attacker tries to spoof different IPs, we extract the real client IP
	if code := makeRequest("1.1.1.1, "); code != http.StatusOK {
		t.Errorf("Request 1 with spoofed IP should succeed, got %d", code)
	}
	if code := makeRequest("2.2.2.2, "); code != http.StatusOK {
		t.Errorf("Request 2 with different spoofed IP should succeed, got %d", code)
	}

	// Third request should be rate limited because we correctly identify the real client
	if code := makeRequest("3.3.3.3, "); code != http.StatusTooManyRequests {
		t.Errorf("Request 3 should be rate limited despite spoofed IP, got %d", code)
	}
}

// TestRateLimiter_GC tests garbage collection of dormant IPs
func TestRateLimiter_GC(t *testing.T) {
	// Use short intervals for testing
	rl := &RateLimiter{
		reqPerWindow: 10,
		window:       time.Minute,
		trustProxy:   false,
		buckets:      make(map[string]*bucket),
		stopCh:       make(chan struct{}),
	}

	// Manually create some buckets with different last-seen times
	now := time.Now()
	rl.buckets["old-ip"] = &bucket{
		count:    1,
		resetAt:  now.Add(time.Minute),
		lastSeen: now.Add(-20 * time.Minute), // Old, should be GC'd
	}
	rl.buckets["recent-ip"] = &bucket{
		count:    1,
		resetAt:  now.Add(time.Minute),
		lastSeen: now.Add(-5 * time.Minute), // Recent, should remain
	}

	// Manually trigger GC logic
	cutoff := now.Add(-15 * time.Minute)
	rl.mu.Lock()
	for ip, b := range rl.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(rl.buckets, ip)
		}
	}
	rl.mu.Unlock()

	// Check that old IP was removed
	if _, exists := rl.buckets["old-ip"]; exists {
		t.Error("Expected old-ip to be garbage collected")
	}

	// Check that recent IP remains
	if _, exists := rl.buckets["recent-ip"]; !exists {
		t.Error("Expected recent-ip to remain after GC")
	}

	rl.Close()
}

// TestRateLimiter_GCRoutine tests that the GC goroutine runs and stops correctly
func TestRateLimiter_GCRoutine(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute, false, mocks.NewMockLogger())

	// Add a bucket that should be GC'd (but we won't wait long enough for it to happen in this test)
	rl.mu.Lock()
	rl.buckets["test-ip"] = &bucket{
		count:    1,
		resetAt:  time.Now().Add(time.Minute),
		lastSeen: time.Now().Add(-20 * time.Minute),
	}
	rl.mu.Unlock()

	// Close the rate limiter - this should stop the GC goroutine
	rl.Close()

	// Wait a bit to ensure goroutine has time to stop
	time.Sleep(100 * time.Millisecond)

	// Test passes if we don't panic or hang
}

// TestRateLimiter_NonAPIEndpoints tests that non-API endpoints are not rate limited
func TestRateLimiter_NonAPIEndpoints(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute, false, mocks.NewMockLogger())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Non-API endpoints should not be rate limited
	paths := []string{"/", "/index.html", "/static/app.js", "/favicon.ico"}

	for _, path := range paths {
		// Make many requests (more than the limit)
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", path, nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Path %s request %d: expected status 200, got %d", path, i, rr.Code)
			}

			// Should not have rate limit headers for non-API endpoints
			if rr.Header().Get("RateLimit-Limit") != "" {
				t.Errorf("Path %s should not have rate limit headers", path)
			}
		}
	}
}

// TestRateLimiter_CollectionEndpointsExempt tests that collection endpoints are exempt
func TestRateLimiter_CollectionEndpointsExempt(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute, false, mocks.NewMockLogger())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Collection endpoints should be exempt from rate limiting
	paths := []string{"/api/collection/items", "/api/collection/stats"}

	for _, path := range paths {
		// Make many requests (more than the limit)
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", path, nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Path %s request %d: expected status 200, got %d", path, i, rr.Code)
			}
		}
	}
}

// TestRateLimiter_ConcurrentRequests tests thread safety with concurrent requests
func TestRateLimiter_ConcurrentRequests(t *testing.T) {
	rl := NewRateLimiter(100, time.Minute, false, mocks.NewMockLogger())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Run concurrent requests from different IPs
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 20; j++ {
				req := httptest.NewRequest("GET", "/api/test", nil)
				req.RemoteAddr = "192.168.1." + strconv.Itoa(id) + ":12345"
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test passes if we don't have race conditions or panics
}

// BenchmarkRateLimiter_Middleware benchmarks the middleware overhead
func BenchmarkRateLimiter_Middleware(b *testing.B) {
	rl := NewRateLimiter(1000000, time.Minute, false, mocks.NewMockLogger())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}
