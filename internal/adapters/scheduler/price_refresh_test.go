package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/pricing"
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

func TestPriceRefreshScheduler_RefreshBatch(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{
		StalePrices: []pricing.StalePrice{
			{CardName: "Charizard", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo", HoursOld: 25},
			{CardName: "Blastoise", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo", HoursOld: 30},
		},
	}

	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval: 1 * time.Hour,
		BatchSize:       10,
		BatchDelay:      10 * time.Millisecond, // Fast for testing
		MaxBurstCalls:   10,
		MaxCallsPerHour: 50,
		Enabled:         true,
	}

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
	ctx := context.Background()

	// Run one batch
	scheduler.refreshBatch(ctx)

	// Verify prices were refreshed
	require.Equal(t, 2, provider.CallCount(), "should refresh 2 prices")
}

func TestPriceRefreshScheduler_SkipsBlockedProvider(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{
		StalePrices: []pricing.StalePrice{
			{CardName: "Pikachu", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo", HoursOld: 25},
		},
		BlockedUntil: time.Now().Add(1 * time.Hour), // Provider blocked
	}

	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval: 1 * time.Hour,
		BatchSize:       10,
		BatchDelay:      10 * time.Millisecond,
		MaxBurstCalls:   10,
		MaxCallsPerHour: 50,
		Enabled:         true,
	}

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
	ctx := context.Background()

	// Run one batch
	scheduler.refreshBatch(ctx)

	// Verify no prices were refreshed due to blocked provider
	require.Equal(t, 0, provider.CallCount(), "should not refresh prices when provider is blocked")
}

func TestPriceRefreshScheduler_RespectsRateLimit(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{
		StalePrices: []pricing.StalePrice{
			{CardName: "Mewtwo", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo", HoursOld: 25},
		},
		APIUsage: &pricing.APIUsageStats{
			Provider:      "doubleholo",
			CallsLastHour: 60, // Over limit
		},
	}

	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval: 1 * time.Hour,
		BatchSize:       10,
		BatchDelay:      10 * time.Millisecond,
		MaxBurstCalls:   10,
		MaxCallsPerHour: 50,
		Enabled:         true,
	}

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
	ctx := context.Background()

	// Run one batch
	scheduler.refreshBatch(ctx)

	// Verify no prices were refreshed due to rate limit
	require.Equal(t, 0, provider.CallCount(), "should not refresh prices when rate limit is reached")
}

func TestPriceRefreshScheduler_NoStalePrices(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{
		StalePrices: []pricing.StalePrice{}, // No stale prices
	}

	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval: 1 * time.Hour,
		BatchSize:       10,
		BatchDelay:      10 * time.Millisecond,
		MaxBurstCalls:   10,
		MaxCallsPerHour: 50,
		Enabled:         true,
	}

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
	ctx := context.Background()

	// Run one batch
	scheduler.refreshBatch(ctx)

	// Verify no API calls were made
	require.Equal(t, 0, provider.CallCount(), "should not make API calls when no stale prices")
}

