package ai

import (
	"context"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// AIOperation identifies the type of AI API call.
type AIOperation string

const (
	OpDigest             AIOperation = "digest"
	OpCampaignAnalysis   AIOperation = "campaign_analysis"
	OpLiquidation        AIOperation = "liquidation"
	OpPurchaseAssessment AIOperation = "purchase_assessment"
	OpSocialCaption      AIOperation = "social_caption"
	OpSocialSuggestion   AIOperation = "social_suggestion"
)

// AIStatus classifies the outcome of an AI API call.
type AIStatus string

const (
	AIStatusSuccess     AIStatus = "success"
	AIStatusError       AIStatus = "error"
	AIStatusRateLimited AIStatus = "rate_limited"
)

// AICallRecord captures a single AI API invocation.
type AICallRecord struct {
	Operation         AIOperation
	Status            AIStatus
	ErrorMessage      string
	LatencyMS         int64
	ToolRounds        int
	InputTokens       int
	OutputTokens      int
	TotalTokens       int
	CostEstimateCents int
	Timestamp         time.Time
}

// NewAICallRecord constructs a record with status classification, latency, and timestamp computed automatically.
func NewAICallRecord(operation AIOperation, callErr error, start time.Time, rounds int, usage *TokenUsage) *AICallRecord {
	status, errMsg := ClassifyAIError(callErr)
	var in, out, total int
	if usage != nil {
		in = usage.InputTokens
		out = usage.OutputTokens
		total = usage.TotalTokens
	}
	return &AICallRecord{
		Operation:         operation,
		Status:            status,
		ErrorMessage:      errMsg,
		LatencyMS:         time.Since(start).Milliseconds(),
		ToolRounds:        rounds,
		InputTokens:       in,
		OutputTokens:      out,
		TotalTokens:       total,
		CostEstimateCents: EstimateCost(in, out),
		Timestamp:         time.Now(),
	}
}

// AIOperationStats holds per-operation aggregate metrics.
type AIOperationStats struct {
	Calls          int64   `json:"calls"`
	Errors         int64   `json:"errors"`
	AvgLatencyMS   float64 `json:"avgLatencyMs"`
	TotalTokens    int64   `json:"totalTokens"`
	TotalCostCents int64   `json:"totalCostCents"`
}

// AIUsageStats holds aggregate AI usage metrics.
// The SQLite implementation uses a rolling 7-day window defined in the ai_usage_summary view.
type AIUsageStats struct {
	TotalCalls        int64
	SuccessCalls      int64
	ErrorCalls        int64
	RateLimitHits     int64
	AvgLatencyMS      float64
	TotalInputTokens  int64
	TotalOutputTokens int64
	TotalTokens       int64
	TotalCostCents    int64
	LastCallAt        *time.Time
	CallsLast24h      int64
	ByOperation       map[AIOperation]*AIOperationStats
}

// AICallTracker records and queries AI call metrics.
type AICallTracker interface {
	RecordAICall(ctx context.Context, call *AICallRecord) error
	GetAIUsage(ctx context.Context) (*AIUsageStats, error)
}

// AIOperations lists all known AI operation names in display order.
// Must stay in sync with the CHECK constraint in migration 000015_ai_calls.
var AIOperations = []AIOperation{OpDigest, OpCampaignAnalysis, OpLiquidation, OpPurchaseAssessment, OpSocialCaption, OpSocialSuggestion}

// ClassifyAIError returns (StatusSuccess, "") for nil errors, or (StatusError/StatusRateLimited, errMsg) for failures.
func ClassifyAIError(err error) (AIStatus, string) {
	if err == nil {
		return AIStatusSuccess, ""
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "too_many_requests") || strings.Contains(errMsg, "429") {
		return AIStatusRateLimited, errMsg
	}
	return AIStatusError, errMsg
}

// EstimateCost computes a cost estimate in cents from token counts.
// Uses Azure OpenAI GPT-4o-class pricing: $2.50/1M input, $10/1M output.
// Always returns at least 1 cent when any tokens are used so sub-cent
// calls are not lost when aggregated via SUM.
func EstimateCost(inputTokens, outputTokens int) int {
	if inputTokens == 0 && outputTokens == 0 {
		return 0
	}
	inputCost := float64(inputTokens) * 0.00025
	outputCost := float64(outputTokens) * 0.001
	cents := int(inputCost + outputCost + 0.5)
	if cents < 1 {
		return 1
	}
	return cents
}

// RecordCall persists an AI call record if a tracker is configured.
// Uses a detached context with a 5-second timeout so telemetry is
// recorded even when the caller's context is already cancelled.
func RecordCall(
	_ context.Context,
	tracker AICallTracker,
	logger observability.Logger,
	operation AIOperation,
	callErr error,
	start time.Time,
	rounds int,
	usage *TokenUsage,
) {
	if tracker == nil {
		return
	}

	record := NewAICallRecord(operation, callErr, start, rounds, usage)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tracker.RecordAICall(ctx, record); err != nil && logger != nil {
		logger.Error(ctx, "failed to record AI call",
			observability.Err(err),
			observability.String("operation", string(operation)))
	}
}
