package advisor

import (
	"fmt"
	"strings"
)

// isLLMRefusal detects common refusal patterns from Azure content filters or
// model safety systems. These should not be treated as successful results.
func isLLMRefusal(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	refusalPatterns := []string{
		"i'm sorry, but i cannot assist",
		"i'm sorry, but i can't assist",
		"i cannot assist with that request",
		"i can't assist with that request",
		"i'm unable to help with that",
		"i'm not able to assist",
		"as an ai, i cannot",
	}
	for _, pattern := range refusalPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// truncateToolResult caps a tool result string at maxLen characters.
// If truncated, it tries to cut at the last newline within the limit to
// avoid splitting a JSON line, and appends a notice so the LLM knows
// the data was cut short.
func truncateToolResult(result string, maxLen int) string {
	if len(result) <= maxLen {
		return result
	}
	cut := result[:maxLen]
	// Try to cut at a newline boundary to avoid splitting a JSON object mid-field.
	if idx := strings.LastIndex(cut, "\n"); idx > maxLen/2 {
		cut = cut[:idx]
	}
	return cut + "\n\n[... truncated — tool returned " + fmt.Sprintf("%d", len(result)) + " chars, showing first " + fmt.Sprintf("%d", len(cut)) + "]"
}
