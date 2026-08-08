package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
	require.Equal(t, 50, config.MaxCallsPerHour)
	require.True(t, config.Enabled)
}

// batchConfig tunes defaultConfig for direct refreshBatch tests: the inter-call
// and burst delays are zeroed so a multi-card batch runs without wall-clock sleeps.
func batchConfig() Config {
	config := defaultConfig()
	config.BatchDelay = 0
	config.BurstPauseDuration = 0
	config.MaxBurstCalls = 0
	return config
}

// candidates builds n distinct refresh candidates.
func candidates(n int) []pricing.RefreshCandidate {
	out := make([]pricing.RefreshCandidate, 0, n)
	for i := range n {
		out = append(out, pricing.RefreshCandidate{
			CardName:   fmt.Sprintf("Charizard Holo %d", i),
			CardNumber: strconv.Itoa(i + 1),
			SetName:    "Base Set",
			Grade:      10,
		})
	}
	return out
}

// consecutiveFailures reads the scheduler's failure counter under its lock.
func consecutiveFailures(s *PriceRefreshScheduler) int {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.consecutiveFailures
}

func newBatchScheduler(
	candidateProvider pricing.RefreshCandidateProvider,
	tracker *mocks.MockDBTracker,
	provider pricing.PriceProvider,
	config Config,
) *PriceRefreshScheduler {
	return NewPriceRefreshScheduler(candidateProvider, tracker, tracker, provider, mocks.NewMockLogger(), config)
}

// TestPriceRefreshScheduler_RefreshBatch_Skips covers the guard branches that
// abandon a batch before any pricing call is made.
func TestPriceRefreshScheduler_RefreshBatch_Skips(t *testing.T) {
	blockErr := errors.New("block lookup failed")
	usageErr := errors.New("usage lookup failed")

	tests := []struct {
		name              string
		providerAvailable bool
		tracker           *mocks.MockDBTracker
		config            Config
		wantCandidateCall int
	}{
		{
			name:              "price provider unavailable short-circuits before the candidate fetch",
			providerAvailable: false,
			tracker:           &mocks.MockDBTracker{},
			config:            batchConfig(),
			wantCandidateCall: 0,
		},
		{
			name:              "provider blocked",
			providerAvailable: true,
			tracker:           &mocks.MockDBTracker{BlockedUntil: time.Now().Add(1 * time.Hour)},
			config:            batchConfig(),
			wantCandidateCall: 1,
		},
		{
			name:              "block-status lookup fails",
			providerAvailable: true,
			tracker: &mocks.MockDBTracker{
				IsProviderBlockedFn: func(context.Context, string) (bool, time.Time, error) {
					return false, time.Time{}, blockErr
				},
			},
			config:            batchConfig(),
			wantCandidateCall: 1,
		},
		{
			name:              "API usage lookup fails",
			providerAvailable: true,
			tracker: &mocks.MockDBTracker{
				GetAPIUsageFn: func(context.Context, string) (*pricing.APIUsageStats, error) {
					return nil, usageErr
				},
			},
			config:            batchConfig(),
			wantCandidateCall: 1,
		},
		{
			name:              "hourly rate limit already reached",
			providerAvailable: true,
			tracker:           &mocks.MockDBTracker{APIUsage: &pricing.APIUsageStats{CallsLastHour: 50}},
			config:            batchConfig(),
			wantCandidateCall: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidateProvider := &mocks.MockRefreshCandidateProvider{Candidates: candidates(3)}
			provider := mocks.NewMockSimplePriceProvider(tt.providerAvailable)
			scheduler := newBatchScheduler(candidateProvider, tt.tracker, provider, tt.config)

			scheduler.refreshBatch(context.Background())

			require.Equal(t, 0, provider.CallCount(), "batch should be skipped before any pricing call")
			require.Equal(t, tt.wantCandidateCall, candidateProvider.CallCount())
			require.Zero(t, consecutiveFailures(scheduler), "a skip is not a failure")
			require.True(t, scheduler.LastFailureAt().IsZero(), "a skip must not record a failure time")
		})
	}
}

// TestPriceRefreshScheduler_RefreshBatch_Budget covers how the hourly budget
// bounds the number of pricing calls within a single batch.
func TestPriceRefreshScheduler_RefreshBatch_Budget(t *testing.T) {
	tests := []struct {
		name            string
		maxCallsPerHour int
		callsLastHour   int64
		candidateCount  int
		wantCalls       int
	}{
		{
			name:            "remaining budget caps the batch mid-way",
			maxCallsPerHour: 10,
			callsLastHour:   8,
			candidateCount:  5,
			wantCalls:       2,
		},
		{
			name:            "budget larger than the batch prices every candidate",
			maxCallsPerHour: 50,
			callsLastHour:   0,
			candidateCount:  4,
			wantCalls:       4,
		},
		{
			name:            "zero MaxCallsPerHour means unlimited",
			maxCallsPerHour: 0,
			callsLastHour:   9000,
			candidateCount:  4,
			wantCalls:       4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := batchConfig()
			config.MaxCallsPerHour = tt.maxCallsPerHour

			candidateProvider := &mocks.MockRefreshCandidateProvider{Candidates: candidates(tt.candidateCount)}
			tracker := &mocks.MockDBTracker{APIUsage: &pricing.APIUsageStats{CallsLastHour: tt.callsLastHour}}
			provider := mocks.NewMockSimplePriceProvider(true)
			scheduler := newBatchScheduler(candidateProvider, tracker, provider, config)

			scheduler.refreshBatch(context.Background())

			require.Equal(t, tt.wantCalls, provider.CallCount())
			require.Zero(t, consecutiveFailures(scheduler))
		})
	}
}

