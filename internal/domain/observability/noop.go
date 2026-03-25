package observability

import (
	"context"
)

// NoopLogger is a no-op implementation of Logger for use when logging is not needed.
// Unlike test mocks, this is safe to use in production code.
type NoopLogger struct{}

// NewNoopLogger creates a new no-op logger that silently discards all log messages.
func NewNoopLogger() Logger {
	return &NoopLogger{}
}

func (n *NoopLogger) Debug(ctx context.Context, msg string, fields ...Field) {}
func (n *NoopLogger) Info(ctx context.Context, msg string, fields ...Field)  {}
func (n *NoopLogger) Warn(ctx context.Context, msg string, fields ...Field)  {}
func (n *NoopLogger) Error(ctx context.Context, msg string, fields ...Field) {}
func (n *NoopLogger) With(ctx context.Context, fields ...Field) Logger {
	return n
}
