package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowPrepLifecycleStartupTickWakeAndStop(t *testing.T) {
	calls := make(chan context.Context, 10)
	var active, peak atomic.Int32
	store := &mocks.ShowPrepWorkerStoreMock{AcquireFn: func(ctx context.Context, owner string) (sp.WorkerLease, bool, error) {
		n := active.Add(1)
		if n > peak.Load() {
			peak.Store(n)
		}
		defer active.Add(-1)
		calls <- ctx
		return sp.WorkerLease{}, false, nil
	}}
	worker := sp.NewEvidenceWorker(store, nil, nil, nil)
	s := NewShowPrepRefreshScheduler(worker, nil, mocks.NewMockLogger(), true)
	require.Equal(t, time.Minute, s.interval)
	s.interval = 15 * time.Millisecond
	group := NewGroup(s)
	group.StartAll(context.Background())
	t.Cleanup(func() { group.StopAll(); group.Wait() })
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("no immediate startup sweep")
	}
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("no periodic sweep")
	}
	require.NoError(t, s.RequestRun(context.Background(), false))
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("no wake")
	}
	group.StopAll()
	group.Wait()
	require.Equal(t, int32(1), peak.Load())
}

func TestShowPrepStopCancelsAndJoinsSource(t *testing.T) {
	entered, left := make(chan struct{}), make(chan struct{})
	worker := sp.NewEvidenceWorker(&mocks.ShowPrepWorkerStoreMock{}, func(ctx context.Context) (sp.Source, error) {
		close(entered)
		<-ctx.Done()
		close(left)
		return nil, ctx.Err()
	}, nil, nil)
	s := NewShowPrepRefreshScheduler(worker, nil, mocks.NewMockLogger(), true)
	group := NewGroup(s)
	group.StartAll(context.Background())
	<-entered
	group.StopAll()
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel and join")
	}
	select {
	case <-left:
	default:
		t.Fatal("source not joined")
	}
}

func TestShowPrepRequestOnlyRecordsIntentAndCoalesces(t *testing.T) {
	var acquire, requests atomic.Int32
	store := &mocks.ShowPrepWorkerStoreMock{
		AcquireFn: func(context.Context, string) (sp.WorkerLease, bool, error) {
			acquire.Add(1)
			return sp.WorkerLease{}, false, nil
		},
		RequestRunFn: func(ctx context.Context, retry bool) error {
			require.NoError(t, ctx.Err())
			requests.Add(1)
			return nil
		},
	}
	s := NewShowPrepRefreshScheduler(sp.NewEvidenceWorker(store, nil, nil, nil), nil, mocks.NewMockLogger(), true)
	ctx, cancel := context.WithCancel(context.Background())
	for range 20 {
		require.NoError(t, s.RequestRun(ctx, false))
	}
	cancel()
	require.Equal(t, int32(20), requests.Load())
	require.Zero(t, acquire.Load())
	require.Len(t, s.wake, 1)
	group := NewGroup(s)
	group.StartAll(context.Background())
	t.Cleanup(func() { group.StopAll(); group.Wait() })
	require.Eventually(t, func() bool { return acquire.Load() > 0 }, time.Second, time.Millisecond)
}

func TestShowPrepDisabledAndStatusNeverAcquire(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "unconfigured"}[enabled], func(t *testing.T) {
			var sourceCalls atomic.Int32
			worker := sp.NewEvidenceWorker(&mocks.ShowPrepWorkerStoreMock{}, func(context.Context) (sp.Source, error) { sourceCalls.Add(1); return nil, nil }, nil, nil)
			s := NewShowPrepRefreshScheduler(worker, nil, mocks.NewMockLogger(), enabled)
			status, err := s.Status(context.Background())
			require.NoError(t, err)
			require.Equal(t, enabled, status.Enabled)
			require.False(t, status.Configured)
			require.Equal(t, map[bool]string{false: "disabled", true: "unconfigured"}[enabled], status.State)
			require.Zero(t, sourceCalls.Load())
			if !enabled {
				s.Start(context.Background())
				require.Zero(t, sourceCalls.Load())
			}
		})
	}
}
