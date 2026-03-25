// Package middleware provides HTTP middleware for the web server.
package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// RateLimiter manages rate limiting for API requests with automatic garbage collection.
// It implements a token bucket algorithm per client IP address with configurable limits.
//
// Features:
//   - Automatic garbage collection of dormant IPs (prevents memory leaks)
//   - Support for proxy headers (X-Forwarded-For, X-Real-IP)
//   - Standard RFC 6585 rate limit headers
//   - Configurable request window and limits
//   - Thread-safe concurrent access
//   - Trusted proxy support to prevent IP spoofing
//   - Auth mode for stricter rate limiting on authentication endpoints
type RateLimiter struct {
	reqPerWindow   int                  // Maximum requests per window
	window         time.Duration        // Time window for rate limiting
	trustProxy     bool                 // Whether to trust X-Forwarded-For headers
	trustedProxies []string             // List of trusted proxy IPs/CIDRs
	parsedNets     []*net.IPNet         // Pre-parsed CIDR networks from trustedProxies
	parsedIPs      []net.IP             // Pre-parsed IPs from trustedProxies
	authMode       bool                 // When true, applies to all requests with auth-specific 429 message
	logger         observability.Logger // Logger for security events

	mu       sync.Mutex
	buckets  map[string]*bucket // IP -> bucket mapping
	stopCh   chan struct{}      // Channel to stop GC goroutine
	stopOnce sync.Once          // Ensures Close() is idempotent
}

// bucket tracks rate limit state for a single client IP
type bucket struct {
	count    int       // Number of requests in current window
	resetAt  time.Time // When the window resets
	lastSeen time.Time // Last activity timestamp (for GC)
}

// NewRateLimiter creates a new rate limiter with background garbage collection.
//
// Parameters:
//   - reqPerWindow: Maximum number of requests allowed per window
//   - window: Time duration for the rate limit window
//   - trustProxy: Whether to trust X-Forwarded-For and X-Real-IP headers
//   - logger: Logger for security events
//
// The garbage collector runs every 5 minutes and removes IPs that haven't
// been seen in the last 15 minutes, preventing unbounded memory growth.
func NewRateLimiter(reqPerWindow int, window time.Duration, trustProxy bool, logger observability.Logger) *RateLimiter {
	return NewRateLimiterWithTrustedProxies(reqPerWindow, window, trustProxy, nil, logger)
}

// NewRateLimiterWithTrustedProxies creates a new rate limiter with trusted proxy support.
//
// Parameters:
//   - reqPerWindow: Maximum number of requests allowed per window
//   - window: Time duration for the rate limit window
//   - trustProxy: Whether to trust X-Forwarded-For and X-Real-IP headers
//   - trustedProxies: List of trusted proxy IPs (e.g., ["10.0.0.0/8", "192.168.1.1"])
//   - logger: Logger for security events
//
// When trustProxy is true and trustedProxies is configured, the rate limiter will
// walk the X-Forwarded-For header from right to left and return the first IP that
// is not in the trusted proxies list. This prevents IP spoofing attacks where
// attackers prepend fake IPs to the header.
func NewRateLimiterWithTrustedProxies(reqPerWindow int, window time.Duration, trustProxy bool, trustedProxies []string, logger observability.Logger) *RateLimiter {
	rl := &RateLimiter{
		reqPerWindow:   reqPerWindow,
		window:         window,
		trustProxy:     trustProxy,
		trustedProxies: trustedProxies,
		logger:         logger,
		buckets:        make(map[string]*bucket),
		stopCh:         make(chan struct{}),
	}
	for _, trusted := range trustedProxies {
		if strings.Contains(trusted, "/") {
			_, cidr, err := net.ParseCIDR(trusted)
			if err == nil {
				rl.parsedNets = append(rl.parsedNets, cidr)
			}
		} else {
			if ip := net.ParseIP(trusted); ip != nil {
				rl.parsedIPs = append(rl.parsedIPs, ip)
			}
		}
	}
	go rl.gc()
	return rl
}

// NewAuthRateLimiter creates a rate limiter for authentication endpoints.
// It uses auth mode which applies rate limiting to all requests (no path filtering)
// and returns an auth-specific 429 message ("Too many authentication attempts").
// Proxy headers are trusted only when trustedProxies is non-empty.
//
// Recommended settings:
//   - reqPerWindow: 10 (conservative for auth endpoints)
//   - window: 1 second
func NewAuthRateLimiter(reqPerWindow int, window time.Duration, trustedProxies []string, logger observability.Logger) *RateLimiter {
	rl := &RateLimiter{
		reqPerWindow:   reqPerWindow,
		window:         window,
		trustProxy:     len(trustedProxies) > 0,
		trustedProxies: trustedProxies,
		authMode:       true,
		logger:         logger,
		buckets:        make(map[string]*bucket),
		stopCh:         make(chan struct{}),
	}
	go rl.gc()
	return rl
}

// Close stops the background garbage collector.
// This should be called when the rate limiter is no longer needed to prevent
// goroutine leaks. Safe to call multiple times.
func (rl *RateLimiter) Close() {
	rl.stopOnce.Do(func() { close(rl.stopCh) })
}

