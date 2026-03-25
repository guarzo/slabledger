package telemetry

import (
	"context"
	"log/slog"
	"os"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SlogLogger implements observability.Logger using standard library slog
type SlogLogger struct {
	logger *slog.Logger
}

// Compile-time interface check
var _ observability.Logger = (*SlogLogger)(nil)

// NewSlogLogger creates a new logger instance
func NewSlogLogger(level slog.Level, format string) *SlogLogger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: false, // Can be enabled for debugging
	}

	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return &SlogLogger{logger: slog.New(handler)}
}

// Debug logs a debug message
func (l *SlogLogger) Debug(ctx context.Context, msg string, fields ...observability.Field) {
	l.logger.DebugContext(ctx, msg, l.fieldsToAttrs(fields)...)
}

// Info logs an info message
func (l *SlogLogger) Info(ctx context.Context, msg string, fields ...observability.Field) {
	l.logger.InfoContext(ctx, msg, l.fieldsToAttrs(fields)...)
}

// Warn logs a warning message
func (l *SlogLogger) Warn(ctx context.Context, msg string, fields ...observability.Field) {
	l.logger.WarnContext(ctx, msg, l.fieldsToAttrs(fields)...)
}

// Error logs an error message
func (l *SlogLogger) Error(ctx context.Context, msg string, fields ...observability.Field) {
	l.logger.ErrorContext(ctx, msg, l.fieldsToAttrs(fields)...)
}

// With creates a child logger with additional fields
func (l *SlogLogger) With(ctx context.Context, fields ...observability.Field) observability.Logger {
	return &SlogLogger{logger: l.logger.With(l.fieldsToAttrs(fields)...)}
}

// fieldsToAttrs converts observability fields to slog attributes
func (l *SlogLogger) fieldsToAttrs(fields []observability.Field) []any {
	attrs := make([]any, 0, len(fields)*2)
	for _, f := range fields {
		attrs = append(attrs, f.Key, f.Value)
	}
	return attrs
}
