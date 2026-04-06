package dh

import (
	"sync"
	"time"
)

const (
	healthWindowMinutes = 7 * 24 * 60 // 7 days in minutes
)

// HealthStats holds aggregate API health metrics.
type HealthStats struct {
	TotalCalls  int     `json:"total_calls"`
	Failures    int     `json:"failures"`
	SuccessRate float64 `json:"success_rate"`
}

type healthBucket struct {
	minute   int64 // unix minute (time / 60)
	success  int
	failures int
}

// HealthTracker tracks API call success/failure counts in a rolling 7-day window
// using minute-granularity buckets.
type HealthTracker struct {
	mu      sync.Mutex
	buckets []healthBucket
	nowFunc func() time.Time
}

// NewHealthTracker creates a new HealthTracker. An optional nowFunc overrides the
// time source (defaults to time.Now); pass a custom function in tests for
// deterministic time control.
func NewHealthTracker(nowFunc ...func() time.Time) *HealthTracker {
	nf := time.Now
	if len(nowFunc) > 0 && nowFunc[0] != nil {
		nf = nowFunc[0]
	}
	return &HealthTracker{nowFunc: nf}
}

func (ht *HealthTracker) currentMinute() int64 {
	return ht.nowFunc().Unix() / 60
}

func (ht *HealthTracker) record(success bool) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	now := ht.currentMinute()
	ht.pruneOld(now)

	// Find or create bucket for current minute.
	if len(ht.buckets) > 0 && ht.buckets[len(ht.buckets)-1].minute == now {
		b := &ht.buckets[len(ht.buckets)-1]
		if success {
			b.success++
		} else {
			b.failures++
		}
		return
	}

	b := healthBucket{minute: now}
	if success {
		b.success = 1
	} else {
		b.failures = 1
	}
	ht.buckets = append(ht.buckets, b)
}

// RecordSuccess records a successful API call.
func (ht *HealthTracker) RecordSuccess() { ht.record(true) }

// RecordFailure records a failed API call.
func (ht *HealthTracker) RecordFailure() { ht.record(false) }

// Stats returns aggregate health metrics across the rolling window.
func (ht *HealthTracker) Stats() HealthStats {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	now := ht.currentMinute()
	ht.pruneOld(now)

	var total, failures int
	for _, b := range ht.buckets {
		total += b.success + b.failures
		failures += b.failures
	}

	rate := 1.0
	if total > 0 {
		rate = float64(total-failures) / float64(total)
	}

	return HealthStats{
		TotalCalls:  total,
		Failures:    failures,
		SuccessRate: rate,
	}
}

// pruneOld removes buckets older than the rolling window. Must be called with mu held.
func (ht *HealthTracker) pruneOld(nowMinute int64) {
	cutoff := nowMinute - healthWindowMinutes
	i := 0
	for i < len(ht.buckets) && ht.buckets[i].minute < cutoff {
		i++
	}
	if i > 0 {
		ht.buckets = ht.buckets[i:]
	}
}
