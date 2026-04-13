package advisor

// This file re-exports types and constants from internal/domain/ai that are part
// of the advisor package's public surface. Callers should use these aliases so
// they can depend on advisor without importing ai directly.

import "github.com/guarzo/slabledger/internal/domain/ai"

// LLM provider and request/response types.
type (
	LLMProvider       = ai.LLMProvider
	CompletionRequest = ai.CompletionRequest
	CompletionChunk   = ai.CompletionChunk
	TokenUsage        = ai.TokenUsage
	ToolDefinition    = ai.ToolDefinition
)

// Tool executor types.
type (
	ToolExecutor         = ai.ToolExecutor
	FilteredToolExecutor = ai.FilteredToolExecutor
)

// AI call tracking types and constants.
type (
	AIOperation      = ai.AIOperation
	AIStatus         = ai.AIStatus
	AICallRecord     = ai.AICallRecord
	AIOperationStats = ai.AIOperationStats
	AIUsageStats     = ai.AIUsageStats
	AICallTracker    = ai.AICallTracker
)

const (
	OpDigest             = ai.OpDigest
	OpCampaignAnalysis   = ai.OpCampaignAnalysis
	OpLiquidation        = ai.OpLiquidation
	OpPurchaseAssessment = ai.OpPurchaseAssessment
	OpSocialCaption      = ai.OpSocialCaption
	OpSocialSuggestion   = ai.OpSocialSuggestion
)

const (
	AIStatusSuccess     = ai.AIStatusSuccess
	AIStatusError       = ai.AIStatusError
	AIStatusRateLimited = ai.AIStatusRateLimited
)

var (
	AIOperations    = ai.AIOperations
	ClassifyAIError = ai.ClassifyAIError
	NewAICallRecord = ai.NewAICallRecord
	EstimateCost    = ai.EstimateCost
)
