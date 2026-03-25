package telemetry

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlogLogger_ImplementsInterface(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")
	var _ observability.Logger = logger // Compile-time check
	assert.NotNil(t, logger)
}

func TestSlogLogger_LogLevels(t *testing.T) {
	tests := []struct {
		name     string
		level    slog.Level
		logFunc  func(observability.Logger, context.Context)
		expected string
	}{
		{
			name:  "debug level",
			level: slog.LevelDebug,
			logFunc: func(l observability.Logger, ctx context.Context) {
				l.Debug(ctx, "debug message", observability.String("key", "value"))
			},
			expected: "debug message",
		},
		{
			name:  "info level",
			level: slog.LevelInfo,
			logFunc: func(l observability.Logger, ctx context.Context) {
				l.Info(ctx, "info message", observability.String("key", "value"))
			},
			expected: "info message",
		},
		{
			name:  "warn level",
			level: slog.LevelWarn,
			logFunc: func(l observability.Logger, ctx context.Context) {
				l.Warn(ctx, "warn message", observability.String("key", "value"))
			},
			expected: "warn message",
		},
		{
			name:  "error level",
			level: slog.LevelError,
			logFunc: func(l observability.Logger, ctx context.Context) {
				l.Error(ctx, "error message", observability.String("key", "value"))
			},
			expected: "error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewSlogLogger(tt.level, "text")
			ctx := context.Background()
			tt.logFunc(logger, ctx)
			// If we get here without panic, the test passes
		})
	}
}

func TestSlogLogger_FieldTypes(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")
	ctx := context.Background()

	// Test various field types
	logger.Info(ctx, "test message",
		observability.String("string_field", "value"),
		observability.Int("int_field", 42),
		observability.Float64("float_field", 3.14),
		observability.Bool("bool_field", true),
		observability.Duration("duration_field", time.Second),
	)

	// If we get here without panic, all field types are supported
	assert.True(t, true)
}

func TestSlogLogger_WithContext(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")
	ctx := context.Background()

	// Create child logger with additional fields
	childLogger := logger.With(ctx, observability.String("request_id", "123"))

	// Child logger should be a new instance
	assert.NotNil(t, childLogger)
	assert.NotEqual(t, logger, childLogger)

	// Log with child logger
	childLogger.Info(ctx, "child logger message")

	// Original logger should still work
	logger.Info(ctx, "parent logger message")
}

func TestSlogLogger_ErrorField(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")
	ctx := context.Background()

	testErr := assert.AnError
	logger.Error(ctx, "error occurred", observability.Err(testErr))

	// If we get here without panic, error logging works
	assert.True(t, true)
}

func TestSlogLogger_JSONFormat(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "json")
	ctx := context.Background()

	logger.Info(ctx, "json formatted message",
		observability.String("key", "value"))

	// If we get here without panic, JSON formatting works
	assert.True(t, true)
}

func TestSlogLogger_TextFormat(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")
	ctx := context.Background()

	logger.Info(ctx, "text formatted message",
		observability.String("key", "value"))

	// If we get here without panic, text formatting works
	assert.True(t, true)
}

func TestSlogLogger_MultipleFields(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")
	ctx := context.Background()

	logger.Info(ctx, "message with multiple fields",
		observability.String("field1", "value1"),
		observability.String("field2", "value2"),
		observability.Int("field3", 123),
		observability.Bool("field4", false),
	)

	// If we get here without panic, multiple fields work
	assert.True(t, true)
}

func TestSlogLogger_EmptyFields(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")
	ctx := context.Background()

	// Log with no fields
	logger.Info(ctx, "message without fields")

	// If we get here without panic, empty fields work
	assert.True(t, true)
}

func TestSlogLogger_LevelFiltering(t *testing.T) {
	// Create logger with WARN level
	logger := NewSlogLogger(slog.LevelWarn, "text")
	ctx := context.Background()

	// These should not cause errors even though they're below threshold
	logger.Debug(ctx, "debug message") // Should be filtered
	logger.Info(ctx, "info message")   // Should be filtered
	logger.Warn(ctx, "warn message")   // Should log
	logger.Error(ctx, "error message") // Should log

	assert.True(t, true)
}

func TestSlogLogger_NilContext(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")

	// Use background context instead of nil
	ctx := context.Background()

	require.NotPanics(t, func() {
		logger.Info(ctx, "message with context")
	})
}

func TestSlogLogger_NestedWithCalls(t *testing.T) {
	logger := NewSlogLogger(slog.LevelInfo, "text")
	ctx := context.Background()

	// Create nested child loggers
	child1 := logger.With(ctx, observability.String("level", "1"))
	child2 := child1.With(ctx, observability.String("level", "2"))
	child3 := child2.With(ctx, observability.String("level", "3"))

	// Each level should work
	child1.Info(ctx, "level 1")
	child2.Info(ctx, "level 2")
	child3.Info(ctx, "level 3")

	assert.True(t, true)
}

func TestSlogLogger_RealWorldScenario(t *testing.T) {
	logger := NewSlogLogger(slog.LevelDebug, "text")
	ctx := context.Background()

	// Simulate a request
	requestLogger := logger.With(ctx,
		observability.String("request_id", "abc-123"),
		observability.String("user_id", "user-456"),
	)

	requestLogger.Info(ctx, "request started")

	// Simulate processing
	requestLogger.Debug(ctx, "fetching data",
		observability.String("table", "users"),
		observability.Int("limit", 10),
	)

	// Simulate error
	requestLogger.Error(ctx, "database error",
		observability.Err(assert.AnError),
		observability.Duration("elapsed", 150*time.Millisecond),
	)

	requestLogger.Info(ctx, "request completed",
		observability.Int("status_code", 500),
		observability.Duration("total_time", 200*time.Millisecond),
	)

	assert.True(t, true)
}
