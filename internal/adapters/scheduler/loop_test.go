package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// nopLogger satisfies observability.Logger for tests with no output.
type nopLogger struct{}

func (nopLogger) Debug(_ context.Context, _ string, _ ...observability.Field) {}
func (nopLogger) Info(_ context.Context, _ string, _ ...observability.Field)  {}
func (nopLogger) Warn(_ context.Context, _ string, _ ...observability.Field)  {}
func (nopLogger) Error(_ context.Context, _ string, _ ...observability.Field) {}
func (l nopLogger) With(_ context.Context, _ ...observability.Field) observability.Logger {
	return l
}

func TestRunLoop_BasicTickCycle(t *testing.T) {
	var count atomic.Int32
	stopChan := make(chan struct{})
	// Signal closed when workFn has been called at least twice (initial + tick).
	ready := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunLoop(ctx, LoopConfig{
			Name:     "test",
			Interval: 10 * time.Millisecond,
			StopChan: stopChan,
			Logger:   nopLogger{},
		}, func(_ context.Context) {
			if count.Add(1) == 2 {
				close(ready)
			}
		})
		close(done)
	}()

	// Wait for at least 2 invocations via channel signal.
	select {
	case <-ready:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for workFn to be called twice")
	}

	cancel()
	<-done

	c := count.Load()
	if c < 2 {
		t.Errorf("expected at least 2 workFn calls (initial + tick), got %d", c)
	}
}

func TestRunLoop_ContextCancellation(t *testing.T) {
	stopChan := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		RunLoop(ctx, LoopConfig{
			Name:     "test",
			Interval: 1 * time.Hour,
			StopChan: stopChan,
			Logger:   nopLogger{},
		}, func(_ context.Context) {})
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("RunLoop did not exit after context cancellation")
	}
}

func TestRunLoop_StopChanSignal(t *testing.T) {
	stopChan := make(chan struct{})
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		RunLoop(ctx, LoopConfig{
			Name:     "test",
			Interval: 1 * time.Hour,
			StopChan: stopChan,
			Logger:   nopLogger{},
		}, func(_ context.Context) {})
		close(done)
	}()

	close(stopChan)

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("RunLoop did not exit after stop signal")
	}
}

func TestRunLoop_InitialDelay(t *testing.T) {
	var count atomic.Int32
	stopChan := make(chan struct{})
	// Signal closed when workFn has been called once (after the initial delay).
	firstRun := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunLoop(ctx, LoopConfig{
			Name:         "test",
			Interval:     1 * time.Hour,
			InitialDelay: 50 * time.Millisecond,
			StopChan:     stopChan,
			Logger:       nopLogger{},
		}, func(_ context.Context) {
			if count.Add(1) == 1 {
				close(firstRun)
			}
		})
		close(done)
	}()

	// Before delay: workFn should not have run yet.
	// The goroutine just launched and the initial delay is 50ms,
	// so an immediate check is safe.
	if c := count.Load(); c != 0 {
		t.Errorf("expected 0 calls before delay, got %d", c)
	}

	// After delay: wait for workFn to signal it has run once.
	select {
	case <-firstRun:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for workFn to be called after initial delay")
	}

	if c := count.Load(); c != 1 {
		t.Errorf("expected 1 call after delay, got %d", c)
	}

	cancel()
	<-done
}

func TestRunLoop_InitialDelayStopDuringDelay(t *testing.T) {
	var count atomic.Int32
	stopChan := make(chan struct{})
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		RunLoop(ctx, LoopConfig{
			Name:         "test",
			Interval:     1 * time.Hour,
			InitialDelay: 1 * time.Hour,
			StopChan:     stopChan,
			Logger:       nopLogger{},
		}, func(_ context.Context) {
			count.Add(1)
		})
		close(done)
	}()

	// Stop during the initial delay
	close(stopChan)

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("RunLoop did not exit when stopped during initial delay")
	}

	if c := count.Load(); c != 0 {
		t.Errorf("expected 0 calls when stopped during delay, got %d", c)
	}
}

func TestRunLoop_WGTracking(t *testing.T) {
	var wg sync.WaitGroup
	stopChan := make(chan struct{})
	ctx := context.Background()

	// workFn signals when it has been called, which proves wg.Add already ran.
	started := make(chan struct{})
	go RunLoop(ctx, LoopConfig{
		Name:     "test",
		Interval: 1 * time.Hour,
		WG:       &wg,
		StopChan: stopChan,
		Logger:   nopLogger{},
	}, func(_ context.Context) {
		select {
		case <-started:
		default:
			close(started)
		}
	})

	// Wait for workFn to execute (wg.Add has happened by now)
	<-started

	close(stopChan)

	// wg.Wait should return once RunLoop finishes
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("WaitGroup did not complete after RunLoop exited")
	}
}

func TestRunLoop_WGNil(t *testing.T) {
	stopChan := make(chan struct{})
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		RunLoop(ctx, LoopConfig{
			Name:     "test",
			Interval: 1 * time.Hour,
			WG:       nil,
			StopChan: stopChan,
			Logger:   nopLogger{},
		}, func(_ context.Context) {})
		close(done)
	}()

	close(stopChan)

	select {
	case <-done:
		// success — no panic with nil WG
	case <-time.After(1 * time.Second):
		t.Fatal("RunLoop did not exit with nil WG")
	}
}
