package mocks

import (
	"context"
	"sync"

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

// LogEntry is one captured log call.
type LogEntry struct {
	Level   string
	Message string
	Fields  []observability.Field
}

// FindField returns the value of the first field in the entry whose Key
// matches, and whether one was found.
func (e LogEntry) FindField(key string) (any, bool) {
	for _, f := range e.Fields {
		if f.Key == key {
			return f.Value, true
		}
	}
	return nil, false
}

// CapturingLogger is a test double for observability.Logger that records
// every call instead of discarding it, so tests can assert on log output.
// It is safe for concurrent use.
type CapturingLogger struct {
	mu      sync.Mutex
	entries []LogEntry
}

var _ observability.Logger = (*CapturingLogger)(nil)

// NewCapturingLogger creates a CapturingLogger with no recorded entries.
func NewCapturingLogger() *CapturingLogger {
	return &CapturingLogger{}
}

func (c *CapturingLogger) record(level, msg string, fields []observability.Field) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, LogEntry{Level: level, Message: msg, Fields: fields})
}

func (c *CapturingLogger) Debug(_ context.Context, msg string, fields ...observability.Field) {
	c.record("debug", msg, fields)
}

func (c *CapturingLogger) Info(_ context.Context, msg string, fields ...observability.Field) {
	c.record("info", msg, fields)
}

func (c *CapturingLogger) Warn(_ context.Context, msg string, fields ...observability.Field) {
	c.record("warn", msg, fields)
}

func (c *CapturingLogger) Error(_ context.Context, msg string, fields ...observability.Field) {
	c.record("error", msg, fields)
}

// With returns the same logger so that fields captured via chained loggers
// still land in the one Entries() slice callers inspect. Production Loggers
// return a child logger that prepends fields to every subsequent call; a
// test double has no such need since assertions read fields directly off
// each LogEntry.
func (c *CapturingLogger) With(_ context.Context, _ ...observability.Field) observability.Logger {
	return c
}

// Entries returns a copy of every log call recorded so far.
func (c *CapturingLogger) Entries() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]LogEntry, len(c.entries))
	copy(out, c.entries)
	return out
}
