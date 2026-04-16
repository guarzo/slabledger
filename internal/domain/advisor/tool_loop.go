package advisor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// operationMaxRounds overrides s.maxToolRounds per operation.
// CampaignAnalysis and Liquidation use 3 rounds to accommodate batch tools
// and larger workflows (prompt says 2 rounds but suggest_price_batch may
// need a separate round after reading EV data).
// Digest uses 4 rounds: 9 broad tools, then EV batch, then optional deep dive, plus escape hatch.
var operationMaxRounds = map[AIOperation]int{
	OpCampaignAnalysis: 3,
	OpDigest:           4,
	OpLiquidation:      3,
}

// toolCallingLoop orchestrates the LLM -> tool -> LLM cycle.
func (s *service) toolCallingLoop(ctx context.Context, operation AIOperation, systemPrompt string, messages []Message, stream func(StreamEvent)) (string, error) {
	maxRounds := s.maxToolRounds
	if override, ok := operationMaxRounds[operation]; ok {
		maxRounds = override
	}
	var tools []ToolDefinition
	if names, ok := operationTools[operation]; ok {
		if filtered, fOk := s.executor.(FilteredToolExecutor); fOk {
			tools = filtered.DefinitionsFor(names)
		} else {
			tools = s.executor.Definitions()
		}
	} else {
		tools = s.executor.Definitions()
	}
	var conversationState any
	start := time.Now()
	var totalUsage TokenUsage
	lastRound := 0
	var lastToolNames []string

	for round := 0; round < maxRounds; round++ {
		lastRound = round
		temp := s.temperature
		completionReq := CompletionRequest{
			SystemPrompt:      systemPrompt,
			Messages:          messages,
			Tools:             tools,
			Temperature:       &temp,
			MaxTokens:         s.maxTokens,
			ConversationState: conversationState,
			ReasoningEffort:   "medium",
			Store:             true,
		}

		var fullContent strings.Builder
		var toolCalls []ToolCall
		var newConversationState any

		err := s.llm.StreamCompletion(ctx, completionReq, func(chunk CompletionChunk) {
			if chunk.Delta != "" {
				fullContent.WriteString(chunk.Delta)
				stream(StreamEvent{Type: EventDelta, Content: chunk.Delta})
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = append(toolCalls, chunk.ToolCalls...)
			}
			if chunk.ConversationState != nil {
				newConversationState = chunk.ConversationState
			}
			if chunk.Usage != nil {
				totalUsage.InputTokens += chunk.Usage.InputTokens
				totalUsage.OutputTokens += chunk.Usage.OutputTokens
				totalUsage.TotalTokens += chunk.Usage.TotalTokens
			}
		})
		if err != nil {
			ai.RecordCall(ctx, s.tracker, s.logger, operation, err, start, lastRound+1, &totalUsage)
			return "", fmt.Errorf("llm completion (round %d): %w", round, err)
		}

		// Diagnostic: log round results for debugging duplicate-item errors.
		if s.logger != nil {
			stateStr := "<nil>"
			if newConversationState != nil {
				stateStr = fmt.Sprintf("%v", newConversationState)
			}
			tcIDs := make([]string, len(toolCalls))
			for i, tc := range toolCalls {
				tcIDs[i] = fmt.Sprintf("%s(item=%s)", tc.ID, tc.ItemID)
			}
			s.logger.Info(ctx, "tool-calling loop round completed",
				observability.Int("round", round),
				observability.Int("toolCalls", len(toolCalls)),
				observability.String("conversationState", stateStr),
				observability.String("toolCallIDs", strings.Join(tcIDs, ", ")),
			)
		}

		if len(toolCalls) == 0 {
			content := fullContent.String()
			if isLLMRefusal(content) {
				refusalErr := errors.NewAppError(ErrCodeLLMRefusal, "LLM returned a refusal instead of analysis").
					WithContext("content", truncateToolResult(content, 200))
				ai.RecordCall(ctx, s.tracker, s.logger, operation, refusalErr, start, lastRound+1, &totalUsage)
				return "", refusalErr
			}
			ai.RecordCall(ctx, s.tracker, s.logger, operation, nil, start, lastRound+1, &totalUsage)
			return content, nil
		}

		lastToolNames = lastToolNames[:0]
		for _, tc := range toolCalls {
			lastToolNames = append(lastToolNames, tc.Name)
		}

		// If the provider returned a non-empty conversation state, it manages
		// history internally. Chain via state and clear local messages to avoid
		// re-sending full history. We still append the assistant message with
		// tool calls — the adapter uses this to build function_call +
		// function_call_output input items.
		//
		// Guard against empty strings: an empty state would clear messages
		// but fail to enable chaining (PreviousResponseID = ""), causing the
		// next round to re-send function_call items with server-generated IDs
		// that the API already has — triggering a "Duplicate item" 400 error.
		if id, ok := newConversationState.(string); ok && id != "" {
			conversationState = newConversationState
			messages = nil
		} else if newConversationState != nil && !ok {
			// Non-string state (future-proofing) — trust it as-is.
			conversationState = newConversationState
			messages = nil
		}

		messages = append(messages, Message{
			Role:      RoleAssistant,
			Content:   fullContent.String(),
			ToolCalls: toolCalls,
		})

		// Execute tool calls concurrently — tools are safe for parallel execution.
		type toolResult struct {
			tc     ToolCall
			result string
		}
		results := make([]toolResult, len(toolCalls))
		var wg sync.WaitGroup
		for i, tc := range toolCalls {
			wg.Add(1)
			go func(idx int, call ToolCall) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						errMsg := fmt.Sprintf(`{"error": "panic: %v"}`, r)
						results[idx] = toolResult{tc: call, result: errMsg}
						if s.logger != nil {
							s.logger.Error(ctx, "tool execution panicked",
								observability.String("tool", call.Name),
								observability.String("panic", fmt.Sprintf("%v", r)),
							)
						}
					}
				}()
				toolCtx, toolCancel := context.WithTimeout(ctx, toolCallTimeout)
				res, execErr := s.executor.Execute(toolCtx, call.Name, call.Arguments)
				toolCancel()
				if execErr != nil {
					res = fmt.Sprintf(`{"error": %q}`, execErr.Error())
					if s.logger != nil {
						s.logger.Warn(ctx, "tool execution failed",
							observability.String("tool", call.Name),
							observability.Err(execErr),
						)
					}
				}
				res = truncateToolResult(res, maxToolResultChars)
				results[idx] = toolResult{tc: call, result: res}
			}(i, tc)
		}

		toolNames := make([]string, len(toolCalls))
		for i, tc := range toolCalls {
			toolNames[i] = tc.Name
		}
		toolSummary := strings.Join(toolNames, ",")
		stream(StreamEvent{Type: EventToolStart, ToolName: toolSummary})
		wg.Wait()
		stream(StreamEvent{Type: EventToolResult, ToolName: toolSummary})

		for _, tr := range results {
			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    tr.result,
				ToolCallID: tr.tc.ID,
			})
		}
	}

	err := errors.NewAppError(ErrCodeMaxRoundsExceeded, "exceeded maximum tool call rounds").
		WithContext("maxRounds", maxRounds).WithContext("lastTools", strings.Join(lastToolNames, ", "))
	ai.RecordCall(ctx, s.tracker, s.logger, operation, err, start, maxRounds, &totalUsage)
	return "", err
}
