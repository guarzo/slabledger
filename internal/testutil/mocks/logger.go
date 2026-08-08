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

	// root is nil on the logger NewCapturingLogger returns and points at it on
	// every logger produced by With, so a chain of children all append to the
	// one entries slice the test holds. fields are the With fields inherited
	// down that chain, prepended to each call's own fields.
	root   *CapturingLogger
	fields []observability.Field
}

var _ observability.Logger = (*CapturingLogger)(nil)

// NewCapturingLogger creates a CapturingLogger with no recorded entries.
func NewCapturingLogger() *CapturingLogger {
	return &CapturingLogger{}
}

// sink returns the logger that owns the entries slice.
func (c *CapturingLogger) sink() *CapturingLogger {
	if c.root != nil {
		return c.root
	}
	return c
}

func (c *CapturingLogger) record(level, msg string, fields []observability.Field) {
	entry := LogEntry{Level: level, Message: msg, Fields: fields}
	if len(c.fields) > 0 {
		combined := make([]observability.Field, 0, len(c.fields)+len(fields))
		combined = append(combined, c.fields...)
		combined = append(combined, fields...)
		entry.Fields = combined
	}
	sink := c.sink()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.entries = append(sink.entries, entry)
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

// With returns a child logger that shares the parent's entries slice and
// carries the supplied fields, so a call made through the child records the
// inherited fields ahead of its own and Entries() on the original logger still
// sees everything. Chained With calls accumulate.
func (c *CapturingLogger) With(_ context.Context, fields ...observability.Field) observability.Logger {
	if len(fields) == 0 {
		return c
	}
	inherited := make([]observability.Field, 0, len(c.fields)+len(fields))
	inherited = append(inherited, c.fields...)
	inherited = append(inherited, fields...)
	return &CapturingLogger{root: c.sink(), fields: inherited}
}

// Entries returns a copy of every log call recorded so far.
func (c *CapturingLogger) Entries() []LogEntry {
	sink := c.sink()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	out := make([]LogEntry, len(sink.entries))
	copy(out, sink.entries)
	return out
}
