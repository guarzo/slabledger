package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// mockSessionJanitor implements SessionJanitor — the two-method seam this
// scheduler depends on — for testing.
type mockSessionJanitor struct {
	cleanupCount int
	cleanupErr   error
	callCount    int32

	stateCleanupCount int
	stateCleanupErr   error
	stateCallCount    int32
}

var _ SessionJanitor = (*mockSessionJanitor)(nil)

func (m *mockSessionJanitor) CleanupExpiredSessions(ctx context.Context) (int, error) {
	atomic.AddInt32(&m.callCount, 1)
	return m.cleanupCount, m.cleanupErr
}

func (m *mockSessionJanitor) CleanupExpiredOAuthStates(ctx context.Context) (int, error) {
	atomic.AddInt32(&m.stateCallCount, 1)
	return m.stateCleanupCount, m.stateCleanupErr
}

func (m *mockSessionJanitor) CallCount() int32 {
	return atomic.LoadInt32(&m.callCount)
}

func (m *mockSessionJanitor) StateCallCount() int32 {
	return atomic.LoadInt32(&m.stateCallCount)
}

func TestSessionCleanupScheduler_Cleanup(t *testing.T) {
	mock := &mockSessionJanitor{cleanupCount: 5}
	logger := mocks.NewMockLogger()

	config := SessionCleanupConfig{
		Enabled:  true,
		Interval: 100 * time.Millisecond,
	}

	scheduler := NewSessionCleanupScheduler(mock, logger, config)

	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	go scheduler.Start(ctx)

	// Wait for context to expire
	<-ctx.Done()

	// Wait for scheduler to have run expected iterations using Eventually
	require.Eventually(t, func() bool {
		return mock.CallCount() >= 3
	}, 200*time.Millisecond, 10*time.Millisecond, "expected at least 3 cleanup calls, got %d", mock.CallCount())
}

func TestSessionCleanupScheduler_Stop(t *testing.T) {
	mock := &mockSessionJanitor{}
	logger := mocks.NewMockLogger()

	config := SessionCleanupConfig{
		Enabled:  true,
		Interval: 1 * time.Hour, // Long interval
	}

	scheduler := NewSessionCleanupScheduler(mock, logger, config)

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait for initial cleanup to run using Eventually
	require.Eventually(t, func() bool {
		return mock.CallCount() >= 1
	}, 200*time.Millisecond, 10*time.Millisecond, "initial cleanup should run")

	scheduler.Stop()

	// Wait for scheduler to stop
	select {
	case <-done:
		// Success - scheduler stopped
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler did not stop within timeout")
	}

	// Should have only run once (initial cleanup)
	if mock.CallCount() != 1 {
		t.Errorf("expected 1 cleanup call after stop, got %d", mock.CallCount())
	}
}

func TestSessionCleanupScheduler_CleanupError(t *testing.T) {
	mock := &mockSessionJanitor{
		cleanupErr: errors.New("database error"),
	}
	logger := mocks.NewMockLogger()

	config := SessionCleanupConfig{
		Enabled:  true,
		Interval: 100 * time.Millisecond,
	}

	scheduler := NewSessionCleanupScheduler(mock, logger, config)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	go scheduler.Start(ctx)

	// Wait for context to expire
	<-ctx.Done()

	// Wait for cleanup to have been called using Eventually
	require.Eventually(t, func() bool {
		return mock.CallCount() >= 1
	}, 200*time.Millisecond, 10*time.Millisecond, "cleanup should be called at least once")
}

// The OAuth-state sweep is the only thing that removes rows left by abandoned
// logins — ConsumeOAuthState only fires on a successful callback.
func TestSessionCleanupScheduler_CleansExpiredOAuthStates(t *testing.T) {
	mock := &mockSessionJanitor{cleanupCount: 2, stateCleanupCount: 7}
	scheduler := NewSessionCleanupScheduler(mock, mocks.NewMockLogger(), SessionCleanupConfig{
		Enabled:  true,
		Interval: 1 * time.Hour,
	})

	scheduler.cleanup(context.Background())

	require.Equal(t, int32(1), mock.CallCount(), "session cleanup should run once")
	require.Equal(t, int32(1), mock.StateCallCount(), "oauth state cleanup should run once")
}

// The two tables are independent, so a failing session sweep must not skip the
// OAuth-state sweep.
func TestSessionCleanupScheduler_OAuthStateCleanupRunsAfterSessionError(t *testing.T) {
	mock := &mockSessionJanitor{cleanupErr: errors.New("database error")}
	scheduler := NewSessionCleanupScheduler(mock, mocks.NewMockLogger(), SessionCleanupConfig{
		Enabled:  true,
		Interval: 1 * time.Hour,
	})

	scheduler.cleanup(context.Background())

	require.Equal(t, int32(1), mock.StateCallCount(),
		"oauth state cleanup should run even when session cleanup fails")
}

