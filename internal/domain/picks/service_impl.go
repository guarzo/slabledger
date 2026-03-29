package picks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type service struct {
	repo          Repository
	llm           ai.LLMProvider
	profitability ProfitabilityProvider
	inventory     InventoryProvider
	logger        observability.Logger
}

// NewService creates a new picks service.
func NewService(
	repo Repository,
	llm ai.LLMProvider,
	profitability ProfitabilityProvider,
	inventory InventoryProvider,
	logger observability.Logger,
) Service {
	return &service{
		repo:          repo,
		llm:           llm,
		profitability: profitability,
		inventory:     inventory,
		logger:        logger,
	}
}

// GenerateDailyPicks orchestrates the 3-stage picks pipeline:
//  1. Gather context (profitability profile, held cards, active watchlist)
//  2. LLM candidate generation (~20-30 candidates)
//  3. LLM scoring & ranking (top 10 picks)
func (s *service) GenerateDailyPicks(ctx context.Context) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)

	// Idempotency check.
	exists, err := s.repo.PicksExistForDate(ctx, today)
	if err != nil {
		return fmt.Errorf("check picks exist: %w", err)
	}
	if exists {
		return ErrPicksAlreadyExist
	}

	// --- Stage 1: Gather context (non-fatal on individual errors) ---
	profile, err := s.profitability.GetProfitablePatterns(ctx)
	if err != nil {
		s.logger.Warn(ctx, "failed to fetch profitability profile, proceeding with empty", observability.Err(err))
		profile = ProfitabilityProfile{}
	}

	heldCards, err := s.inventory.GetHeldCardNames(ctx)
	if err != nil {
		s.logger.Warn(ctx, "failed to fetch held card names, proceeding with empty", observability.Err(err))
		heldCards = nil
	}

	watchlist, err := s.repo.GetActiveWatchlist(ctx)
	if err != nil {
		s.logger.Warn(ctx, "failed to fetch active watchlist, proceeding with empty", observability.Err(err))
		watchlist = nil
	}

	s.logger.Info(ctx, "picks context gathered",
		observability.Int("held_cards", len(heldCards)),
		observability.Int("watchlist_items", len(watchlist)),
	)

	// --- Stage 2: LLM candidate generation ---
	candidatePrompt := BuildCandidatePrompt(profile, watchlist, heldCards)
	candidateRaw, err := s.callLLM(ctx, candidatePrompt)
	if err != nil {
		return fmt.Errorf("%w: candidate generation: %w", ErrLLMFailure, err)
	}

	candidates, err := ParseCandidates(candidateRaw)
	if err != nil {
		s.logger.Warn(ctx, "candidate JSON parse failed, retrying", observability.Err(err))
		candidateRaw, err = s.retryWithFix(ctx, candidatePrompt, candidateRaw)
		if err != nil {
			return fmt.Errorf("%w: candidate retry: %w", ErrLLMFailure, err)
		}
		candidates, err = ParseCandidates(candidateRaw)
		if err != nil {
			return fmt.Errorf("%w: candidate JSON malformed after retry: %w", ErrLLMFailure, err)
		}
	}

	s.logger.Info(ctx, "candidates generated", observability.Int("count", len(candidates)))

	// --- Stage 3: LLM scoring & ranking ---
	scoringPrompt := BuildScoringPrompt(candidates, profile)
	scoredRaw, err := s.callLLM(ctx, scoringPrompt)
	if err != nil {
		return fmt.Errorf("%w: scoring: %w", ErrLLMFailure, err)
	}

	picks, err := ParseScoredPicks(scoredRaw)
	if err != nil {
		s.logger.Warn(ctx, "scoring JSON parse failed, retrying", observability.Err(err))
		scoredRaw, err = s.retryWithFix(ctx, scoringPrompt, scoredRaw)
		if err != nil {
			return fmt.Errorf("%w: scoring retry: %w", ErrLLMFailure, err)
		}
		picks, err = ParseScoredPicks(scoredRaw)
		if err != nil {
			return fmt.Errorf("%w: scoring JSON malformed after retry: %w", ErrLLMFailure, err)
		}
	}

	// Stamp date and source on every pick.
	for i := range picks {
		picks[i].Date = today
		picks[i].Source = SourceAI
	}

	s.logger.Info(ctx, "picks scored", observability.Int("count", len(picks)))

	// --- Persist picks ---
	if err := s.repo.SavePicks(ctx, picks); err != nil {
		return fmt.Errorf("save picks: %w", err)
	}

	// --- Auto-watchlist high-confidence buys ---
	for i := range picks {
		if picks[i].Direction == DirectionBuy && picks[i].Confidence == ConfidenceHigh {
			item := WatchlistItem{
				CardName: picks[i].CardName,
				SetName:  picks[i].SetName,
				Grade:    picks[i].Grade,
				Source:   WatchlistAutoFromPick,
				Active:   true,
			}
			if addErr := s.repo.SaveWatchlistItem(ctx, item); addErr != nil {
				s.logger.Warn(ctx, "auto-watchlist save failed",
					observability.String("card", picks[i].CardName),
					observability.Err(addErr),
				)
			}
		}
	}

	// --- Update watchlist assessments for matched items ---
	s.updateWatchlistAssessments(ctx, picks, watchlist)

	return nil
}

