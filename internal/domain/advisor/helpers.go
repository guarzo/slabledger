package advisor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// maxPriorContextLen caps the prior analysis content appended to the system prompt.
// This prevents old, large analyses from consuming too many input tokens.
const maxPriorContextLen = 2000

// priorContext fetches the most recent completed analysis of the given type from
// the cache and returns a system prompt section summarizing it. Returns "" if no
// cache is configured or no prior analysis exists.
func (s *service) priorContext(ctx context.Context, analysisType AnalysisType) string {
	if s.cache == nil {
		return ""
	}
	cached, err := s.cache.Get(ctx, analysisType)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "failed to fetch prior analysis for context",
				observability.String("type", string(analysisType)),
				observability.Err(err))
		}
		return ""
	}
	if cached == nil || cached.Status != StatusComplete || cached.Content == "" {
		return ""
	}

	content := cached.Content
	if len(content) > maxPriorContextLen {
		content = content[:maxPriorContextLen] + "\n\n[... truncated — see full prior analysis in the cache]"
	}

	age := time.Since(cached.CompletedAt).Round(time.Hour)
	return fmt.Sprintf(`

## Prior Analysis (%s ago)
Below is your previous %s analysis. Reference it to track changes, follow up on prior recommendations, and note what improved or worsened. Do NOT repeat it — focus on what's new or different.

<prior_analysis>
%s
</prior_analysis>`, age, analysisType, content)
}

// isLLMRefusal detects common refusal patterns from Azure content filters or
// model safety systems. These refusals should not be cached as successful results
// because they poison the prior context for subsequent runs.
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
