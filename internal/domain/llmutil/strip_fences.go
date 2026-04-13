// Package llmutil provides pure utility functions for processing LLM responses.
package llmutil

import "strings"

// StripMarkdownFences removes ```json / ``` wrappers from LLM output.
// It trims leading/trailing whitespace, strips an optional ```json prefix,
// strips a trailing ``` suffix, and trims whitespace again.
func StripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
