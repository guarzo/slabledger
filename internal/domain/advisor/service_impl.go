package advisor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	defaultMaxToolRounds = 3
	// defaultMaxTokens is the output token limit per LLM round. Digest and
	// liquidation analyses call 10-14 tools and then produce a comprehensive
	// multi-section report. 4 096 tokens was consistently too small, causing
	// Azure to return "status incomplete" on the final analysis round.
	// 16 384 tokens ≈ 12 000 words — enough for a detailed weekly digest.
	defaultMaxTokens   = 16_384
	defaultTemperature = 0.3
	toolCallTimeout    = 30 * time.Second

	// maxToolResultChars caps individual tool results to prevent input token
	// bloat. Large tool outputs (full inventory, sell sheets) can push round-2
	// input tokens past 100K, causing Azure to time out and return "status
	// incomplete". 12 000 chars ≈ 3 000 tokens — enough for detailed data
	// while keeping total input manageable across multiple tool calls.
	maxToolResultChars = 12_000
)

// Tool subsets per operation — only send relevant tools to reduce prompt tokens
// and prevent the LLM from calling irrelevant tools.
var operationTools = map[string][]string{
	"digest": {
		"list_campaigns", "get_campaign_pnl", "get_pnl_by_channel",
		"get_campaign_tuning", "get_inventory_aging", "get_global_inventory",
		"get_sell_sheet", "get_portfolio_health", "get_portfolio_insights",
		"get_credit_summary", "get_weekly_review", "get_capital_timeline",
		"get_channel_velocity", "get_dashboard_summary",
		"get_acquisition_targets", "get_crack_opportunities",
	},
	"campaign_analysis": {
		"list_campaigns", "get_campaign_pnl", "get_pnl_by_channel",
		"get_campaign_tuning", "get_inventory_aging", "get_expected_values",
		"get_crack_candidates", "get_campaign_suggestions", "run_projection",
		"get_channel_velocity", "get_credit_summary",
	},
	"liquidation": {
		"list_campaigns", "get_global_inventory", "get_sell_sheet",
		"get_credit_summary", "get_expected_values", "get_inventory_aging",
		"get_portfolio_health", "suggest_price", "get_cert_lookup",
		"get_channel_velocity", "get_capital_timeline", "get_suggestion_stats",
		"get_dashboard_summary", "get_crack_opportunities",
	},
	"purchase_assessment": {
		"list_campaigns", "get_campaign_tuning", "get_portfolio_insights",
		"get_cert_lookup", "evaluate_purchase", "get_campaign_pnl",
		"get_channel_velocity",
	},
}

type service struct {
	llm           LLMProvider
	executor      ToolExecutor
	logger        observability.Logger
	tracker       AICallTracker
	cache         CacheStore
	maxToolRounds int
	maxTokens     int
	temperature   float64
}

// NewService creates a new advisor service.
func NewService(llm LLMProvider, executor ToolExecutor, opts ...ServiceOption) Service {
	s := &service{
		llm:           llm,
		executor:      executor,
		maxToolRounds: defaultMaxToolRounds,
		maxTokens:     defaultMaxTokens,
		temperature:   defaultTemperature,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

var _ Service = (*service)(nil)

func (s *service) GenerateDigest(ctx context.Context, stream func(StreamEvent)) error {
	sysPrompt := digestSystemPrompt + s.priorContext(ctx, AnalysisDigest)
	_, err := s.runAnalysis(ctx, OpDigest, sysPrompt, digestUserPrompt, stream)
	return err
}

func (s *service) AnalyzeCampaign(ctx context.Context, campaignID string, stream func(StreamEvent)) error {
	userPrompt := fmt.Sprintf(campaignAnalysisUserPrompt, campaignID)
	_, err := s.runAnalysis(ctx, OpCampaignAnalysis, campaignAnalysisSystemPrompt, userPrompt, stream)
	return err
}

func (s *service) AnalyzeLiquidation(ctx context.Context, stream func(StreamEvent)) error {
	sysPrompt := liquidationSystemPrompt + s.priorContext(ctx, AnalysisLiquidation)
	_, err := s.runAnalysis(ctx, OpLiquidation, sysPrompt, liquidationUserPrompt, stream)
	return err
}

func (s *service) AssessPurchase(ctx context.Context, req PurchaseAssessmentRequest, stream func(StreamEvent)) error {
	userPrompt := fmt.Sprintf(purchaseAssessmentUserPrompt,
		req.CardName, req.Grade, float64(req.BuyCostCents)/100,
		req.CampaignName, req.CampaignID, req.SetName, req.CertNumber, float64(req.CLValueCents)/100,
	)
	_, err := s.runAnalysis(ctx, OpPurchaseAssessment, purchaseAssessmentSystemPrompt, userPrompt, stream)
	return err
}

func (s *service) runAnalysis(ctx context.Context, operation AIOperation, systemPrompt, userPrompt string, stream func(StreamEvent)) (string, error) {
	messages := []Message{
		{Role: RoleUser, Content: userPrompt},
	}
	return s.toolCallingLoop(ctx, operation, systemPrompt, messages, stream)
}

func (s *service) CollectDigest(ctx context.Context) (string, error) {
	sysPrompt := digestSystemPrompt + s.priorContext(ctx, AnalysisDigest)
	return s.runAnalysis(ctx, OpDigest, sysPrompt, digestUserPrompt, func(StreamEvent) {})
}

func (s *service) CollectLiquidation(ctx context.Context) (string, error) {
	sysPrompt := liquidationSystemPrompt + s.priorContext(ctx, AnalysisLiquidation)
	return s.runAnalysis(ctx, OpLiquidation, sysPrompt, liquidationUserPrompt, func(StreamEvent) {})
}

// toolCallingLoop orchestrates the LLM -> tool -> LLM cycle.
func (s *service) toolCallingLoop(ctx context.Context, operation AIOperation, systemPrompt string, messages []Message, stream func(StreamEvent)) (string, error) {
	var tools []ToolDefinition
	if names, ok := operationTools[string(operation)]; ok {
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

	for round := 0; round < s.maxToolRounds; round++ {
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
			ai.RecordCall(ctx, s.tracker, s.logger, operation, nil, start, lastRound+1, &totalUsage)
			return fullContent.String(), nil
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

		stream(StreamEvent{Type: EventToolStart, ToolName: toolCalls[0].Name})
		wg.Wait()
		stream(StreamEvent{Type: EventToolResult, ToolName: toolCalls[0].Name})

		for _, tr := range results {
			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    tr.result,
				ToolCallID: tr.tc.ID,
			})
		}
	}

	err := fmt.Errorf("exceeded maximum tool rounds (%d); last tools called: %s",
		s.maxToolRounds, strings.Join(lastToolNames, ", "))
	ai.RecordCall(ctx, s.tracker, s.logger, operation, err, start, s.maxToolRounds, &totalUsage)
	return "", err
}

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
