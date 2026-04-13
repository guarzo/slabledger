package advisor

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/scoring"
)

// Service defines the AI advisor analysis capabilities.
// Streaming methods (Generate*, Analyze*, Assess*) emit events via callback.
// Collect methods return the full markdown result synchronously.
type Service interface {
	// GenerateDigest produces a narrative weekly intelligence digest.
	GenerateDigest(ctx context.Context, stream func(StreamEvent)) error

	// AnalyzeCampaign produces a health and tuning narrative for a campaign.
	AnalyzeCampaign(ctx context.Context, campaignID string, stream func(StreamEvent)) error

	// AnalyzeLiquidation identifies cards to consider selling now with pricing.
	AnalyzeLiquidation(ctx context.Context, stream func(StreamEvent)) error

	// AssessPurchase scores a potential purchase with reasoning.
	AssessPurchase(ctx context.Context, req PurchaseAssessmentRequest, stream func(StreamEvent)) error

	// CollectDigest runs the digest analysis and returns the full markdown result.
	CollectDigest(ctx context.Context) (string, error)
	// CollectLiquidation runs liquidation analysis and returns the full markdown result.
	CollectLiquidation(ctx context.Context) (string, error)
}

// ServiceOption configures optional service dependencies.
type ServiceOption func(*service)

// WithLogger enables structured logging for the advisor service.
func WithLogger(l observability.Logger) ServiceOption {
	return func(s *service) { s.logger = l }
}

// WithMaxToolRounds sets the maximum number of tool-calling iterations.
// Defaults to 5 if not set.
func WithMaxToolRounds(n int) ServiceOption {
	return func(s *service) { s.maxToolRounds = n }
}

// WithMaxTokens sets the max tokens for LLM completion requests.
func WithMaxTokens(n int) ServiceOption {
	return func(s *service) { s.maxTokens = n }
}

// WithTemperature sets the temperature for LLM completion requests.
func WithTemperature(t float64) ServiceOption {
	return func(s *service) { s.temperature = t }
}

// WithAITracker enables recording of AI call metrics.
func WithAITracker(t AICallTracker) ServiceOption {
	return func(s *service) { s.tracker = t }
}

// WithCacheStore enables injecting prior analysis context into prompts.
func WithCacheStore(c CacheStore) ServiceOption {
	return func(s *service) { s.cache = c }
}

// ScoringDataProvider gathers raw data needed by factor computers.
type ScoringDataProvider interface {
	PurchaseData(ctx context.Context, req PurchaseAssessmentRequest) (*PurchaseFactorData, error)
	CampaignData(ctx context.Context, campaignID string) (*CampaignFactorData, error)
}

// WithScoringDataProvider injects the data provider used by the scoring orchestrator.
func WithScoringDataProvider(p ScoringDataProvider) ServiceOption {
	return func(s *service) { s.scoringData = p }
}

// WithGapStore injects the gap store for recording data gaps during scoring.
func WithGapStore(gs scoring.GapStore) ServiceOption {
	return func(s *service) { s.gapStore = gs }
}
