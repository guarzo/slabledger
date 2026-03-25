package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name         string
		path         string
		checkHeaders map[string]string
		skipCSP      bool // CSP not applied to API requests
	}{
		{
			name: "web UI request includes all headers",
			path: "/",
			checkHeaders: map[string]string{
				"X-Content-Type-Options":  "nosniff",
				"X-Frame-Options":         "DENY",
				"Referrer-Policy":         "strict-origin-when-cross-origin",
				"Content-Security-Policy": "default-src 'self'",
				"Permissions-Policy":      "camera=()",
			},
			skipCSP: false,
		},
		{
			name: "API request includes security headers but not CSP",
			path: "/api/health",
			checkHeaders: map[string]string{
				"X-Content-Type-Options": "nosniff",
				"X-Frame-Options":        "DENY",
				"Referrer-Policy":        "strict-origin-when-cross-origin",
				"Permissions-Policy":     "camera=()",
			},
			skipCSP: true,
		},
		{
			name: "static asset request includes all headers",
			path: "/static/app.js",
			checkHeaders: map[string]string{
				"X-Content-Type-Options":  "nosniff",
				"X-Frame-Options":         "DENY",
				"Referrer-Policy":         "strict-origin-when-cross-origin",
				"Content-Security-Policy": "default-src 'self'",
			},
			skipCSP: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			middleware := SecurityHeaders(handler)
			middleware.ServeHTTP(rec, req)

			// Check required headers
			for header, expectedValue := range tt.checkHeaders {
				value := rec.Header().Get(header)
				assert.Contains(t, value, expectedValue,
					"Header %s should contain %s, got: %s", header, expectedValue, value)
			}

			// Verify CSP behavior
			csp := rec.Header().Get("Content-Security-Policy")
			if tt.skipCSP {
				assert.Empty(t, csp, "CSP should not be set for API requests")
			} else {
				assert.NotEmpty(t, csp, "CSP should be set for non-API requests")
			}
		})
	}
}

func TestSecurityHeaders_AllHeadersPresent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	middleware := SecurityHeaders(handler)
	middleware.ServeHTTP(rec, req)

	requiredHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Content-Security-Policy",
		"Permissions-Policy",
	}

	for _, header := range requiredHeaders {
		assert.NotEmpty(t, rec.Header().Get(header),
			"Required security header %s should be present", header)
	}
}

func TestSecurityHeaders_CSPDirectives(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	middleware := SecurityHeaders(handler)
	middleware.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")

	expectedDirectives := []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"img-src 'self' data: https:",
		"font-src 'self' data:",
		"connect-src 'self'",
	}

	for _, directive := range expectedDirectives {
		assert.Contains(t, csp, directive,
			"CSP should contain directive: %s", directive)
	}
}

func TestSecurityHeaders_PermissionsPolicy(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	middleware := SecurityHeaders(handler)
	middleware.ServeHTTP(rec, req)

	permissions := rec.Header().Get("Permissions-Policy")

	blockedFeatures := []string{
		"camera=()",
		"microphone=()",
		"geolocation=()",
		"payment=()",
	}

	for _, feature := range blockedFeatures {
		assert.Contains(t, permissions, feature,
			"Permissions-Policy should block: %s", feature)
	}
}

func TestSecurityHeaders_DoesNotBreakHandler(t *testing.T) {
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	middleware := SecurityHeaders(handler)
	middleware.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "Handler should be called")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test response", rec.Body.String())
}

func TestIsAPIRequest(t *testing.T) {
	tests := []struct {
		path  string
		isAPI bool
	}{
		{"/api/health", true},
		{"/api/sets", true},
		{"/api/analyze", true},
		{"/", false},
		{"/index.html", false},
		{"/static/app.js", false},
		{"/apidocs", false}, // Not /api/ prefix
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			result := isAPIRequest(req)
			assert.Equal(t, tt.isAPI, result,
				"isAPIRequest(%s) should be %v", tt.path, tt.isAPI)
		})
	}
}
