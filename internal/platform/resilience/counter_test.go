package resilience

import (
	"testing"
	"time"
)

func TestResettingCounter_IncrementAndLoad(t *testing.T) {
	c := NewResettingCounter(24 * time.Hour)
	if got := c.Load(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	c.Inc()
	c.Inc()
	c.Inc()
	if got := c.Load(); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestResettingCounter_ResetAfterInterval(t *testing.T) {
	c := NewResettingCounter(1 * time.Millisecond)
	c.Inc()
	c.Inc()
	if got := c.Load(); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
	time.Sleep(5 * time.Millisecond)
	if got := c.Load(); got != 0 {
		t.Fatalf("expected 0 after reset, got %d", got)
	}
}

func TestResettingCounter_ConcurrentAccess(t *testing.T) {
	c := NewResettingCounter(24 * time.Hour)
	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			c.Inc()
			c.Load()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
	if got := c.Load(); got != 100 {
		t.Fatalf("expected 100, got %d", got)
	}
}
