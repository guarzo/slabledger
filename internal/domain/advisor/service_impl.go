package advisor

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/scoring"
)

const (
	defaultMaxToolRounds = 3
	// defaultMaxTokens is the output token limit per LLM round. Digest and
	// liquidation analyses call 10-14 tools and then produce a comprehensive
	// multi-section report. 16 384 tokens was still too small for large
	// flagged inventories, causing Azure to return "status incomplete" on
	// the final analysis round. 32 768 gives ample room for detailed tables.
	defaultMaxTokens   = 32_768
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
var operationTools = map[AIOperation][]string{
	OpDigest: {
		"get_dashboard_summary", "get_weekly_review", "get_global_inventory",
		"get_portfolio_insights", "get_flagged_inventory", "get_inventory_alerts",
		"get_acquisition_targets", "get_deslab_opportunities", "get_dh_suggestions",
		"get_expected_values_batch",
		"get_campaign_tuning", "get_campaign_pnl",
	},
	OpCampaignAnalysis: {
		"get_campaign_pnl", "get_pnl_by_channel",
		"get_campaign_tuning", "get_inventory_aging",
		"get_expected_values", "get_deslab_candidates",
	},
	OpLiquidation: {
		"get_dashboard_summary", "get_flagged_inventory",
		"get_suggestion_stats", "get_inventory_alerts",
		"get_expected_values_batch", "suggest_price_batch",
	},
}

type service struct {
	llm           LLMProvider
	executor      ToolExecutor
	logger        observability.Logger
	tracker       AICallTracker
	cache         CacheStore
	scoringData   ScoringDataProvider
	gapStore      scoring.GapStore
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
