package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// requestIDKey uses the shared contextKey type from auth.go
const requestIDKey contextKey = "request_id"

// responseWriterGuard wraps http.ResponseWriter to track write state
type responseWriterGuard struct {
	http.ResponseWriter
	written bool
}

func (rw *responseWriterGuard) WriteHeader(code int) {
	if !rw.written {
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriterGuard) Write(b []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// RecoveryMiddleware recovers from panics and returns 500 errors with structured logging.
// It prevents double header writes and captures full stack traces.
func RecoveryMiddleware(logger observability.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap response writer with guard
			guard := &responseWriterGuard{ResponseWriter: w}

			defer func() {
				if err := recover(); err != nil {
					// Get request ID from context if available
					requestID := ""
					if id := r.Context().Value(requestIDKey); id != nil {
						if str, ok := id.(string); ok {
							requestID = str
						}
					}

					// Structured panic logging
					logger.Error(r.Context(), "panic recovered",
						observability.String("method", r.Method),
						observability.String("path", r.URL.Path),
						observability.String("remote_addr", r.RemoteAddr),
						observability.String("request_id", requestID),
						observability.Field{Key: "panic", Value: err},
						observability.Field{Key: "stack", Value: string(debug.Stack())})

					// Prevent double write
					if !guard.written {
						http.Error(guard, "Internal Server Error", http.StatusInternalServerError)
					}
				}
			}()

			next.ServeHTTP(guard, r)
		})
	}
}
