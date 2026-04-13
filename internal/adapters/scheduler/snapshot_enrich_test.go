package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// --- Config default tests ---

func TestNewSnapshotEnrichScheduler_DefaultInterval(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled: true,
		// Interval is zero — should default to 15s
	})

	require.Equal(t, 15*time.Second, s.config.Interval,
		"zero Interval should default to 15s")
}

func TestNewSnapshotEnrichScheduler_DefaultRetryInterval(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:  true,
		Interval: 10 * time.Second,
		// RetryInterval is zero — should default to 30m
	})

	require.Equal(t, 30*time.Minute, s.config.RetryInterval,
		"zero RetryInterval should default to 30m")
}

func TestNewSnapshotEnrichScheduler_DefaultBatchSize(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:       true,
		Interval:      10 * time.Second,
		RetryInterval: 5 * time.Minute,
		// BatchSize is zero — should default to 10
	})

	require.Equal(t, 10, s.config.BatchSize,
		"zero BatchSize should default to 10")
}

func TestNewSnapshotEnrichScheduler_AllDefaults(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled: true,
	})

	require.Equal(t, 15*time.Second, s.config.Interval)
	require.Equal(t, 30*time.Minute, s.config.RetryInterval)
	require.Equal(t, 10, s.config.BatchSize)
}

func TestNewSnapshotEnrichScheduler_ExplicitValues(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:       true,
		Interval:      45 * time.Second,
		RetryInterval: 10 * time.Minute,
		BatchSize:     25,
	})

	require.Equal(t, 45*time.Second, s.config.Interval,
		"explicit Interval should be preserved")
	require.Equal(t, 10*time.Minute, s.config.RetryInterval,
		"explicit RetryInterval should be preserved")
	require.Equal(t, 25, s.config.BatchSize,
		"explicit BatchSize should be preserved")
}

func TestNewSnapshotEnrichScheduler_NegativeValues(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:       true,
		Interval:      -1 * time.Second,
		RetryInterval: -5 * time.Minute,
		BatchSize:     -3,
	})

	require.Equal(t, 15*time.Second, s.config.Interval,
		"negative Interval should default to 15s")
	require.Equal(t, 30*time.Minute, s.config.RetryInterval,
		"negative RetryInterval should default to 30m")
	require.Equal(t, 10, s.config.BatchSize,
		"negative BatchSize should default to 10")
}

// --- Disabled / nil service guard tests ---

func TestSnapshotEnrichScheduler_DisabledDoesNotStart(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:  false,
		Interval: 10 * time.Millisecond,
	})

	done := make(chan struct{})
	go func() {
		s.Start(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Start returned immediately — correct behaviour
	case <-time.After(1 * time.Second):
		t.Fatal("disabled scheduler did not return immediately from Start")
	}

	require.Equal(t, int32(0), svc.PendingCallCount(),
		"disabled scheduler should not call ProcessPendingSnapshots")
	require.Equal(t, int32(0), svc.RetryCallCount(),
		"disabled scheduler should not call RetryFailedSnapshots")
}

func TestSnapshotEnrichScheduler_NilServiceDoesNotStart(t *testing.T) {
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(nil, logger, config.SnapshotEnrichConfig{
		Enabled:  true,
		Interval: 10 * time.Millisecond,
	})

	done := make(chan struct{})
	go func() {
		s.Start(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Start returned immediately — correct behaviour
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler with nil service did not return immediately from Start")
	}
}

// --- tickPending tests ---

func TestSnapshotEnrichScheduler_TickPending(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{
		PendingProcessed: 3,
		PendingSkipped:   1,
		PendingFailed:    0,
	}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:   true,
		Interval:  10 * time.Second,
		BatchSize: 20,
	})

	s.tickPending(context.Background())

	require.Equal(t, int32(1), svc.PendingCallCount(),
		"tickPending should call ProcessPendingSnapshots once")
	require.Equal(t, 20, svc.PendingLimitSeen(),
		"tickPending should pass configured BatchSize as limit")
}

func TestSnapshotEnrichScheduler_TickPendingDefaultBatchSize(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled: true,
		// BatchSize defaults to 10
	})

	s.tickPending(context.Background())

	require.Equal(t, 10, svc.PendingLimitSeen(),
		"tickPending should pass default BatchSize (10) as limit")
}

func TestSnapshotEnrichScheduler_TickPendingZeroResults(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{
		PendingProcessed: 0,
		PendingSkipped:   0,
		PendingFailed:    0,
	}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:   true,
		BatchSize: 5,
	})

	// Should not panic when all counters are zero (no-op logging path)
	s.tickPending(context.Background())

	require.Equal(t, int32(1), svc.PendingCallCount(),
		"tickPending should still call ProcessPendingSnapshots even when results are zero")
}

// --- tickRetry tests ---

