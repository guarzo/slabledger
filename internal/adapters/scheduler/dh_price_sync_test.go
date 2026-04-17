package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type fakePriceSyncService struct {
	calls  int32
	result dhpricing.SyncBatchResult
}

func (f *fakePriceSyncService) SyncPurchasePrice(_ context.Context, _ string) dhpricing.SyncResult {
	return dhpricing.SyncResult{}
}

func (f *fakePriceSyncService) SyncDriftedPurchases(_ context.Context) dhpricing.SyncBatchResult {
	atomic.AddInt32(&f.calls, 1)
	return f.result
}

func TestDHPriceSyncScheduler_DisabledDoesNothing(t *testing.T) {
	svc := &fakePriceSyncService{}
	s := NewDHPriceSyncScheduler(svc, observability.NewNoopLogger(), DHPriceSyncConfig{Enabled: false, Interval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	// Start returns immediately when disabled.
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("disabled Start did not return within 100ms")
	}

	if got := atomic.LoadInt32(&svc.calls); got != 0 {
		t.Errorf("disabled scheduler called service %d times, want 0", got)
	}
}

func TestDHPriceSyncScheduler_Ticks(t *testing.T) {
	svc := &fakePriceSyncService{}
	s := NewDHPriceSyncScheduler(svc, observability.NewNoopLogger(), DHPriceSyncConfig{Enabled: true, Interval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	// Wait long enough for initial run plus at least one ticker-driven run.
	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop within 1s of cancel")
	}
	s.Wait()

	if got := atomic.LoadInt32(&svc.calls); got < 2 {
		t.Errorf("scheduler called service %d times, want >= 2", got)
	}
}