func TestPriceRefreshScheduler_ContextCancellation(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{
		StalePrices: []pricing.StalePrice{
			{CardName: "Charizard", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo", HoursOld: 25},
			{CardName: "Blastoise", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo", HoursOld: 30},
		},
	}

	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval: 100 * time.Millisecond,
		BatchSize:       10,
		BatchDelay:      10 * time.Millisecond,
		MaxBurstCalls:   10,
		MaxCallsPerHour: 50,
		Enabled:         true,
	}

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
	ctx, cancel := context.WithCancel(context.Background())

	// Start scheduler in background
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Let it run for a bit
	time.Sleep(50 * time.Millisecond)

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

func TestPriceRefreshScheduler_DisabledScheduler(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{
		StalePrices: []pricing.StalePrice{
			{CardName: "Charizard", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo", HoursOld: 25},
		},
	}

	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval: 1 * time.Hour,
		BatchSize:       10,
		BatchDelay:      10 * time.Millisecond,
		MaxBurstCalls:   10,
		MaxCallsPerHour: 50,
		Enabled:         false, // Disabled
	}

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
	ctx := context.Background()

	// Start scheduler
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Scheduler should return immediately when disabled
	select {
	case <-done:
		// Success - scheduler returned immediately
	case <-time.After(100 * time.Millisecond):
		t.Fatal("disabled scheduler did not return immediately")
	}

	// Verify no prices were refreshed
	require.Equal(t, 0, provider.CallCount(), "should not refresh prices when scheduler is disabled")
}

func TestPriceRefreshScheduler_GroupByProvider(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{}
	provider := mocks.NewMockSimplePriceProvider(true)
	config := defaultConfig()

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)

	stalePrices := []pricing.StalePrice{
		{CardName: "Charizard", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo"},
		{CardName: "Blastoise", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo"},
		{CardName: "Pikachu", SetName: "Base Set", Grade: "PSA 10", Source: "tcgplayer"},
	}

	grouped := scheduler.groupByProvider(stalePrices)

	require.Len(t, grouped, 2, "should group into 2 providers")
	require.Len(t, grouped["doubleholo"], 2, "should have 2 doubleholo entries")
	require.Len(t, grouped["tcgplayer"], 1, "should have 1 tcgplayer entry")
}

func TestPriceRefreshScheduler_Health(t *testing.T) {
	logger := mocks.NewMockLogger()

	t.Run("healthy", func(t *testing.T) {
		repo := &mocks.MockPriceRepository{}
		provider := mocks.NewMockSimplePriceProvider(true)
		config := defaultConfig()

		scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
		ctx := context.Background()

		err := scheduler.Health(ctx)
		require.NoError(t, err, "health check should pass when all components are healthy")
	})

	t.Run("disabled", func(t *testing.T) {
		repo := &mocks.MockPriceRepository{}
		provider := mocks.NewMockSimplePriceProvider(true)
		config := defaultConfig()
		config.Enabled = false

		scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
		ctx := context.Background()

		err := scheduler.Health(ctx)
		// Disabled scheduler is intentionally inactive, not unhealthy
		// Health check should pass (no error) when scheduler is disabled
		require.NoError(t, err, "health check should pass when scheduler is disabled (intentionally inactive)")
	})

	t.Run("provider unavailable", func(t *testing.T) {
		repo := &mocks.MockPriceRepository{}
		provider := mocks.NewMockSimplePriceProvider(false)
		config := defaultConfig()

		scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
		ctx := context.Background()

		err := scheduler.Health(ctx)
		require.Error(t, err, "health check should fail when provider is unavailable")
	})
}

func TestConfig_DefaultConfig(t *testing.T) {
	config := defaultConfig()

	require.Equal(t, 1*time.Hour, config.RefreshInterval)
	require.Equal(t, 50, config.BatchSize)
	require.Equal(t, 1*time.Second, config.BatchDelay)
	require.Equal(t, 10, config.MaxBurstCalls)
	require.Equal(t, 30*time.Second, config.BurstPauseDuration) // New: was hardcoded 5 minutes
	require.True(t, config.Enabled)
}

func TestConfig_DefaultConfig_Values(t *testing.T) {
	config := defaultConfig()

	// Verify all default values are set correctly
	require.Equal(t, 50, config.MaxCallsPerHour)
	require.Equal(t, 30*time.Second, config.BurstPauseDuration)
}

func TestPriceRefreshScheduler_Stop(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{
		StalePrices: []pricing.StalePrice{
			{CardName: "Charizard", SetName: "Base Set", Grade: "PSA 10", Source: "doubleholo", HoursOld: 25},
		},
	}

	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval:    1 * time.Hour, // Long interval
		BatchSize:          10,
		BatchDelay:         10 * time.Millisecond,
		MaxBurstCalls:      10,
		MaxCallsPerHour:    50,
		BurstPauseDuration: 30 * time.Second,
		Enabled:            true,
	}

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
	ctx := context.Background()

	// Start scheduler in background
	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Let it run initial batch
	time.Sleep(50 * time.Millisecond)

	// Stop via Stop() method
	scheduler.Stop()

	// Wait for scheduler to stop
	select {
	case <-done:
		// Success - scheduler stopped
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler did not stop within timeout via Stop()")
	}
}

func TestPriceRefreshScheduler_BurstCounterSkipsDuplicates(t *testing.T) {
	logger := mocks.NewMockLogger()

	// Create 15 entries: 5 unique cards, each with 3 grades (= 3 duplicate rows each)
	var stalePrices []pricing.StalePrice
	cards := []string{"Charizard", "Blastoise", "Venusaur", "Pikachu", "Mewtwo"}
	grades := []string{"PSA 10", "PSA 9", "PSA 8"}
	for _, card := range cards {
		for _, grade := range grades {
			stalePrices = append(stalePrices, pricing.StalePrice{
				CardName: card,
				SetName:  "Base Set",
				Grade:    grade,
				Source:   "doubleholo",
				HoursOld: 25,
			})
		}
	}

	repo := &mocks.MockPriceRepository{
		StalePrices: stalePrices,
	}

	provider := mocks.NewMockSimplePriceProvider(true)

	config := Config{
		RefreshInterval:    1 * time.Hour,
		BatchSize:          50,
		BatchDelay:         1 * time.Millisecond,
		MaxBurstCalls:      3, // Burst after 3 actual API calls
		BurstPauseDuration: 1 * time.Millisecond,
		MaxCallsPerHour:    100,
		Enabled:            true,
	}

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)
	ctx := context.Background()

	scheduler.refreshBatch(ctx)

	// Only 5 unique cards should have made API calls (10 duplicates skipped)
	require.Equal(t, 5, provider.CallCount(), "should only make API calls for 5 unique cards, not 15")
}

func TestPriceRefreshScheduler_StopIdempotent(t *testing.T) {
	logger := mocks.NewMockLogger()

	repo := &mocks.MockPriceRepository{}
	provider := mocks.NewMockSimplePriceProvider(true)
	config := defaultConfig()
	config.Enabled = false // Disabled so Start returns immediately

	scheduler := NewPriceRefreshScheduler(repo, repo, repo, provider, logger, config)

	// Multiple calls to Stop should be safe (idempotent)
	scheduler.Stop()
	scheduler.Stop() // Should not panic
	scheduler.Stop() // Should not panic
}
