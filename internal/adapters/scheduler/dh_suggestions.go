package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*DHSuggestionsScheduler)(nil)

// DHSuggestionsConfig holds configuration for the DH suggestions scheduler.
type DHSuggestionsConfig struct {
	Enabled  bool
	Interval time.Duration // default 6h
}

// DHSuggestionsScheduler periodically fetches daily buy/sell suggestions from
// the DH API and stores them locally.
type DHSuggestionsScheduler struct {
	StopHandle
	dhClient    *dh.Client
	suggestRepo intelligence.SuggestionsRepository
	logger      observability.Logger
	config      DHSuggestionsConfig
}

// NewDHSuggestionsScheduler creates a new DH suggestions scheduler.
func NewDHSuggestionsScheduler(
	dhClient *dh.Client,
	suggestRepo intelligence.SuggestionsRepository,
	logger observability.Logger,
	config DHSuggestionsConfig,
) *DHSuggestionsScheduler {
	if config.Interval == 0 {
		config.Interval = 6 * time.Hour
	}

	return &DHSuggestionsScheduler{
		StopHandle:  NewStopHandle(),
		dhClient:    dhClient,
		suggestRepo: suggestRepo,
		logger:      logger.With(context.Background(), observability.String("component", "dh-suggestions")),
		config:      config,
	}
}

// Start begins the background suggestions fetch scheduler.
func (s *DHSuggestionsScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "DH suggestions scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-suggestions",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.fetch)
}

// fetch retrieves suggestions from the DH API and stores them.
func (s *DHSuggestionsScheduler) fetch(ctx context.Context) {
	s.logger.Debug(ctx, "fetching DH suggestions")

	resp, err := s.dhClient.Suggestions(ctx)
	if err != nil {
		s.logger.Error(ctx, "failed to fetch DH suggestions", observability.Err(err))
		return
	}

	now := time.Now()
	suggestions := convertSuggestionsResponse(resp, now)

	if len(suggestions) == 0 {
		s.logger.Debug(ctx, "no suggestions returned from DH")
		return
	}

	if storeErr := s.suggestRepo.StoreSuggestions(ctx, suggestions); storeErr != nil {
		s.logger.Error(ctx, "failed to store DH suggestions", observability.Err(storeErr))
		return
	}

	s.logger.Info(ctx, "DH suggestions stored",
		observability.Int("count", len(suggestions)),
		observability.String("date", resp.SuggestionDate))
}

// convertSuggestionsResponse flattens a SuggestionsResponse into a slice of domain suggestions.
func convertSuggestionsResponse(resp *dh.SuggestionsResponse, fetchedAt time.Time) []intelligence.Suggestion {
	var suggestions []intelligence.Suggestion

	type groupDef struct {
		group    dh.SuggestionGroup
		typeName string
	}

	groups := []groupDef{
		{group: resp.Cards, typeName: "cards"},
		{group: resp.Sealed, typeName: "sealed"},
	}

	for _, g := range groups {
		type categoryDef struct {
			items    []dh.SuggestionItem
			category string
		}
		categories := []categoryDef{
			{items: g.group.HottestCards, category: "hottest_cards"},
			{items: g.group.ConsiderSelling, category: "consider_selling"},
		}

		for _, cat := range categories {
			for _, item := range cat.items {
				s := intelligence.Suggestion{
					SuggestionDate:    resp.SuggestionDate,
					Type:              g.typeName,
					Category:          cat.category,
					Rank:              item.Rank,
					IsManual:          item.IsManual,
					DHCardID:          fmt.Sprintf("%d", item.Card.ID),
					CardName:          item.Card.Name,
					SetName:           item.Card.SetName,
					CardNumber:        item.Card.Number,
					ImageURL:          item.Card.ImageURL,
					CurrentPriceCents: mathutil.ToCents(item.Card.CurrentPrice),
					ConfidenceScore:   item.ConfidenceScore,
					Reasoning:         item.Reasoning,
					FetchedAt:         fetchedAt,
				}

				if item.StructuredReasoning != nil {
					if b, err := json.Marshal(item.StructuredReasoning); err == nil {
						s.StructuredReasoning = string(b)
					}
				}
				if item.Metrics != nil {
					if b, err := json.Marshal(item.Metrics); err == nil {
						s.Metrics = string(b)
					}
				}
				if item.Sentiment != nil {
					s.SentimentScore = item.Sentiment.Score
					s.SentimentTrend = item.Sentiment.Trend
					s.SentimentMentions = item.Sentiment.MentionCount
				}

				suggestions = append(suggestions, s)
			}
		}
	}

	return suggestions
}
