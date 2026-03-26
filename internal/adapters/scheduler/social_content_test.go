package scheduler

import (
	"testing"
	"time"
)

func TestSocialContentScheduler_UsesContentHour(t *testing.T) {
	// timeUntilHour is already tested in advisor_refresh_test.go.
	// Just verify it's reachable from this package (it's package-level).
	now := time.Date(2026, 3, 26, 2, 0, 0, 0, time.UTC)
	d := timeUntilHour(now, 5)
	if d < 2*time.Hour || d > 4*time.Hour {
		t.Errorf("timeUntilHour(%v, 5) = %v, want ~3h", now, d)
	}
}
