package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// defaultConfig returns sensible defaults for tests.
func defaultConfig() Config {
	return Config{
		RefreshInterval:    1 * time.Hour,
		BatchSize:          50,
		BatchDelay:         1 * time.Second,
		MaxBurstCalls:      10,
		MaxCallsPerHour:    50,
		BurstPauseDuration: 30 * time.Second,
		Enabled:            true,
	}
}

// TODO(Task 4): Re-enable and rewrite scheduler tests once refreshBatch is
// reimplemented as a purchase-driven refresh (stale_prices VIEW was dropped in
// migration 000038).

func TestPriceRefreshScheduler_Health(t *testing.T) {
	logger := mocks.NewMockLogger()

	t.Run("healthy", func(t *testing.T) {
		tracker := &mocks.MockDBTracker{}
		provider := mocks.NewMockSimplePriceProvider(true)
		config := defaultConfig()

		scheduler := NewPriceRefreshScheduler(&mocks.MockRefreshCandidateProvider{}, tracker, tracker, provider, logger, config)
		ctx := context.Background()

		err := scheduler.Health(ctx)
		require.NoError(t, err, "health check should pass when all components are healthy")
	})

	t.Run("disabled", func(t *testing.T) {
		tracker := &mocks.MockDBTracker{}
		provider := mocks.NewMockSimplePriceProvider(true)
		config := defaultConfig()
		config.Enabled = false

		scheduler := NewPriceRefreshScheduler(&mocks.MockRefreshCandidateProvider{}, tracker, tracker, provider, logger, config)
		ctx := context.Background()

		err := scheduler.Health(ctx)
		require.NoError(t, err, "health check should pass when scheduler is disabled (intentionally inactive)")
	})

	t.Run("provider unavailable", func(t *testing.T) {
		tracker := &mocks.MockDBTracker{}
		provider := mocks.NewMockSimplePriceProvider(false)
		config := defaultConfig()

		scheduler := NewPriceRefreshScheduler(&mocks.MockRefreshCandidateProvider{}, tracker, tracker, provider, logger, config)
		ctx := context.Background()

		err := scheduler.Health(ctx)
		require.Error(t, err, "health check should fail when provider is unavailable")
	})
}

func TestPriceRefreshScheduler_DisabledScheduler(t *testing.T) {
	logger := mocks.NewMockLogger()
	tracker := &mocks.MockDBTracker{}
	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval: 1 * time.Hour,
		BatchSize:       10,
		BatchDelay:      10 * time.Millisecond,
		MaxBurstCalls:   10,
		MaxCallsPerHour: 50,
		Enabled:         false,
	}

	scheduler := NewPriceRefreshScheduler(&mocks.MockRefreshCandidateProvider{}, tracker, tracker, provider, logger, config)
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success - scheduler returned immediately
	case <-time.After(100 * time.Millisecond):
		t.Fatal("disabled scheduler did not return immediately")
	}

	require.Equal(t, 0, provider.CallCount(), "should not refresh prices when scheduler is disabled")
}

func TestPriceRefreshScheduler_Stop(t *testing.T) {
	logger := mocks.NewMockLogger()
	tracker := &mocks.MockDBTracker{}
	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval:    100 * time.Millisecond,
		BatchSize:          10,
		BatchDelay:         1 * time.Millisecond,
		MaxBurstCalls:      10,
		MaxCallsPerHour:    50,
		BurstPauseDuration: 1 * time.Millisecond,
		Enabled:            true,
	}

	scheduler := NewPriceRefreshScheduler(&mocks.MockRefreshCandidateProvider{}, tracker, tracker, provider, logger, config)
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	scheduler.Stop()

	select {
	case <-done:
		// Success - scheduler stopped
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler did not stop within timeout via Stop()")
	}
}

func TestPriceRefreshScheduler_ContextCancellation(t *testing.T) {
	logger := mocks.NewMockLogger()
	tracker := &mocks.MockDBTracker{}
	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval:    100 * time.Millisecond,
		BatchSize:          10,
		BatchDelay:         1 * time.Millisecond,
		MaxBurstCalls:      10,
		MaxCallsPerHour:    50,
		BurstPauseDuration: 1 * time.Millisecond,
		Enabled:            true,
	}

	scheduler := NewPriceRefreshScheduler(&mocks.MockRefreshCandidateProvider{}, tracker, tracker, provider, logger, config)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success - scheduler stopped
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler did not stop within timeout")
	}
}

func TestPriceRefreshScheduler_StopIdempotent(t *testing.T) {
	logger := mocks.NewMockLogger()
	tracker := &mocks.MockDBTracker{}
	provider := mocks.NewMockSimplePriceProvider(true)
	config := defaultConfig()
	config.Enabled = false

	scheduler := NewPriceRefreshScheduler(&mocks.MockRefreshCandidateProvider{}, tracker, tracker, provider, logger, config)

	scheduler.Stop()
	scheduler.Stop()
	scheduler.Stop()
}

func TestConfig_DefaultConfig(t *testing.T) {
	config := defaultConfig()

	require.Equal(t, 1*time.Hour, config.RefreshInterval)
	require.Equal(t, 50, config.BatchSize)
	require.Equal(t, 1*time.Second, config.BatchDelay)
	require.Equal(t, 10, config.MaxBurstCalls)
	require.Equal(t, 30*time.Second, config.BurstPauseDuration)
	require.True(t, config.Enabled)
}

func TestConfig_DefaultConfig_Values(t *testing.T) {
	config := defaultConfig()

	require.Equal(t, 50, config.MaxCallsPerHour)
	require.Equal(t, 30*time.Second, config.BurstPauseDuration)
}
