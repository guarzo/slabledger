package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/scoring"
)

var _ scoring.GapStore = (*MockGapStore)(nil)

type MockGapStore struct {
	mu             sync.Mutex
	RecordGapsFn   func(ctx context.Context, gaps []scoring.GapRecord) error
	GetGapReportFn func(ctx context.Context, since time.Time) (*scoring.GapReport, error)
	PruneOldGapsFn func(ctx context.Context, olderThan time.Time) (int64, error)

	recordedGaps []scoring.GapRecord
}

func NewMockGapStore() *MockGapStore {
	return &MockGapStore{}
}

// RecordedGaps returns a snapshot of recorded gaps (safe for concurrent access).
func (m *MockGapStore) RecordedGaps() []scoring.GapRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]scoring.GapRecord, len(m.recordedGaps))
	copy(out, m.recordedGaps)
	return out
}

func (m *MockGapStore) RecordGaps(ctx context.Context, gaps []scoring.GapRecord) error {
	if m.RecordGapsFn != nil {
		return m.RecordGapsFn(ctx, gaps)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedGaps = append(m.recordedGaps, gaps...)
	return nil
}

func (m *MockGapStore) GetGapReport(ctx context.Context, since time.Time) (*scoring.GapReport, error) {
	if m.GetGapReportFn != nil {
		return m.GetGapReportFn(ctx, since)
	}
	return &scoring.GapReport{Period: "7d"}, nil
}

func (m *MockGapStore) PruneOldGaps(ctx context.Context, olderThan time.Time) (int64, error) {
	if m.PruneOldGapsFn != nil {
		return m.PruneOldGapsFn(ctx, olderThan)
	}
	return 0, nil
}
