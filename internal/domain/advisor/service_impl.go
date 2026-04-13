package advisor

import (
	"context"
	"encoding/json"
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
	OpPurchaseAssessment: {
		"get_campaign_tuning", "get_cert_lookup",
		"evaluate_purchase", "get_campaign_pnl",
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

	var scoreCard *scoring.ScoreCard
	if s.scoringData != nil {
		data, err := s.scoringData.CampaignData(ctx, campaignID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "AnalyzeCampaign: failed to load scoring data — proceeding without scorecard",
					observability.String("campaignID", campaignID),
					observability.Err(err))
			}
		} else if data != nil {
			sc, scErr := BuildScoreCard(campaignID, "campaign", data, scoring.CampaignAnalysisProfile)
			if scErr != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "AnalyzeCampaign: failed to build scorecard — proceeding without",
						observability.String("campaignID", campaignID),
						observability.Err(scErr))
				}
			} else {
				scoreCard = &sc
				s.recordGaps(ctx, sc, "", "")
			}
		}
	}

	if scoreCard == nil {
		_, err := s.runAnalysis(ctx, OpCampaignAnalysis, campaignAnalysisSystemPrompt, userPrompt, stream)
		return err
	}

	_, err := s.runScoredAnalysis(ctx, OpCampaignAnalysis, campaignAnalysisSystemPrompt, userPrompt, scoreCard, campaignAnalysisSchema, stream)
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

	var scoreCard *scoring.ScoreCard
	if s.scoringData != nil {
		data, err := s.scoringData.PurchaseData(ctx, req)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "AssessPurchase: failed to load scoring data — proceeding without scorecard",
					observability.String("certNumber", req.CertNumber),
					observability.Err(err))
			}
		} else if data != nil {
			sc, scErr := BuildScoreCard(req.CertNumber, "purchase", data, scoring.PurchaseAssessmentProfile)
			if scErr != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "AssessPurchase: failed to build scorecard — proceeding without",
						observability.String("certNumber", req.CertNumber),
						observability.Err(scErr))
				}
			} else {
				scoreCard = &sc
				s.recordGaps(ctx, sc, req.CardName, req.SetName)
			}
		}
	}

	if scoreCard == nil {
		_, err := s.runAnalysis(ctx, OpPurchaseAssessment, purchaseAssessmentSystemPrompt, userPrompt, stream)
		return err
	}

	_, err := s.runScoredAnalysis(ctx, OpPurchaseAssessment, purchaseAssessmentSystemPrompt, userPrompt, scoreCard, purchaseAssessmentSchema, stream)
	if err != nil {
		fallback := scoring.FallbackResult(*scoreCard)
		if fbJSON, fbErr := json.Marshal(fallback); fbErr == nil {
			stream(StreamEvent{Type: EventDelta, Content: string(fbJSON)})
			return nil
		}
	}
	return err
}

func (s *service) runAnalysis(ctx context.Context, operation AIOperation, systemPrompt, userPrompt string, stream func(StreamEvent)) (string, error) {
	messages := []Message{
		{Role: RoleUser, Content: userPrompt},
	}
	return s.toolCallingLoop(ctx, operation, systemPrompt, messages, stream)
}

// runScoredAnalysis augments the system prompt with a pre-computed ScoreCard and
// structured output schema, then delegates to the tool-calling loop.
func (s *service) runScoredAnalysis(ctx context.Context, operation AIOperation, baseSystemPrompt, userPrompt string, scoreCard *scoring.ScoreCard, schema string, stream func(StreamEvent)) (string, error) {
	sysPrompt := baseSystemPrompt
	if scoreCard != nil {
		if scoreJSON, err := json.Marshal(scoreCard); err == nil {
			stream(StreamEvent{Type: EventScore, Content: string(scoreJSON)})
			sysPrompt += fmt.Sprintf(scoreCardInjectionTemplate, string(scoreJSON))
		}
	}
	sysPrompt += fmt.Sprintf(structuredOutputInstruction, schema)
	messages := []Message{{Role: RoleUser, Content: userPrompt}}
	return s.toolCallingLoop(ctx, operation, sysPrompt, messages, stream)
}

// recordGaps persists data gaps from a ScoreCard in the background.
func (s *service) recordGaps(ctx context.Context, sc scoring.ScoreCard, cardName, setName string) {
	if s.gapStore == nil || len(sc.DataGaps) == 0 {
		return
	}
	records := make([]scoring.GapRecord, len(sc.DataGaps))
	for i, g := range sc.DataGaps {
		records[i] = scoring.GapRecord{
			FactorName: g.FactorName,
			Reason:     g.Reason,
			EntityType: sc.EntityType,
			EntityID:   sc.EntityID,
			CardName:   cardName,
			SetName:    setName,
		}
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.gapStore.RecordGaps(bgCtx, records); err != nil && s.logger != nil {
			s.logger.Warn(bgCtx, "failed to record data gaps",
				observability.Err(err),
				observability.String("entityID", sc.EntityID),
			)
		}
	}()
}

func (s *service) CollectDigest(ctx context.Context) (string, error) {
	sysPrompt := digestSystemPrompt + s.priorContext(ctx, AnalysisDigest)
	return s.runAnalysis(ctx, OpDigest, sysPrompt, digestUserPrompt, func(StreamEvent) {})
}

func (s *service) CollectLiquidation(ctx context.Context) (string, error) {
	sysPrompt := liquidationSystemPrompt + s.priorContext(ctx, AnalysisLiquidation)
	return s.runAnalysis(ctx, OpLiquidation, sysPrompt, liquidationUserPrompt, func(StreamEvent) {})
}
