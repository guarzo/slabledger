package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// mockPriceRepository embeds mocks.MockDBTracker and overrides
// CleanupOldAccessLogs and Ping with custom test-specific behavior.
type mockPriceRepository struct {
	mocks.MockDBTracker
	cleanupCalls       int
	cleanupReturnCount int64
	cleanupReturnError error
	lastRetentionDays  int
	pingError          error
}

func (m *mockPriceRepository) CleanupOldAccessLogs(_ context.Context, retentionDays int) (int64, error) {
	m.cleanupCalls++
	m.lastRetentionDays = retentionDays
	return m.cleanupReturnCount, m.cleanupReturnError
}

func (m *mockPriceRepository) Ping(_ context.Context) error {
	return m.pingError
}

func TestDefaultAccessLogCleanupConfig(t *testing.T) {
	config := DefaultAccessLogCleanupConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if config.Interval != 24*time.Hour {
		t.Errorf("expected Interval to be 24h, got %v", config.Interval)
	}
	if config.RetentionDays != 30 {
		t.Errorf("expected RetentionDays to be 30, got %d", config.RetentionDays)
	}
}

func TestNewAccessLogCleanupScheduler(t *testing.T) {
	mockRepo := &mockPriceRepository{}
	mockLog := mocks.NewMockLogger()

	config := AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      1 * time.Hour,
		RetentionDays: 60,
	}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)
	if scheduler.interval != 1*time.Hour {
		t.Errorf("expected interval to be 1h, got %v", scheduler.interval)
	}
	if scheduler.retentionDays != 60 {
		t.Errorf("expected retentionDays to be 60, got %d", scheduler.retentionDays)
	}
}

func TestNewAccessLogCleanupScheduler_DefaultValues(t *testing.T) {
	mockRepo := &mockPriceRepository{}
	mockLog := mocks.NewMockLogger()

	// Zero values should get defaults
	config := AccessLogCleanupConfig{}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	if scheduler.interval != 24*time.Hour {
		t.Errorf("expected interval default to be 24h, got %v", scheduler.interval)
	}
	if scheduler.retentionDays != 30 {
		t.Errorf("expected retentionDays default to be 30, got %d", scheduler.retentionDays)
	}
}

func TestAccessLogCleanupScheduler_RunOnce(t *testing.T) {
	mockRepo := &mockPriceRepository{
		cleanupReturnCount: 42,
	}
	mockLog := mocks.NewMockLogger()

	config := AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      24 * time.Hour,
		RetentionDays: 30,
	}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	ctx := context.Background()
	count, err := scheduler.RunOnce(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if count != 42 {
		t.Errorf("expected count to be 42, got %d", count)
	}
	if mockRepo.cleanupCalls != 1 {
		t.Errorf("expected cleanupCalls to be 1, got %d", mockRepo.cleanupCalls)
	}
	if mockRepo.lastRetentionDays != 30 {
		t.Errorf("expected lastRetentionDays to be 30, got %d", mockRepo.lastRetentionDays)
	}
}

func TestAccessLogCleanupScheduler_RunOnce_Error(t *testing.T) {
	expectedErr := errors.New("database connection failed")
	mockRepo := &mockPriceRepository{
		cleanupReturnError: expectedErr,
	}
	mockLog := mocks.NewMockLogger()

	config := AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      24 * time.Hour,
		RetentionDays: 30,
	}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	ctx := context.Background()
	count, err := scheduler.RunOnce(ctx)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if count != 0 {
		t.Errorf("expected count to be 0 on error, got %d", count)
	}
	if mockRepo.cleanupCalls != 1 {
		t.Errorf("expected cleanupCalls to be 1, got %d", mockRepo.cleanupCalls)
	}
	if mockRepo.lastRetentionDays != 30 {
		t.Errorf("expected lastRetentionDays to be 30, got %d", mockRepo.lastRetentionDays)
	}
}

func TestAccessLogCleanupScheduler_Cleanup(t *testing.T) {
	mockRepo := &mockPriceRepository{
		cleanupReturnCount: 100,
	}
	mockLog := mocks.NewMockLogger()

	config := AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      24 * time.Hour,
		RetentionDays: 45,
	}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	ctx := context.Background()
	_, err := scheduler.RunOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected error from RunOnce: %v", err)
	}

	if mockRepo.cleanupCalls != 1 {
		t.Errorf("expected cleanupCalls to be 1, got %d", mockRepo.cleanupCalls)
	}
	if mockRepo.lastRetentionDays != 45 {
		t.Errorf("expected retention days to be 45, got %d", mockRepo.lastRetentionDays)
	}
}

func TestAccessLogCleanupScheduler_Stop(t *testing.T) {
	mockRepo := &mockPriceRepository{}
	mockLog := mocks.NewMockLogger()

	config := DefaultAccessLogCleanupConfig()
	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	// Calling Stop should not panic
	scheduler.Stop()
}

// threadSafeMockRepo is a thread-safe version of mockPriceRepository for concurrent tests
type threadSafeMockRepo struct {
	mockPriceRepository
	mu               sync.Mutex
	cleanupCallCount int
	cleanupTimes     []time.Time
}

func (m *threadSafeMockRepo) CleanupOldAccessLogs(ctx context.Context, retentionDays int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupCallCount++
	m.cleanupTimes = append(m.cleanupTimes, time.Now())
	return m.cleanupReturnCount, m.cleanupReturnError
}

func (m *threadSafeMockRepo) getCleanupCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cleanupCallCount
}

