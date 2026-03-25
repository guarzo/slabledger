package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// apiKeyError writes a JSON error in the pricing API format: {"error": "code", "message": "description"}.
func apiKeyError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message}) //nolint:errcheck // response already committed; write error unactionable
}

// RequireAPIKey returns middleware that validates Bearer token authentication.
// It uses constant-time comparison to prevent timing attacks.
func RequireAPIKey(apiKey string) func(http.Handler) http.Handler {
	keyBytes := []byte(apiKey)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				apiKeyError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing API key")
				return
			}

			token := []byte(strings.TrimPrefix(auth, "Bearer "))
			if subtle.ConstantTimeCompare(token, keyBytes) != 1 {
				apiKeyError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// APIRateLimiter provides global rate limiting for the pricing API.
type APIRateLimiter struct {
	mu           sync.Mutex
	reqPerWindow int
	window       time.Duration
	count        int
	resetAt      time.Time
}

// NewAPIRateLimiter creates a rate limiter for the pricing API.
func NewAPIRateLimiter(reqPerWindow int, window time.Duration) *APIRateLimiter {
	return &APIRateLimiter{
		reqPerWindow: reqPerWindow,
		window:       window,
		resetAt:      time.Now().Add(window),
	}
}

// Middleware wraps a handler with rate limiting. Returns 429 with Retry-After
// header when limit is exceeded.
func (rl *APIRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		rl.mu.Lock()
		if now.After(rl.resetAt) {
			rl.count = 0
			rl.resetAt = now.Add(rl.window)
		}
		count := rl.count
		resetAt := rl.resetAt
		if count >= rl.reqPerWindow {
			rl.mu.Unlock()
			retryAfter := int(time.Until(resetAt).Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			apiKeyError(w, http.StatusTooManyRequests, "rate_limited",
				fmt.Sprintf("Rate limit exceeded. Retry after %d seconds.", retryAfter))
			return
		}
		rl.count++
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
