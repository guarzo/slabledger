package dh

import (
	"math"
	"testing"
	"time"
)

const floatEpsilon = 1e-9

func TestHealthTracker_Stats(t *testing.T) {
	tests := []struct {
		name            string
		successes       int
		failures        int
		wantTotalCalls  int
		wantFailures    int
		wantSuccessRate float64
	}{
		{"empty", 0, 0, 0, 0, 1.0},
		{"successes only", 3, 0, 3, 0, 1.0},
		{"failures only", 0, 1, 1, 1, 0.0},
		{"mixed calls", 8, 2, 10, 2, 0.8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ht := NewHealthTracker()
			for i := 0; i < tt.successes; i++ {
				ht.RecordSuccess()
			}
			for i := 0; i < tt.failures; i++ {
				ht.RecordFailure()
			}

			stats := ht.Stats()
			if stats.TotalCalls != tt.wantTotalCalls {
				t.Errorf("TotalCalls = %d, want %d", stats.TotalCalls, tt.wantTotalCalls)
			}
			if stats.Failures != tt.wantFailures {
				t.Errorf("Failures = %d, want %d", stats.Failures, tt.wantFailures)
			}
			if math.Abs(stats.SuccessRate-tt.wantSuccessRate) > floatEpsilon {
				t.Errorf("SuccessRate = %v, want %v (delta %v)", stats.SuccessRate, tt.wantSuccessRate, stats.SuccessRate-tt.wantSuccessRate)
			}
		})
	}
}

func TestHealthTracker_PruneOldBuckets(t *testing.T) {
	now := time.Now()
	ht := NewHealthTracker(func() time.Time { return now })

	// Manually inject a stale bucket (older than 7 days)
	ht.mu.Lock()
	staleBucket := healthBucket{
		minute:  ht.currentMinute() - healthWindowMinutes - 10,
		success: 100,
	}
	ht.buckets = append([]healthBucket{staleBucket}, ht.buckets...)
	ht.mu.Unlock()

	// Record a fresh call
	ht.RecordSuccess()

	stats := ht.Stats()
	// Stale bucket should be pruned; only the fresh call should count.
	if stats.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1 (stale bucket should be pruned)", stats.TotalCalls)
	}
}