// updateWatchlistAssessments links saved picks back to watchlist items by card name.
func (s *service) updateWatchlistAssessments(ctx context.Context, picks []Pick, watchlist []WatchlistItem) {
	if len(watchlist) == 0 {
		return
	}

	// Build a lookup of card name -> pick ID (only persisted picks have IDs assigned by repo).
	// We re-fetch today's picks to get IDs.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	savedPicks, err := s.repo.GetPicksByDate(ctx, today)
	if err != nil {
		s.logger.Warn(ctx, "could not fetch saved picks for watchlist assessment", observability.Err(err))
		return
	}

	pickByCard := make(map[string]Pick, len(savedPicks))
	for _, p := range savedPicks {
		key := normalizeCardKey(p.CardName, p.SetName, p.Grade)
		pickByCard[key] = p
	}

	for _, w := range watchlist {
		key := normalizeCardKey(w.CardName, w.SetName, w.Grade)
		if pick, ok := pickByCard[key]; ok {
			if updErr := s.repo.UpdateWatchlistAssessment(ctx, w.ID, pick.ID); updErr != nil {
				s.logger.Warn(ctx, "failed to update watchlist assessment",
					observability.Int("watchlist_id", w.ID),
					observability.Err(updErr),
				)
			}
		}
	}
}

// normalizeCardKey returns a lowercase composite key for card matching.
func normalizeCardKey(cardName, setName, grade string) string {
	return strings.ToLower(cardName) + "|" + strings.ToLower(setName) + "|" + strings.ToLower(grade)
}

// callLLM streams a completion from the LLM and returns the full response text.
func (s *service) callLLM(ctx context.Context, systemPrompt string) (string, error) {
	var response string
	err := s.llm.StreamCompletion(ctx, ai.CompletionRequest{
		SystemPrompt: systemPrompt,
		Messages:     []ai.Message{{Role: ai.RoleUser, Content: "Generate the recommendations now."}},
		MaxTokens:    4096,
	}, func(chunk ai.CompletionChunk) {
		response += chunk.Delta
	})
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}
	return response, nil
}

// retryWithFix sends the malformed response back as an assistant message and
// asks the LLM to correct it, returning the new raw response.
func (s *service) retryWithFix(ctx context.Context, systemPrompt, badResponse string) (string, error) {
	const fixPrompt = "Your previous response was not valid JSON. Please return ONLY a valid JSON array with no markdown fences or extra text."

	var response string
	err := s.llm.StreamCompletion(ctx, ai.CompletionRequest{
		SystemPrompt: systemPrompt,
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: "Generate the recommendations now."},
			{Role: ai.RoleAssistant, Content: badResponse},
			{Role: ai.RoleUser, Content: fixPrompt},
		},
		MaxTokens: 4096,
	}, func(chunk ai.CompletionChunk) {
		response += chunk.Delta
	})
	if err != nil {
		return "", fmt.Errorf("LLM retry call failed: %w", err)
	}
	return response, nil
}

// GetLatestPicks returns the most recent set of picks.
func (s *service) GetLatestPicks(ctx context.Context) ([]Pick, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	picks, err := s.repo.GetPicksByDate(ctx, today)
	if err != nil {
		return nil, fmt.Errorf("get latest picks: %w", err)
	}
	return picks, nil
}

// GetPickHistory returns picks across a date range.
func (s *service) GetPickHistory(ctx context.Context, days int) ([]Pick, error) {
	to := time.Now().UTC().Truncate(24 * time.Hour)
	from := to.AddDate(0, 0, -days)
	picks, err := s.repo.GetPicksRange(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("get pick history: %w", err)
	}
	return picks, nil
}

// AddToWatchlist adds a card to the watchlist.
func (s *service) AddToWatchlist(ctx context.Context, item WatchlistItem) error {
	if err := s.repo.SaveWatchlistItem(ctx, item); err != nil {
		return fmt.Errorf("add to watchlist: %w", err)
	}
	return nil
}

// RemoveFromWatchlist deactivates a watchlist item by ID.
func (s *service) RemoveFromWatchlist(ctx context.Context, id int) error {
	if err := s.repo.DeleteWatchlistItem(ctx, id); err != nil {
		return fmt.Errorf("remove from watchlist: %w", err)
	}
	return nil
}

// GetWatchlist returns all active watchlist items.
func (s *service) GetWatchlist(ctx context.Context) ([]WatchlistItem, error) {
	items, err := s.repo.GetActiveWatchlist(ctx)
	if err != nil {
		return nil, fmt.Errorf("get watchlist: %w", err)
	}
	return items, nil
}
