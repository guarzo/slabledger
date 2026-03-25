package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
)

const defaultOrigin = "http://localhost:8081"

func TestCORSMiddleware_AllowedOrigins(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name            string
		origin          string
		shouldBeAllowed bool
	}{
		{"localhost:8081", defaultOrigin, true},
		{"127.0.0.1:8081", "http://127.0.0.1:8081", true},
		{"external domain", "https://evil.com", false},
		{"external IP", "http://192.168.1.100:8080", false},
		{"https localhost", "https://localhost:8081", false}, // Only http allowed by default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Origin", tt.origin)
			rec := httptest.NewRecorder()

			middleware := CORSMiddleware(mocks.NewMockLogger(), handler)
			middleware.ServeHTTP(rec, req)

			allowedOrigin := rec.Header().Get("Access-Control-Allow-Origin")

			if tt.shouldBeAllowed {
				assert.Equal(t, tt.origin, allowedOrigin, "Expected origin to be allowed")
			} else {
				assert.NotEqual(t, tt.origin, allowedOrigin, "Expected origin to be blocked")
			}
		})
	}
}

func TestCORSMiddleware_CustomOriginEnvVar(t *testing.T) {
	// Set custom origin via environment variable
	os.Setenv("ALLOWED_ORIGIN", "https://custom.example.com,https://another.example.com")
	defer os.Unsetenv("ALLOWED_ORIGIN")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name            string
		origin          string
		shouldBeAllowed bool
	}{
		{"default localhost still works", defaultOrigin, true},
		{"custom origin 1", "https://custom.example.com", true},
		{"custom origin 2", "https://another.example.com", true},
		{"non-configured origin", "https://evil.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Origin", tt.origin)
			rec := httptest.NewRecorder()

			middleware := CORSMiddleware(mocks.NewMockLogger(), handler)
			middleware.ServeHTTP(rec, req)

			allowedOrigin := rec.Header().Get("Access-Control-Allow-Origin")

			if tt.shouldBeAllowed {
				assert.Equal(t, tt.origin, allowedOrigin, "Expected origin to be allowed")
			} else {
				assert.NotEqual(t, tt.origin, allowedOrigin, "Expected origin to be blocked")
			}
		})
	}
}

func TestCORSMiddleware_NoOriginHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// Don't set Origin header (same-origin request)
	rec := httptest.NewRecorder()

	middleware := CORSMiddleware(mocks.NewMockLogger(), handler)
	middleware.ServeHTTP(rec, req)

	// Same-origin requests should NOT have CORS headers (proper security practice)
	allowedOrigin := rec.Header().Get("Access-Control-Allow-Origin")
	assert.Empty(t, allowedOrigin, "Same-origin requests should not have CORS headers")

	// Request should succeed (handler called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", defaultOrigin)
	rec := httptest.NewRecorder()

	middleware := CORSMiddleware(mocks.NewMockLogger(), handler)
	middleware.ServeHTTP(rec, req)

	// Preflight should return 204 No Content
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Should have proper CORS headers
	assert.Equal(t, defaultOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, Authorization", rec.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "3600", rec.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddleware_SecurityHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", defaultOrigin)
	rec := httptest.NewRecorder()

	middleware := CORSMiddleware(mocks.NewMockLogger(), handler)
	middleware.ServeHTTP(rec, req)

	// Verify all CORS headers are set
	assert.Equal(t, defaultOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, Authorization", rec.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "3600", rec.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddleware_BlocksMaliciousOrigins(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	maliciousOrigins := []string{
		"https://evil.com",
		"http://phishing.site",
		"https://192.168.1.100:8080",
		"http://10.0.0.1:8080",
		"https://attack.localhost.com", // Not localhost
		"javascript:alert('xss')",
		"data:text/html,<script>alert('xss')</script>",
	}

	for _, origin := range maliciousOrigins {
		t.Run("block_"+origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()

			middleware := CORSMiddleware(mocks.NewMockLogger(), handler)
			middleware.ServeHTTP(rec, req)

			allowedOrigin := rec.Header().Get("Access-Control-Allow-Origin")
			assert.NotEqual(t, origin, allowedOrigin, "Should block malicious origin: %s", origin)
		})
	}
}
