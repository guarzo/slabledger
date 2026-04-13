package llmutil_test

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/llmutil"
)

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no fences",
			input: `{"key":"value"}`,
			want:  `{"key":"value"}`,
		},
		{
			name:  "json fences",
			input: "```json\n{\"key\":\"value\"}\n```",
			want:  `{"key":"value"}`,
		},
		{
			name:  "plain code fences",
			input: "```\n{\"key\":\"value\"}\n```",
			want:  `{"key":"value"}`,
		},
		{
			name:  "leading/trailing whitespace",
			input: "  \n```json\n{\"key\":\"value\"}\n```\n  ",
			want:  `{"key":"value"}`,
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only fences",
			input: "```json\n```",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := llmutil.StripMarkdownFences(tc.input)
			if got != tc.want {
				t.Errorf("StripMarkdownFences(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