func (m *threadSafeMockRepo) getCleanupTimes() []time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]time.Time, len(m.cleanupTimes))
	copy(result, m.cleanupTimes)
	return result
}

func TestAccessLogCleanupScheduler_FullLifecycle(t *testing.T) {
	mockRepo := &threadSafeMockRepo{
		mockPriceRepository: mockPriceRepository{
			cleanupReturnCount: 5,
		},
	}
	mockLog := mocks.NewMockLogger()

	// Use short interval for faster test
	config := AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      50 * time.Millisecond,
		RetentionDays: 30,
	}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	ctx := context.Background()

	// Start scheduler in background with done channel
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait for at least 2 cleanup calls using Eventually (initial + at least 1 interval)
	require.Eventually(t, func() bool {
		return mockRepo.getCleanupCallCount() >= 2
	}, 500*time.Millisecond, 10*time.Millisecond, "expected at least 2 cleanup calls")

	// Stop the scheduler
	scheduler.Stop()

	// Wait for goroutine to exit using select with timeout
	select {
	case <-done:
		// Scheduler stopped successfully
	case <-time.After(200 * time.Millisecond):
		t.Fatal("scheduler did not stop within timeout")
	}
}

func TestAccessLogCleanupScheduler_StopIdempotency(t *testing.T) {
	mockRepo := &threadSafeMockRepo{
		mockPriceRepository: mockPriceRepository{
			cleanupReturnCount: 1,
		},
	}
	mockLog := mocks.NewMockLogger()

	config := AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      50 * time.Millisecond,
		RetentionDays: 30,
	}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	ctx := context.Background()

	// Start scheduler in background with done channel
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait for at least one cleanup using Eventually
	require.Eventually(t, func() bool {
		return mockRepo.getCleanupCallCount() >= 1
	}, 200*time.Millisecond, 10*time.Millisecond, "expected at least 1 cleanup call")

	// First stop
	scheduler.Stop()

	// Wait for goroutine to exit using select with timeout
	select {
	case <-done:
		// Scheduler stopped successfully
	case <-time.After(200 * time.Millisecond):
		t.Fatal("scheduler did not stop within timeout")
	}

	// Record count after first stop
	countAfterFirstStop := mockRepo.getCleanupCallCount()

	// Verify no additional cleanup calls occur after stop using polling-based assertion
	require.Never(t, func() bool {
		return mockRepo.getCleanupCallCount() > countAfterFirstStop
	}, 100*time.Millisecond, 10*time.Millisecond, "cleanup calls should not increase after Stop")

	// Calling Stop again should be safe (idempotent) - should not panic
	scheduler.Stop()
}

func TestAccessLogCleanupScheduler_StopActuallyStops(t *testing.T) {
	mockRepo := &threadSafeMockRepo{
		mockPriceRepository: mockPriceRepository{
			cleanupReturnCount: 1,
		},
	}
	mockLog := mocks.NewMockLogger()

	config := AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      30 * time.Millisecond,
		RetentionDays: 30,
	}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	ctx := context.Background()

	// Start scheduler in background with done channel
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait for at least one cleanup using Eventually
	require.Eventually(t, func() bool {
		return mockRepo.getCleanupCallCount() >= 1
	}, 200*time.Millisecond, 10*time.Millisecond, "expected at least 1 cleanup call")

	// Stop the scheduler
	scheduler.Stop()

	// Wait for goroutine to exit using select with timeout
	select {
	case <-done:
		// Scheduler stopped successfully
	case <-time.After(200 * time.Millisecond):
		t.Fatal("scheduler did not stop within timeout")
	}

	// Record the time of last cleanup
	timesBeforeWait := mockRepo.getCleanupTimes()
	if len(timesBeforeWait) == 0 {
		t.Fatal("expected at least one cleanup to have occurred")
	}
	countBeforeWait := len(timesBeforeWait)

	// Verify no new cleanup timestamps appear using polling-based assertion
	require.Never(t, func() bool {
		return len(mockRepo.getCleanupTimes()) > countBeforeWait
	}, 100*time.Millisecond, 10*time.Millisecond, "new cleanup calls should not occur after Stop")
}

func TestAccessLogCleanupScheduler_ContextCancellation(t *testing.T) {
	mockRepo := &threadSafeMockRepo{
		mockPriceRepository: mockPriceRepository{
			cleanupReturnCount: 1,
		},
	}
	mockLog := mocks.NewMockLogger()

	config := AccessLogCleanupConfig{
		Enabled:       true,
		Interval:      30 * time.Millisecond,
		RetentionDays: 30,
	}

	scheduler := NewAccessLogCleanupScheduler(mockRepo, mockLog, config)

	ctx, cancel := context.WithCancel(context.Background())

	// Start scheduler in background
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait for at least one cleanup using Eventually
	require.Eventually(t, func() bool {
		return mockRepo.getCleanupCallCount() >= 1
	}, 200*time.Millisecond, 10*time.Millisecond, "expected at least 1 cleanup call")

	// Cancel context instead of calling Stop
	cancel()

	// Verify the goroutine exits
	select {
	case <-done:
		// Success - scheduler exited
	case <-time.After(500 * time.Millisecond):
		t.Error("scheduler did not exit after context cancellation")
	}

	// Record count after cancellation
	countAfterCancel := mockRepo.getCleanupCallCount()

	// Verify no additional cleanup calls occur after context cancellation using polling-based assertion
	require.Never(t, func() bool {
		return mockRepo.getCleanupCallCount() > countAfterCancel
	}, 100*time.Millisecond, 10*time.Millisecond, "cleanup calls should not increase after context cancellation")
}
