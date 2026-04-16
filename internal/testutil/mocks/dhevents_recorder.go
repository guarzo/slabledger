package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
)

// MockEventRecorder implements dhevents.Recorder with Fn-field pattern.
type MockEventRecorder struct {
	RecordFn func(ctx context.Context, e dhevents.Event) error
	Events   []dhevents.Event
}

func (m *MockEventRecorder) Record(ctx context.Context, e dhevents.Event) error {
	m.Events = append(m.Events, e)
	if m.RecordFn != nil {
		return m.RecordFn(ctx, e)
	}
	return nil
}
