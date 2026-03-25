package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/platform/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoveryMiddleware_NoPanic(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := RecoveryMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String())
}

func TestRecoveryMiddleware_WithPanic(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := RecoveryMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic
	require.NotPanics(t, func() {
		handler.ServeHTTP(rec, req)
	})

	// Should return 500
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Internal Server Error")
}

func TestRecoveryMiddleware_WithRequestID(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := RecoveryMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic with request id")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	// Add request ID to context
	ctx := context.WithValue(req.Context(), requestIDKey, "test-request-id-123")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should return 500
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRecoveryMiddleware_PreventDoubleWrite(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := RecoveryMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write headers first
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial response"))
		// Then panic
		panic("panic after write")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should have the first status code (200), not 500
	// because headers were already written
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "partial response")
}

func TestRecoveryMiddleware_StackTrace(t *testing.T) {
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "text")

	handler := RecoveryMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("stack trace test")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should return 500
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestResponseWriterGuard_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	guard := &responseWriterGuard{ResponseWriter: rec}

	// First write should work
	guard.WriteHeader(http.StatusOK)
	assert.True(t, guard.written)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second write should be ignored
	guard.WriteHeader(http.StatusInternalServerError)
	assert.Equal(t, http.StatusOK, rec.Code, "Status should not change after second WriteHeader")
}

func TestResponseWriterGuard_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	guard := &responseWriterGuard{ResponseWriter: rec}

	// Write should mark as written
	guard.Write([]byte("test"))
	assert.True(t, guard.written)
	assert.Equal(t, "test", rec.Body.String())
}
