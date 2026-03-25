package middleware

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// corsMaxAge is the preflight cache duration in seconds.
const corsMaxAge = "3600"

// CORSMiddleware adds CORS headers to all responses with origin validation
// Restricts to localhost origins by default for security
func CORSMiddleware(logger observability.Logger, next http.Handler) http.Handler {
	// Default allowed origins (localhost only for single-user deployment)
	allowedOrigins := []string{
		"http://localhost:8081",
		"http://127.0.0.1:8081",
	}

	// Allow additional origins via environment variable for custom deployments
	if customOrigin := os.Getenv("ALLOWED_ORIGIN"); customOrigin != "" {
		for _, origin := range strings.Split(customOrigin, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				allowedOrigins = append(allowedOrigins, origin)
			}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// No Origin header = same-origin request, skip CORS headers entirely
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if this is a same-origin request (origin matches the host)
		// Some browsers (like Brave) send Origin headers even for same-origin requests
		isSameOrigin := false
		if host := r.Host; host != "" {
			// Extract host from origin (strip protocol)
			originHost := origin
			if idx := strings.Index(origin, "://"); idx != -1 {
				originHost = origin[idx+3:]
			}
			// Remove port from comparison if present in one but not the other
			originHostNoPort := strings.Split(originHost, ":")[0]
			hostNoPort := strings.Split(host, ":")[0]
			if originHostNoPort == hostNoPort {
				isSameOrigin = true
			}
		}

		// Check if origin is allowed
		allowed := isSameOrigin
		if !allowed {
			for _, allowedOrigin := range allowedOrigins {
				if origin == allowedOrigin {
					allowed = true
					break
				}
			}
		}

		// Reject requests from disallowed origins
		if !allowed {
			logger.Warn(context.Background(), "CORS request from disallowed origin",
				observability.String("origin", origin),
				observability.String("remote_addr", r.RemoteAddr))
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		// Set CORS headers only for allowed cross-origin requests
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", corsMaxAge)

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
