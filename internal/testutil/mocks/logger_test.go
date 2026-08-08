package mocks

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func TestCapturingLogger_With(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// log makes exactly one call through the root logger or a child of it.
		log        func(root *CapturingLogger)
		wantLevel  string
		wantFields map[string]any
	}{
		{
			name:       "no With: only the call's own fields",
			log:        func(root *CapturingLogger) { root.Warn(ctx, "msg", observability.String("a", "1")) },
			wantLevel:  "warn",
			wantFields: map[string]any{"a": "1"},
		},
		{
			name: "child prepends its inherited fields",
			log: func(root *CapturingLogger) {
				root.With(ctx, observability.String("job", "refresh")).Info(ctx, "msg", observability.String("a", "1"))
			},
			wantLevel:  "info",
			wantFields: map[string]any{"job": "refresh", "a": "1"},
		},
		{
			name: "chained With accumulates",
			log: func(root *CapturingLogger) {
				root.With(ctx, observability.String("job", "refresh")).
					With(ctx, observability.String("step", "cards")).
					Error(ctx, "msg")
			},
			wantLevel:  "error",
			wantFields: map[string]any{"job": "refresh", "step": "cards"},
		},
		{
			name: "child entries land in the root's Entries",
			log: func(root *CapturingLogger) {
				root.With(ctx, observability.String("job", "refresh")).Debug(ctx, "msg")
			},
			wantLevel:  "debug",
			wantFields: map[string]any{"job": "refresh"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := NewCapturingLogger()
			tc.log(root)

			entries := root.Entries()
			if len(entries) != 1 {
				t.Fatalf("Entries() = %d, want 1: %+v", len(entries), entries)
			}
			got := entries[0]
			if got.Level != tc.wantLevel {
				t.Errorf("Level = %q, want %q", got.Level, tc.wantLevel)
			}
			if len(got.Fields) != len(tc.wantFields) {
				t.Fatalf("Fields = %+v, want %d entries", got.Fields, len(tc.wantFields))
			}
			for key, want := range tc.wantFields {
				val, ok := got.FindField(key)
				if !ok {
					t.Errorf("missing field %q in %+v", key, got.Fields)
					continue
				}
				if val != want {
					t.Errorf("field %q = %v, want %v", key, val, want)
				}
			}
		})
	}
}

// TestCapturingLogger_WithDoesNotShareBacking guards against two children of
// the same parent aliasing one inherited-fields slice, which would let the
// second With overwrite the first child's field.
func TestCapturingLogger_WithDoesNotShareBacking(t *testing.T) {
	ctx := context.Background()
	root := NewCapturingLogger()
	parent := root.With(ctx, observability.String("job", "refresh"))

	first := parent.With(ctx, observability.String("step", "cards"))
	_ = parent.With(ctx, observability.String("step", "characters"))
	first.Info(ctx, "msg")

	entries := root.Entries()
	if len(entries) != 1 {
		t.Fatalf("Entries() = %d, want 1", len(entries))
	}
	val, ok := entries[0].FindField("step")
	if !ok {
		t.Fatalf("missing field \"step\" in %+v", entries[0].Fields)
	}
	if val != "cards" {
		t.Errorf("field \"step\" = %v, want \"cards\"", val)
	}
}
