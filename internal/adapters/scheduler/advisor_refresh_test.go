package scheduler

import (
	"fmt"
	"testing"
)

func TestIsTransientAIError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"broken pipe", fmt.Errorf("broken pipe"), true},
		{"EOF", fmt.Errorf("unexpected EOF"), true},
		{"i/o timeout", fmt.Errorf("i/o timeout"), true},
		{"SSE stream incomplete", fmt.Errorf("SSE stream ended without response.completed (possible network interruption)"), true},
		{"context deadline in LLM round", fmt.Errorf("llm completion (round 3): context deadline exceeded"), true},
		{"context deadline bare", fmt.Errorf("context deadline exceeded"), true},
		{"permanent 400 error", fmt.Errorf("azure ai returned 400: bad request"), false},
		{"unknown error", fmt.Errorf("something unexpected"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientAIError(tt.err)
			if got != tt.want {
				t.Errorf("isTransientAIError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
