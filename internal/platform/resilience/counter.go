package resilience

import (
	"sync"
	"sync/atomic"
	"time"
)

// ResettingCounter is a thread-safe counter that resets to zero after a
// configurable interval. Useful for tracking daily or per-minute API call
// counts across multiple clients.
type ResettingCounter struct {
	count    atomic.Int64
	resetAt  atomic.Int64 // unix nano timestamp of next reset
	mu       sync.Mutex   // protects the reset-and-zero operation
	interval time.Duration
}

// NewResettingCounter creates a counter that auto-resets every interval.
func NewResettingCounter(interval time.Duration) *ResettingCounter {
	rc := &ResettingCounter{interval: interval}
	rc.resetAt.Store(time.Now().Add(interval).UnixNano())
	return rc
}

// Inc atomically increments the counter by 1, resetting first if needed.
func (rc *ResettingCounter) Inc() {
	rc.resetIfNeeded()
	rc.count.Add(1)
}

// Load returns the current count, resetting first if the interval has elapsed.
func (rc *ResettingCounter) Load() int64 {
	rc.resetIfNeeded()
	return rc.count.Load()
}

func (rc *ResettingCounter) resetIfNeeded() {
	now := time.Now().UnixNano()
	if now < rc.resetAt.Load() {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	oldReset := rc.resetAt.Load()
	if now >= oldReset {
		next := time.Now().Add(rc.interval).UnixNano()
		if rc.resetAt.CompareAndSwap(oldReset, next) {
			rc.count.Store(0)
		}
	}
}
