package mocks

import (
	"context"
	"time"
)

// PSASnapshotStoreMock is a test double for psaportal.SnapshotStore.
type PSASnapshotStoreMock struct {
	CurrentSnapshotFn func(ctx context.Context) ([]map[string]string, time.Time, error)
}

func (m *PSASnapshotStoreMock) CurrentSnapshot(ctx context.Context) ([]map[string]string, time.Time, error) {
	if m.CurrentSnapshotFn != nil {
		return m.CurrentSnapshotFn(ctx)
	}
	return nil, time.Time{}, nil
}

// PSASnapshotWriterMock is a test double for psaportal.SnapshotWriter.
// It records the last saved rows/fetchedAt so tests can assert on them.
type PSASnapshotWriterMock struct {
	SaveSnapshotFn func(ctx context.Context, rows []map[string]string, fetchedAt time.Time) error

	SavedRows      []map[string]string
	SavedFetchedAt time.Time
}

func (m *PSASnapshotWriterMock) SaveSnapshot(ctx context.Context, rows []map[string]string, fetchedAt time.Time) error {
	m.SavedRows = rows
	m.SavedFetchedAt = fetchedAt
	if m.SaveSnapshotFn != nil {
		return m.SaveSnapshotFn(ctx, rows, fetchedAt)
	}
	return nil
}
