package mocks

import (
	"context"
	"sync"
	"sync/atomic"
)

// MockSnapshotEnrichService is a test double for scheduler.SnapshotEnrichService.
// Each method delegates to a function field, allowing per-test configuration.
// Thread-safe atomic counters track call counts; mutex-protected fields track
// the last limit argument seen by each method.
//
// Example:
//
//	svc := &MockSnapshotEnrichService{
//	    ProcessPendingSnapshotsFn: func(_ context.Context, limit int) (int, int, int, error) {
//	        return 3, 1, 0, nil
//	    },
//	}
type MockSnapshotEnrichService struct {
	ProcessPendingSnapshotsFn func(ctx context.Context, limit int) (int, int, int, error)
	RetryFailedSnapshotsFn    func(ctx context.Context, limit int) (int, int, int, error)

	// Return values used when the corresponding Fn field is nil.
	PendingProcessed int
	PendingSkipped   int
	PendingFailed    int
	RetryProcessed   int
	RetrySkipped     int
	RetryFailed      int

	mu               sync.Mutex
	pendingLimitSeen int
	retryLimitSeen   int
	pendingCallCount int32
	retryCallCount   int32
}

// ProcessPendingSnapshots implements scheduler.SnapshotEnrichService.
func (m *MockSnapshotEnrichService) ProcessPendingSnapshots(ctx context.Context, limit int) (int, int, int, error) {
	atomic.AddInt32(&m.pendingCallCount, 1)
	m.mu.Lock()
	m.pendingLimitSeen = limit
	m.mu.Unlock()
	if m.ProcessPendingSnapshotsFn != nil {
		return m.ProcessPendingSnapshotsFn(ctx, limit)
	}
	return m.PendingProcessed, m.PendingSkipped, m.PendingFailed, nil
}

// RetryFailedSnapshots implements scheduler.SnapshotEnrichService.
func (m *MockSnapshotEnrichService) RetryFailedSnapshots(ctx context.Context, limit int) (int, int, int, error) {
	atomic.AddInt32(&m.retryCallCount, 1)
	m.mu.Lock()
	m.retryLimitSeen = limit
	m.mu.Unlock()
	if m.RetryFailedSnapshotsFn != nil {
		return m.RetryFailedSnapshotsFn(ctx, limit)
	}
	return m.RetryProcessed, m.RetrySkipped, m.RetryFailed, nil
}

// PendingCallCount returns the number of times ProcessPendingSnapshots was called.
func (m *MockSnapshotEnrichService) PendingCallCount() int32 {
	return atomic.LoadInt32(&m.pendingCallCount)
}

// RetryCallCount returns the number of times RetryFailedSnapshots was called.
func (m *MockSnapshotEnrichService) RetryCallCount() int32 {
	return atomic.LoadInt32(&m.retryCallCount)
}

// PendingLimitSeen returns the last limit argument passed to ProcessPendingSnapshots.
func (m *MockSnapshotEnrichService) PendingLimitSeen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pendingLimitSeen
}

// RetryLimitSeen returns the last limit argument passed to RetryFailedSnapshots.
func (m *MockSnapshotEnrichService) RetryLimitSeen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.retryLimitSeen
}
