package advisor

import "github.com/guarzo/slabledger/internal/domain/ai"

type AIOperation = ai.AIOperation

const (
	OpDigest             = ai.OpDigest
	OpCampaignAnalysis   = ai.OpCampaignAnalysis
	OpLiquidation        = ai.OpLiquidation
	OpPurchaseAssessment = ai.OpPurchaseAssessment
	OpSocialCaption      = ai.OpSocialCaption
	OpSocialSuggestion   = ai.OpSocialSuggestion
)

type AIStatus = ai.AIStatus

const (
	AIStatusSuccess     = ai.AIStatusSuccess
	AIStatusError       = ai.AIStatusError
	AIStatusRateLimited = ai.AIStatusRateLimited
)

type AICallRecord = ai.AICallRecord
type AIOperationStats = ai.AIOperationStats
type AIUsageStats = ai.AIUsageStats
type AICallTracker = ai.AICallTracker

var (
	AIOperations    = ai.AIOperations
	ClassifyAIError = ai.ClassifyAIError
	NewAICallRecord = ai.NewAICallRecord
	EstimateCost    = ai.EstimateCost
)
