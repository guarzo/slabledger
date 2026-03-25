package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// MockLogger is a test double for observability.Logger.
// It silently discards all log messages, like the production NoopLogger,
// but lives in the test mocks package so tests don't depend on production types.
type MockLogger struct{}

var _ observability.Logger = (*MockLogger)(nil)

// NewMockLogger creates a new test logger that discards all output.
func NewMockLogger() observability.Logger {
	return &MockLogger{}
}

func (m *MockLogger) Debug(_ context.Context, _ string, _ ...observability.Field) {}
func (m *MockLogger) Info(_ context.Context, _ string, _ ...observability.Field)  {}
func (m *MockLogger) Warn(_ context.Context, _ string, _ ...observability.Field)  {}
func (m *MockLogger) Error(_ context.Context, _ string, _ ...observability.Field) {}
func (m *MockLogger) With(_ context.Context, _ ...observability.Field) observability.Logger {
	return m
}
