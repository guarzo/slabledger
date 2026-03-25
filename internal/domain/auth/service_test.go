package auth

import (
	"testing"
)

func TestGenerateState(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "generate state 1"},
		{name: "generate state 2"},
		{name: "generate state 3"},
	}

	states := make(map[string]bool)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := GenerateState()
			if err != nil {
				t.Errorf("GenerateState() error = %v", err)
				return
			}

			if state == "" {
				t.Error("Expected non-empty state")
			}

			// Check uniqueness
			if states[state] {
				t.Error("State should be unique")
			}
			states[state] = true

			// Check length (base64 encoded 32 bytes should be ~44 chars)
			if len(state) < 40 {
				t.Errorf("State too short: %d characters", len(state))
			}
		})
	}
}
