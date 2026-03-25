package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

var _ pricing.APITracker = (*MockAPITracker)(nil)

// MockAPITracker implements pricing.APITracker for testing.
type MockAPITracker struct {
	mu            sync.Mutex
	RecordedCalls []pricing.APICallRecord
}

func (m *MockAPITracker) RecordAPICall(_ context.Context, call *pricing.APICallRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RecordedCalls = append(m.RecordedCalls, *call)
	return nil
}

// GetRecordedCalls returns a copy of the recorded calls (thread-safe).
func (m *MockAPITracker) GetRecordedCalls() []pricing.APICallRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]pricing.APICallRecord, len(m.RecordedCalls))
	copy(cp, m.RecordedCalls)
	return cp
}

func (m *MockAPITracker) GetAPIUsage(_ context.Context, provider string) (*pricing.APIUsageStats, error) {
	return &pricing.APIUsageStats{Provider: provider}, nil
}

func (m *MockAPITracker) UpdateRateLimit(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (m *MockAPITracker) IsProviderBlocked(_ context.Context, _ string) (bool, time.Time, error) {
	return false, time.Time{}, nil
}
