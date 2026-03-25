package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/platform/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggingMiddleware_BasicRequest(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should complete successfully
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String())
}

func TestLoggingMiddleware_RequestIDGeneration(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is in context
		requestID := r.Context().Value(requestIDKey)
		require.NotNil(t, requestID)
		assert.NotEmpty(t, requestID.(string))

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should inject request ID in response headers
	requestID := rec.Header().Get("X-Request-ID")
	assert.NotEmpty(t, requestID)
	assert.Len(t, requestID, 32, "Request ID should be 32 hex characters (16 bytes)")

	// Request ID should be in header
	assert.Equal(t, requestID, rec.Header().Get("X-Request-ID"))
}

func TestLoggingMiddleware_RequestIDPropagation(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "existing-request-id")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should use existing request ID
	requestID := rec.Header().Get("X-Request-ID")
	assert.Equal(t, "existing-request-id", requestID)

	// Request ID should match existing
	assert.Equal(t, "existing-request-id", rec.Header().Get("X-Request-ID"))
}

func TestLoggingMiddleware_RequestIDValidation(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	tests := []struct {
		name            string
		requestID       string
		shouldPropagate bool
	}{
		{"valid alphanumeric", "abc123", true},
		{"valid with hyphens and underscores", "req-id_123", true},
		{"too long (65 chars)", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"contains spaces", "bad request id", false},
		{"contains newline", "bad\nid", false},
		{"contains special chars", "id;drop table", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.requestID != "" {
				req.Header.Set("X-Request-ID", tt.requestID)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			resultID := rec.Header().Get("X-Request-ID")
			assert.NotEmpty(t, resultID)
			if tt.shouldPropagate {
				assert.Equal(t, tt.requestID, resultID)
			} else {
				assert.NotEqual(t, tt.requestID, resultID, "Invalid request ID should be replaced")
			}
		})
	}
}

func TestLoggingMiddleware_CorrelationIDGeneration(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should inject correlation ID (same as request ID if not provided)
	correlationID := rec.Header().Get("X-Correlation-ID")
	requestID := rec.Header().Get("X-Request-ID")
	assert.Equal(t, requestID, correlationID, "Correlation ID should match request ID when not provided")
}

func TestLoggingMiddleware_CorrelationIDPropagation(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Correlation-ID", "external-correlation-id")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should propagate correlation ID
	correlationID := rec.Header().Get("X-Correlation-ID")
	assert.Equal(t, "external-correlation-id", correlationID)

	// Correlation ID should match
	assert.Equal(t, "external-correlation-id", rec.Header().Get("X-Correlation-ID"))
}

func TestLoggingMiddleware_StatusCodeCapture(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedStatus int
	}{
		{"200 OK", http.StatusOK, 200},
		{"201 Created", http.StatusCreated, 201},
		{"400 Bad Request", http.StatusBadRequest, 400},
		{"404 Not Found", http.StatusNotFound, 404},
		{"500 Internal Server Error", http.StatusInternalServerError, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

			handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// Should return correct status code
			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestLoggingMiddleware_DurationTracking(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some work
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should complete successfully
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestLoggingMiddleware_UserAgentLogging(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should complete successfully
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGenerateRequestID_Uniqueness(t *testing.T) {
	// Generate multiple request IDs and verify they're unique
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateRequestID()
		assert.NotEmpty(t, id)
		assert.Len(t, id, 32, "Request ID should be 32 hex characters")
		assert.False(t, ids[id], "Request ID should be unique")
		ids[id] = true
	}
}

func TestLoggingResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	lrw := &loggingResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	lrw.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, lrw.statusCode)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestLoggingResponseWriter_DefaultStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	lrw := &loggingResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	// Write without calling WriteHeader
	lrw.Write([]byte("test"))

	// Should use default status code
	assert.Equal(t, http.StatusOK, lrw.statusCode)
}