// gc performs background garbage collection to prevent unbounded map growth.
// It runs every 5 minutes and removes buckets for IPs that haven't been seen
// in the last 15 minutes.
func (rl *RateLimiter) gc() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-15 * time.Minute)
			rl.mu.Lock()
			for ip, b := range rl.buckets {
				if b.lastSeen.Before(cutoff) {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Middleware returns a middleware function that applies rate limiting.
//
// In normal mode, it only applies to /api/* endpoints, excluding read-only
// collection and autocomplete endpoints.
//
// In auth mode, it applies to all requests with an auth-specific 429 message.
//
// The middleware adds standard RFC 6585 headers:
//   - RateLimit-Limit: Maximum requests per window
//   - RateLimit-Remaining: Remaining requests in current window
//   - RateLimit-Reset: Seconds until window resets
//   - Retry-After: Seconds to wait before retrying (when rate limited)
//
// Returns HTTP 429 Too Many Requests when rate limit is exceeded.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl.shouldRateLimit(r) {
			ip := rl.clientIP(r, rl.trustProxy)

			now := time.Now()
			rl.mu.Lock()
			b := rl.buckets[ip]
			if b == nil {
				b = &bucket{resetAt: now.Add(rl.window)}
				rl.buckets[ip] = b
			}
			if now.After(b.resetAt) {
				b.count = 0
				b.resetAt = now.Add(rl.window)
			}
			b.count++
			b.lastSeen = now

			count := b.count
			resetAt := b.resetAt
			rl.mu.Unlock()

			// Calculate remaining requests
			remaining := rl.reqPerWindow - count
			if remaining < 0 {
				remaining = 0
			}

			// Standard rate limit headers (RFC 6585)
			w.Header().Set("RateLimit-Limit", strconv.Itoa(rl.reqPerWindow))
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("RateLimit-Reset", strconv.Itoa(int(time.Until(resetAt).Seconds())))

			// Reject if limit exceeded
			if count > rl.reqPerWindow {
				// Log security event
				logMsg := "Rate limit exceeded"
				if rl.authMode {
					logMsg = "Auth rate limit exceeded"
				}
				rl.logger.Warn(r.Context(), logMsg,
					observability.String("ip", ip),
					observability.String("path", r.URL.Path),
					observability.String("user_agent", r.UserAgent()))

				retryAfter := int(time.Until(resetAt).Seconds())
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))

				errMsg := http.StatusText(http.StatusTooManyRequests)
				if rl.authMode {
					errMsg = "Too many authentication attempts"
				}
				http.Error(w, errMsg, http.StatusTooManyRequests)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// shouldRateLimit determines whether a request should be subject to rate limiting.
// In auth mode, all requests are rate limited.
// In normal mode, only /api/* endpoints are rate limited, excluding collection
// and autocomplete endpoints.
func (rl *RateLimiter) shouldRateLimit(r *http.Request) bool {
	if rl.authMode {
		return true
	}

	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}

	// Exempt read-only endpoints from rate limiting
	// Collection endpoints: read-only, need to load together
	if strings.HasPrefix(r.URL.Path, "/api/collection/") {
		return false
	}

	return true
}

// clientIP extracts the client IP from the request, respecting proxy headers if enabled.
//
// When trustProxy is true and trustedProxies is configured, it first validates that the
// immediate sender (RemoteAddr) is a trusted proxy before honoring X-Forwarded-For headers.
// It walks the XFF chain from right to left and returns the first untrusted IP.
// This prevents IP spoofing attacks where attackers prepend fake IPs to the header.
//
// When trustProxy is true but trustedProxies is empty, X-Forwarded-For is ignored
// (since the leftmost IP can be trivially spoofed) and X-Real-IP is checked instead.
//
// When trustProxy is false, it only uses RemoteAddr.
func (rl *RateLimiter) clientIP(r *http.Request, trustProxy bool) string {
	// Extract remote IP from RemoteAddr first
	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteIP = host
	}

	if trustProxy {
		// If trusted proxies are configured, only honor XFF if the immediate sender is trusted
		if len(rl.trustedProxies) > 0 && !rl.isTrustedProxy(remoteIP) {
			// Direct client is not a trusted proxy - ignore XFF headers to prevent spoofing
			return remoteIP
		}

		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")

			// Walk from right to left and find the first non-trusted IP (the real client).
			// If no trusted proxies are configured, the leftmost IP is NOT used because
			// it can be trivially spoofed by the client. Instead, fall through to RemoteAddr.
			if len(rl.trustedProxies) > 0 {
				for i := len(ips) - 1; i >= 0; i-- {
					ip := strings.TrimSpace(ips[i])
					if ip == "" {
						continue
					}
					// Return first untrusted IP from the right
					if !rl.isTrustedProxy(ip) {
						return ip
					}
				}
				// All IPs in XFF are trusted, fall back to RemoteAddr
			}
		}
		// Only trust X-Real-IP when the sender is a known trusted proxy
		if len(rl.trustedProxies) > 0 {
			if xr := r.Header.Get("X-Real-IP"); xr != "" {
				return xr
			}
		}
	}
	return remoteIP
}

// isTrustedProxy checks if the given IP is in the trusted proxies list.
// Uses pre-parsed CIDRs and IPs for efficiency (parsed once at construction).
func (rl *RateLimiter) isTrustedProxy(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	for _, cidr := range rl.parsedNets {
		if cidr.Contains(parsedIP) {
			return true
		}
	}
	for _, trusted := range rl.parsedIPs {
		if trusted.Equal(parsedIP) {
			return true
		}
	}
	return false
}