func TestSessionCleanupScheduler_OAuthStateCleanupError(t *testing.T) {
	mock := &mockSessionJanitor{stateCleanupErr: errors.New("database error")}
	scheduler := NewSessionCleanupScheduler(mock, mocks.NewMockLogger(), SessionCleanupConfig{
		Enabled:  true,
		Interval: 1 * time.Hour,
	})

	// Errors are logged, not propagated — the loop must survive them.
	scheduler.cleanup(context.Background())

	require.Equal(t, int32(1), mock.StateCallCount())
}

func TestSessionCleanupScheduler_DefaultInterval(t *testing.T) {
	mock := &mockSessionJanitor{}
	logger := mocks.NewMockLogger()

	// Config with zero interval should use default
	config := SessionCleanupConfig{
		Enabled:  true,
		Interval: 0,
	}

	scheduler := NewSessionCleanupScheduler(mock, logger, config)

	// Verify default interval is applied
	require.Equal(t, 1*time.Hour, scheduler.interval, "should use default 1 hour interval")
}

func TestSessionCleanupScheduler_ContextCancellation(t *testing.T) {
	mock := &mockSessionJanitor{cleanupCount: 1}
	logger := mocks.NewMockLogger()

	config := SessionCleanupConfig{
		Enabled:  true,
		Interval: 100 * time.Millisecond,
	}

	scheduler := NewSessionCleanupScheduler(mock, logger, config)
	ctx, cancel := context.WithCancel(context.Background())

	// Start scheduler in background
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait for initial cleanup to run using Eventually
	require.Eventually(t, func() bool {
		return mock.CallCount() >= 1
	}, 200*time.Millisecond, 10*time.Millisecond, "initial cleanup should run")

	// Cancel context
	cancel()

	// Wait for scheduler to stop
	select {
	case <-done:
		// Success - scheduler stopped
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler did not stop within timeout")
	}
}

func TestDefaultSessionCleanupConfig(t *testing.T) {
	config := DefaultSessionCleanupConfig()

	require.True(t, config.Enabled, "should be enabled by default")
	require.Equal(t, 1*time.Hour, config.Interval, "should have 1 hour default interval")
}

// TestSessionCleanupScheduler_ConcurrentCalls tests that the mutex prevents concurrent cleanup
func TestSessionCleanupScheduler_ConcurrentCalls(t *testing.T) {
	var concurrentCalls int32
	var maxConcurrent int32
	var totalCalls int32

	mock := &mockSessionJanitor{}
	logger := mocks.NewMockLogger()

	config := SessionCleanupConfig{
		Enabled:  true,
		Interval: 10 * time.Millisecond, // Very short interval
	}

	scheduler := NewSessionCleanupScheduler(mock, logger, config)

	// Use a custom mock that tracks concurrent calls
	trackingMock := &concurrentTrackingMock{
		concurrentCalls: &concurrentCalls,
		maxConcurrent:   &maxConcurrent,
		totalCalls:      &totalCalls,
	}

	scheduler.authService = trackingMock

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Start scheduler
	go scheduler.Start(ctx)

	// Wait for context to expire
	<-ctx.Done()

	// Wait for at least one cleanup to complete (handles in-flight operations)
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&totalCalls) >= 1
	}, 200*time.Millisecond, 10*time.Millisecond,
		"expected at least one cleanup call to execute, got %d", atomic.LoadInt32(&totalCalls))

	// Verify at least one cleanup ran (prevents vacuous pass)
	require.Greater(t, atomic.LoadInt32(&totalCalls), int32(0),
		"test requires at least one cleanup to have executed")

	// The mutex should ensure maxConcurrent never exceeds 1
	require.LessOrEqual(t, atomic.LoadInt32(&maxConcurrent), int32(1),
		"mutex should prevent concurrent cleanup calls")
}

// concurrentTrackingMock tracks concurrent calls to CleanupExpiredSessions
type concurrentTrackingMock struct {
	mockSessionJanitor
	concurrentCalls *int32
	maxConcurrent   *int32
	totalCalls      *int32
}

func (m *concurrentTrackingMock) CleanupExpiredSessions(ctx context.Context) (int, error) {
	// Track total calls
	atomic.AddInt32(m.totalCalls, 1)

	// Track concurrent calls
	current := atomic.AddInt32(m.concurrentCalls, 1)
	defer atomic.AddInt32(m.concurrentCalls, -1)

	// Update max concurrent
	for {
		max := atomic.LoadInt32(m.maxConcurrent)
		if current <= max {
			break
		}
		if atomic.CompareAndSwapInt32(m.maxConcurrent, max, current) {
			break
		}
	}

	// Simulate some work while respecting context cancellation
	select {
	case <-time.After(5 * time.Millisecond):
		return 0, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