func TestSnapshotEnrichScheduler_TickRetry(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{
		RetryProcessed: 2,
		RetrySkipped:   0,
		RetryFailed:    1,
	}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:   true,
		BatchSize: 15,
	})

	s.tickRetry(context.Background())

	require.Equal(t, int32(1), svc.RetryCallCount(),
		"tickRetry should call RetryFailedSnapshots once")
	require.Equal(t, 15, svc.RetryLimitSeen(),
		"tickRetry should pass configured BatchSize as limit")
}

func TestSnapshotEnrichScheduler_TickRetryDefaultBatchSize(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled: true,
		// BatchSize defaults to 10
	})

	s.tickRetry(context.Background())

	require.Equal(t, 10, svc.RetryLimitSeen(),
		"tickRetry should pass default BatchSize (10) as limit")
}

func TestSnapshotEnrichScheduler_TickRetryZeroResults(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{
		RetryProcessed: 0,
		RetrySkipped:   0,
		RetryFailed:    0,
	}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:   true,
		BatchSize: 5,
	})

	// Should not panic when all counters are zero (no-op logging path)
	s.tickRetry(context.Background())

	require.Equal(t, int32(1), svc.RetryCallCount(),
		"tickRetry should still call RetryFailedSnapshots even when results are zero")
}

// --- Start/Stop lifecycle tests ---

func TestSnapshotEnrichScheduler_StartAndStop(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:       true,
		Interval:      50 * time.Millisecond,
		RetryInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()

	// Start in goroutine. Start() calls WG.Add synchronously before launching
	// its own goroutines, then returns. We use a channel to know when Start
	// has returned so we can safely call Stop + Wait without a data race.
	started := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(started)
	}()

	// Wait for Start to return (WG.Add calls have been made)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return in time")
	}

	s.Stop()

	waitDone := make(chan struct{})
	go func() {
		s.Wait()
		close(waitDone)
	}()

	// If Wait() returns, the two WG-tracked goroutines exited properly.
	select {
	case <-waitDone:
		// Both goroutines drained — correct
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop within timeout after Stop()+Wait()")
	}
}

func TestSnapshotEnrichScheduler_ContextCancellationStopsLoops(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:       true,
		Interval:      50 * time.Millisecond,
		RetryInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())

	go s.Start(ctx)

	// Wait for the scheduler to actually invoke the service.
	// The snapshot-enrich loop has a 5s InitialDelay so we need a generous window.
	deadline := time.After(10 * time.Second)
	for svc.PendingCallCount() == 0 && svc.RetryCallCount() == 0 {
		select {
		case <-deadline:
			require.FailNow(t, "scheduler did not invoke the service within the polling window")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()

	// WG should eventually drain
	waitDone := make(chan struct{})
	go func() {
		s.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("WaitGroup did not drain after context cancellation")
	}
}

func TestSnapshotEnrichScheduler_StopIdempotent(t *testing.T) {
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled: false, // disabled so Start returns immediately
	})

	// Multiple calls to Stop should not panic
	s.Stop()
	s.Stop()
	s.Stop()
}

// --- WaitGroup ordering test ---

func TestSnapshotEnrichScheduler_WGAddBeforeGoroutine(t *testing.T) {
	// This verifies the fix where WG.Add is called before goroutine launch.
	// If WG.Add were inside the goroutine, Stop()+Wait() right after Start()
	// could return before the goroutine increments the counter, causing a
	// premature return from Wait(). We test this by starting and immediately
	// stopping — Wait() must block until both goroutines have finished.
	svc := &mocks.MockSnapshotEnrichService{}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:       true,
		Interval:      1 * time.Hour, // long interval — goroutines block on initial delay
		RetryInterval: 1 * time.Hour,
	})

	s.Start(context.Background())
	s.Stop()

	waitDone := make(chan struct{})
	go func() {
		s.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// Both goroutines drained — WG.Add was before goroutine launch
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() did not return promptly; WG.Add may be inside the goroutine")
	}
}

// --- Integration-style test: verify both loops invoke the service ---

func TestSnapshotEnrichScheduler_BothLoopsRun(t *testing.T) {
	// Use very short intervals and no initial delay is not possible since
	// RunLoop has a hardcoded InitialDelay. But we can call the tick methods
	// directly to verify both paths work.
	svc := &mocks.MockSnapshotEnrichService{
		PendingProcessed: 1,
		RetryProcessed:   1,
	}
	logger := mocks.NewMockLogger()

	s := NewSnapshotEnrichScheduler(svc, logger, config.SnapshotEnrichConfig{
		Enabled:   true,
		BatchSize: 7,
	})

	ctx := context.Background()

	s.tickPending(ctx)
	s.tickPending(ctx)
	s.tickRetry(ctx)

	require.Equal(t, int32(2), svc.PendingCallCount(),
		"tickPending called twice")
	require.Equal(t, int32(1), svc.RetryCallCount(),
		"tickRetry called once")
	require.Equal(t, 7, svc.PendingLimitSeen())
	require.Equal(t, 7, svc.RetryLimitSeen())
}
