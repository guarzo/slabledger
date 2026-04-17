package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// waitForCalls polls callCount until it reaches want (or a deadline expires).
// Preferable to a fixed time.Sleep because it is both faster in the happy case
// and more tolerant of CI load.
func waitForCalls(t *testing.T, callCount *int32, want int32, deadline time.Duration) {
	t.Helper()
	timeout := time.After(deadline)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if atomic.LoadInt32(callCount) >= want {
			return
		}
		select {
		case <-timeout:
			t.Fatalf("expected >= %d calls within %s, got %d",
				want, deadline, atomic.LoadInt32(callCount))
		case <-tick.C:
		}
	}
}

func TestDHPriceSyncScheduler_DisabledDoesNothing(t *testing.T) {
	var calls int32
	svc := &mocks.MockDHPriceSyncService{
		SyncDriftedPurchasesFn: func(_ context.Context) dhpricing.SyncBatchResult {
			atomic.AddInt32(&calls, 1)
			return dhpricing.SyncBatchResult{ByOutcome: map[dhpricing.Outcome]int{}}
		},
	}
	s := NewDHPriceSyncScheduler(svc, observability.NewNoopLogger(),
		DHPriceSyncConfig{Enabled: false, Interval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("disabled Start did not return within 100ms")
	}

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("disabled scheduler called service %d times, want 0", got)
	}
}

func TestDHPriceSyncScheduler_Ticks(t *testing.T) {
	var calls int32
	svc := &mocks.MockDHPriceSyncService{
		SyncDriftedPurchasesFn: func(_ context.Context) dhpricing.SyncBatchResult {
			atomic.AddInt32(&calls, 1)
			return dhpricing.SyncBatchResult{ByOutcome: map[dhpricing.Outcome]int{}}
		},
	}
	s := NewDHPriceSyncScheduler(svc, observability.NewNoopLogger(),
		DHPriceSyncConfig{Enabled: true, Interval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	// Wait for the initial synchronous tick plus at least one ticker-driven tick.
	waitForCalls(t, &calls, 2, time.Second)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop within 1s of cancel")
	}
	s.Wait()
}
