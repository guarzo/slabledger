package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockScheduler struct {
	startFn func(ctx context.Context)
	stopFn  func()
}

func (m *mockScheduler) Start(ctx context.Context) {
	if m.startFn != nil {
		m.startFn(ctx)
	}
}

func (m *mockScheduler) Stop() {
	if m.stopFn != nil {
		m.stopFn()
	}
}

func TestGroup_EmptyGroup(t *testing.T) {
	g := NewGroup()

	g.StartAll(context.Background())
	g.StopAll()

	done := make(chan struct{})
	go func() {
		g.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Wait should return immediately for empty group")
	}
}

func TestGroup_SingleScheduler(t *testing.T) {
	var startCalled, stopCalled atomic.Int32
	stopCh := make(chan struct{})

	sched := &mockScheduler{
		startFn: func(_ context.Context) {
			startCalled.Add(1)
			<-stopCh
		},
		stopFn: func() {
			stopCalled.Add(1)
			select {
			case <-stopCh:
			default:
				close(stopCh)
			}
		},
	}

	g := NewGroup(sched)
	g.StartAll(context.Background())

	// Give goroutine time to start
	time.Sleep(20 * time.Millisecond)

	if startCalled.Load() != 1 {
		t.Errorf("expected Start called once, got %d", startCalled.Load())
	}

	g.StopAll()

	done := make(chan struct{})
	go func() {
		g.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Wait should return after StopAll")
	}

	if stopCalled.Load() != 1 {
		t.Errorf("expected Stop called once, got %d", stopCalled.Load())
	}
}

func TestGroup_MultipleSchedulers(t *testing.T) {
	var startCount, stopCount atomic.Int32
	var mu sync.Mutex
	stopChannels := make([]chan struct{}, 3)

	for i := range stopChannels {
		stopChannels[i] = make(chan struct{})
	}

	schedulers := make([]Scheduler, 3)
	for i := range schedulers {
		idx := i
		schedulers[i] = &mockScheduler{
			startFn: func(_ context.Context) {
				startCount.Add(1)
				<-stopChannels[idx]
			},
			stopFn: func() {
				stopCount.Add(1)
				mu.Lock()
				defer mu.Unlock()
				select {
				case <-stopChannels[idx]:
				default:
					close(stopChannels[idx])
				}
			},
		}
	}

	g := NewGroup(schedulers...)
	g.StartAll(context.Background())

	// Give goroutines time to start
	time.Sleep(30 * time.Millisecond)

	if c := startCount.Load(); c != 3 {
		t.Errorf("expected 3 starts, got %d", c)
	}

	g.StopAll()

	done := make(chan struct{})
	go func() {
		g.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Wait should return after all schedulers stopped")
	}

	if c := stopCount.Load(); c != 3 {
		t.Errorf("expected 3 stops, got %d", c)
	}
}

func TestGroup_WaitBlocksUntilDone(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	sched := &mockScheduler{
		startFn: func(_ context.Context) {
			close(started)
			<-release
		},
		stopFn: func() {},
	}

	g := NewGroup(sched)
	g.StartAll(context.Background())

	// Wait for scheduler to start
	<-started

	// Wait should block
	waitDone := make(chan struct{})
	go func() {
		g.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		t.Fatal("Wait should block while scheduler is running")
	case <-time.After(50 * time.Millisecond):
		// expected — still blocked
	}

	// Release the scheduler
	close(release)

	select {
	case <-waitDone:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Wait should return after scheduler finishes")
	}
}
