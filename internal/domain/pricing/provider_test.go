package pricing

import "testing"

func TestIsDHTombstoned(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		want     bool
	}{
		{name: "no attempts", attempts: 0, want: false},
		{name: "one below threshold", attempts: DHTombstoneThreshold - 2, want: false},
		{name: "just below threshold", attempts: DHTombstoneThreshold - 1, want: false},
		{name: "at threshold", attempts: DHTombstoneThreshold, want: true},
		{name: "above threshold", attempts: DHTombstoneThreshold + 1, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDHTombstoned(tt.attempts); got != tt.want {
				t.Errorf("IsDHTombstoned(%d) = %v, want %v", tt.attempts, got, tt.want)
			}
		})
	}
}

// TestDHTombstoneThresholdValue pins the concrete boundary the adapters and the
// dh_card_tombstones query rely on, so a change to the constant is a deliberate
// edit here rather than a silent behavior shift.
func TestDHTombstoneThresholdValue(t *testing.T) {
	if DHTombstoneThreshold != 3 {
		t.Errorf("DHTombstoneThreshold = %d, want 3", DHTombstoneThreshold)
	}
}
