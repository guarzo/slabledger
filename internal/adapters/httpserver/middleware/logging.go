package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// validRequestID matches alphanumeric, hyphens, and underscores (1-64 chars).
var validRequestID = regexp.MustCompile(`^[a-zA-Z0-9\-_]{1,64}$`)

// fallbackCounter provides a monotonically-incrementing counter for collision-resistant
// request ID generation when crypto/rand is unavailable.
var fallbackCounter uint64

// LoggingMiddleware logs HTTP requests with structured logging, request ID injection,
// and correlation ID propagation.
//
// Features:
//   - Generates unique request ID for each request
//   - Propagates correlation ID from X-Correlation-ID header
//   - Structured logging with method, path, status, duration
//   - Injects request ID into response headers
//   - Stores request ID in context for downstream use
func LoggingMiddleware(logger observability.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Generate or extract request ID; accept inbound value only if it
			// matches a safe pattern to prevent log injection.
			requestID := r.Header.Get("X-Request-ID")
			if !validRequestID.MatchString(requestID) {
				requestID = generateRequestID()
			}

			// Extract correlation ID if provided
			correlationID := r.Header.Get("X-Correlation-ID")
			if correlationID == "" {
				correlationID = requestID // Use request ID as correlation ID if not provided
			}

			// Store request ID in context for downstream use
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			r = r.WithContext(ctx)

			// Inject request ID and correlation ID into response headers
			w.Header().Set("X-Request-ID", requestID)
			w.Header().Set("X-Correlation-ID", correlationID)

			// Wrap the response writer to capture status code
			wrapped := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Structured request logging
			duration := time.Since(start)

			fields := []observability.Field{
				observability.String("method", r.Method),
				observability.String("path", r.URL.Path),
				observability.String("remote_addr", r.RemoteAddr),
				observability.String("user_agent", r.UserAgent()),
				observability.String("request_id", requestID),
				observability.Int("status", wrapped.statusCode),
				observability.Duration("duration", duration),
				observability.Int64("duration_ms", duration.Milliseconds()),
			}

			// Add correlation ID if different from request ID
			if correlationID != requestID {
				fields = append(fields, observability.String("correlation_id", correlationID))
			}

			logger.Info(ctx, "http request", fields...)
		})
	}
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *loggingResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher by delegating to the underlying ResponseWriter
// if it supports flushing. This is required for SSE (Server-Sent Events) to work
// correctly when the logging middleware wraps the response writer.
func (rw *loggingResponseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// generateRequestID creates a unique request ID using crypto/rand
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to collision-resistant ID combining timestamp, counter, and PID
		return fmt.Sprintf("%x-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&fallbackCounter, 1), os.Getpid())
	}
	return hex.EncodeToString(b)
}
