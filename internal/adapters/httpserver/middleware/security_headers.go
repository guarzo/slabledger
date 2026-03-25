package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders adds standard HTTP security headers to all responses
// This middleware implements defense-in-depth security measures
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME sniffing attacks
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking attacks
		w.Header().Set("X-Frame-Options", "DENY")

		// Referrer policy - only send origin when crossing origins
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy for web UI
		// Only apply CSP to non-API requests to avoid breaking API clients
		if !isAPIRequest(r) {
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self'; "+
					"style-src 'self'; "+
					"img-src 'self' data: https:; "+
					"font-src 'self' data:; "+
					"connect-src 'self'")
		}

		// Permissions Policy - restrict access to browser features
		w.Header().Set("Permissions-Policy",
			"camera=(), "+
				"microphone=(), "+
				"geolocation=(), "+
				"payment=()")

		next.ServeHTTP(w, r)
	})
}

// isAPIRequest checks if the request is for an API endpoint
func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}
