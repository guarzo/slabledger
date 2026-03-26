package azureai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// parseSSEStream reads the SSE event stream and emits CompletionChunks.
func (c *Client) parseSSEStream(ctx context.Context, body io.Reader, stream func(ai.CompletionChunk)) error {
	scanner := bufio.NewScanner(body)
	// Increase buffer for potentially large tool call arguments.
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	// Accumulate tool calls across chunks (they arrive incrementally).
	toolCalls := make(map[int]*ai.ToolCall)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			// Emit any accumulated tool calls with the final chunk.
			if len(toolCalls) > 0 {
				stream(ai.CompletionChunk{
					ToolCalls: flattenToolCalls(toolCalls),
					Done:      true,
				})
			} else {
				stream(ai.CompletionChunk{Done: true})
			}
			return nil
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			if c.logger != nil {
				c.logger.Warn(ctx, "failed to parse SSE chunk",
					observability.String("data", data),
					observability.Err(err),
				)
			}
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// Stream text content.
		if delta.Content != "" {
			stream(ai.CompletionChunk{Delta: delta.Content})
		}

		// Accumulate tool calls (they arrive as partial deltas).
		for _, tc := range delta.ToolCalls {
			existing, ok := toolCalls[tc.Index]
			if !ok {
				existing = &ai.ToolCall{}
				toolCalls[tc.Index] = existing
			}
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Function.Name != "" {
				existing.Name = tc.Function.Name
			}
			existing.Arguments += tc.Function.Arguments
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading SSE stream: %w", err)
	}

	// Stream ended without [DONE] — emit what we have.
	if c.logger != nil {
		c.logger.Warn(ctx, "SSE stream ended without [DONE]",
			observability.Int("pending_tool_calls", len(toolCalls)),
		)
	}
	if len(toolCalls) > 0 {
		stream(ai.CompletionChunk{
			ToolCalls: flattenToolCalls(toolCalls),
			Done:      true,
		})
	}
	return nil
}

// isPermanentError returns true for non-retriable client errors (4xx except 429).
func isPermanentError(err error) bool {
	if err == nil {
		return false
	}
	if errors.As(err, new(*rateLimitError)) || errors.As(err, new(*capacityError)) {
		return false // retriable
	}
	msg := err.Error()
	// Match "azure ai returned 4XX:" where XX is not 29
	if strings.HasPrefix(msg, "azure ai returned 4") && !strings.HasPrefix(msg, "azure ai returned 429") {
		return true
	}
	return false
}

func flattenToolCalls(m map[int]*ai.ToolCall) []ai.ToolCall {
	result := make([]ai.ToolCall, 0, len(m))
	for i := 0; i < len(m); i++ {
		if tc, ok := m[i]; ok {
			result = append(result, *tc)
		}
	}
	return result
}
