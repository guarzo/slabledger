package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// mockAuthService implements auth.Service for testing
type mockAuthService struct {
	cleanupCount int
	cleanupErr   error
	callCount    int32
}

func (m *mockAuthService) CleanupExpiredSessions(ctx context.Context) (int, error) {
	atomic.AddInt32(&m.callCount, 1)
	return m.cleanupCount, m.cleanupErr
}

func (m *mockAuthService) CallCount() int32 {
	return atomic.LoadInt32(&m.callCount)
}

// Implement other interface methods as no-ops
func (m *mockAuthService) GetLoginURL(state string) string {
	return ""
}
func (m *mockAuthService) ExchangeCodeForTokens(ctx context.Context, code string) (*auth.UserTokens, error) {
	return nil, nil
}
func (m *mockAuthService) GetUserInfo(ctx context.Context, accessToken string) (*auth.UserInfo, error) {
	return nil, nil
}
func (m *mockAuthService) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	return nil
}
func (m *mockAuthService) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	return true, nil
}
func (m *mockAuthService) CreateSession(ctx context.Context, userID int64, userAgent, ipAddress string) (*auth.Session, error) {
	return nil, nil
}
func (m *mockAuthService) ValidateSession(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
	return nil, nil, nil
}
func (m *mockAuthService) DeleteSession(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockAuthService) GetOrCreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	return nil, nil
}
func (m *mockAuthService) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	return nil, nil
}
func (m *mockAuthService) UpdateLastLogin(ctx context.Context, userID int64) error {
	return nil
}
func (m *mockAuthService) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	return nil
}

func (m *mockAuthService) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	return false, nil
}

func (m *mockAuthService) ListAllowedEmails(ctx context.Context) ([]auth.AllowedEmail, error) {
	return nil, nil
}

func (m *mockAuthService) AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error {
	return nil
}

func (m *mockAuthService) RemoveAllowedEmail(ctx context.Context, email string) error {
	return nil
}

func (m *mockAuthService) ListUsers(ctx context.Context) ([]auth.User, error) {
	return nil, nil
}

func (m *mockAuthService) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	return nil
}

func TestSessionCleanupScheduler_Cleanup(t *testing.T) {
	mock := &mockAuthService{cleanupCount: 5}
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
	mock := &mockAuthService{}
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
	mock := &mockAuthService{
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

func TestSessionCleanupScheduler_DefaultInterval(t *testing.T) {
	mock := &mockAuthService{}
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
	mock := &mockAuthService{cleanupCount: 1}
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

	mock := &mockAuthService{}
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
	mockAuthService
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
