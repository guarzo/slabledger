package azureai

import (
	"reflect"
	"testing"
)

func TestEnsureProperties(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		expect any
	}{
		{
			name:   "nil input",
			input:  nil,
			expect: nil,
		},
		{
			name:   "object without properties gets empty properties",
			input:  map[string]any{"type": "object"},
			expect: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name: "object with existing properties unchanged",
			input: map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
			},
			expect: map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
			},
		},
		{
			name: "nested object schemas get properties",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"nested": map[string]any{"type": "object"},
				},
			},
			expect: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"nested": map[string]any{"type": "object", "properties": map[string]any{}},
				},
			},
		},
		{
			name: "array items get properties",
			input: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "object"},
			},
			expect: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
		{
			name:   "string type unchanged",
			input:  map[string]any{"type": "string"},
			expect: map[string]any{"type": "string"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureProperties(tc.input)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("ensureProperties() =\n  %v\nwant\n  %v", got, tc.expect)
			}
		})
	}
}