func TestPriceRefreshScheduler_RefreshBatch_CandidateFetchError(t *testing.T) {
	fetchErr := errors.New("candidate query failed")
	candidateProvider := &mocks.MockRefreshCandidateProvider{Err: fetchErr}
	provider := mocks.NewMockSimplePriceProvider(true)
	scheduler := newBatchScheduler(candidateProvider, &mocks.MockDBTracker{}, provider, batchConfig())

	scheduler.refreshBatch(context.Background())
	require.Equal(t, 1, consecutiveFailures(scheduler))
	require.False(t, scheduler.LastFailureAt().IsZero())

	// Consecutive failures accumulate across batches.
	scheduler.refreshBatch(context.Background())
	require.Equal(t, 2, consecutiveFailures(scheduler))
	require.Equal(t, 0, provider.CallCount(), "no pricing calls once the candidate fetch fails")

	// A batch that fetches candidates successfully clears the counter.
	candidateProvider.Err = nil
	candidateProvider.Candidates = candidates(2)
	scheduler.refreshBatch(context.Background())
	require.Zero(t, consecutiveFailures(scheduler))
	require.False(t, scheduler.LastFailureAt().IsZero(), "recovery preserves the last failure time")
}

func TestPriceRefreshScheduler_RefreshBatch_EmptyCandidates(t *testing.T) {
	candidateProvider := &mocks.MockRefreshCandidateProvider{}
	provider := mocks.NewMockSimplePriceProvider(true)
	scheduler := newBatchScheduler(candidateProvider, &mocks.MockDBTracker{}, provider, batchConfig())

	scheduler.refreshBatch(context.Background())

	require.Equal(t, 1, candidateProvider.CallCount())
	require.Equal(t, 0, provider.CallCount(), "an empty candidate set is a no-op")
	require.Zero(t, consecutiveFailures(scheduler))
	require.True(t, scheduler.LastFailureAt().IsZero())
}

func TestPriceRefreshScheduler_RefreshBatch_PricingErrorsCountAsFailure(t *testing.T) {
	priceErr := errors.New("upstream 503")
	candidateProvider := &mocks.MockRefreshCandidateProvider{Candidates: candidates(3)}
	provider := mocks.NewMockSimplePriceProvider(true)
	provider.GetPriceFn = func(context.Context, pricing.Card) (*pricing.Price, error) {
		return nil, priceErr
	}
	scheduler := newBatchScheduler(candidateProvider, &mocks.MockDBTracker{}, provider, batchConfig())

	scheduler.refreshBatch(context.Background())

	require.Equal(t, 3, provider.CallCount(), "a per-card error does not abort the batch")
	require.Equal(t, 1, consecutiveFailures(scheduler))
	require.False(t, scheduler.LastFailureAt().IsZero())

	// A clean batch resets the counter.
	provider.GetPriceFn = nil
	scheduler.refreshBatch(context.Background())
	require.Zero(t, consecutiveFailures(scheduler))
}

func TestPriceRefreshScheduler_RefreshBatch_NoDataIsNotAFailure(t *testing.T) {
	candidateProvider := &mocks.MockRefreshCandidateProvider{Candidates: candidates(3)}
	provider := mocks.NewMockSimplePriceProvider(true)
	provider.GetPriceFn = func(context.Context, pricing.Card) (*pricing.Price, error) {
		return nil, nil
	}
	scheduler := newBatchScheduler(candidateProvider, &mocks.MockDBTracker{}, provider, batchConfig())

	scheduler.refreshBatch(context.Background())

	require.Equal(t, 3, provider.CallCount())
	require.Zero(t, consecutiveFailures(scheduler), "a nil price is missing data, not an error")
	require.True(t, scheduler.LastFailureAt().IsZero())
}

func TestPriceRefreshScheduler_RefreshBatch_SkipsDuplicatesAndUnnamedCards(t *testing.T) {
	duplicate := pricing.RefreshCandidate{
		CardName:   "Charizard Holo",
		CardNumber: "4",
		SetName:    "Base Set",
		Grade:      10,
	}
	candidateProvider := &mocks.MockRefreshCandidateProvider{
		Candidates: []pricing.RefreshCandidate{
			duplicate,
			duplicate,
			{CardName: "", CardNumber: "5", SetName: "Base Set"},
		},
	}
	provider := mocks.NewMockSimplePriceProvider(true)
	scheduler := newBatchScheduler(candidateProvider, &mocks.MockDBTracker{}, provider, batchConfig())

	scheduler.refreshBatch(context.Background())

	require.Equal(t, 1, provider.CallCount(), "duplicate and unnamed candidates are skipped")
	require.Zero(t, consecutiveFailures(scheduler), "skips are not errors")
}

func TestPriceRefreshScheduler_RefreshBatch_ContextCancellation(t *testing.T) {
	candidateProvider := &mocks.MockRefreshCandidateProvider{Candidates: candidates(5)}
	provider := mocks.NewMockSimplePriceProvider(true)
	scheduler := newBatchScheduler(candidateProvider, &mocks.MockDBTracker{}, provider, batchConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scheduler.refreshBatch(ctx)

	require.Equal(t, 0, provider.CallCount(), "a cancelled context stops the batch before pricing")
}
