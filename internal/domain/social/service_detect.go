package social

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) llmGenerate(ctx context.Context) (int, error) {
	cards, err := s.repo.GetAvailableCardsForPosts(ctx)
	if err != nil {
		return 0, fmt.Errorf("get available cards: %w", err)
	}
	if len(cards) < s.minCards {
		return 0, nil
	}

	// Cap card list to avoid huge prompts
	if len(cards) > 100 {
		cards = cards[:100]
	}

	prompt := buildPostSuggestionPrompt(cards)
	var result strings.Builder
	var usage ai.TokenUsage
	start := time.Now()

	err = s.llm.StreamCompletion(ctx, ai.CompletionRequest{
		SystemPrompt: postSuggestionSystemPrompt,
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: prompt},
		},
		MaxTokens: 2048,
	}, func(chunk ai.CompletionChunk) {
		result.WriteString(chunk.Delta)
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
	})
	if err != nil {
		ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialSuggestion, err, start, 0, &usage)
		return 0, fmt.Errorf("LLM call: %w", err)
	}
	ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialSuggestion, nil, start, 0, &usage)

	// Parse JSON response — strip markdown fences and fix control characters
	raw := sanitizeLLMJSON(stripMarkdownFences(result.String()))

	var resp postSuggestionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return 0, fmt.Errorf("parse LLM suggestions: %w", err)
	}

	// Build a lookup of available card IDs for validation
	cardMap := make(map[string]bool, len(cards))
	for _, c := range cards {
		cardMap[c.PurchaseID] = true
	}

	// Build card lookup once for deduplication across all suggestions
	cardLookup := make(map[string]PostCardDetail, len(cards))
	for _, c := range cards {
		cardLookup[c.PurchaseID] = c
	}

	type cardIdentityKey struct {
		name  string
		set   string
		grade float64
	}

	created := 0
	usedIDs := make(map[string]bool)                 // prevent purchase IDs from appearing in multiple posts
	usedIdentities := make(map[cardIdentityKey]bool) // prevent same logical card across posts

	for _, suggestion := range resp.Posts {
		// Validate and filter purchase IDs, also excluding cards whose identity is already used
		seen := make(map[string]bool)
		var validIDs []string
		for _, pid := range suggestion.PurchaseIDs {
			if !cardMap[pid] || usedIDs[pid] || seen[pid] {
				continue
			}
			if card, ok := cardLookup[pid]; ok {
				key := cardIdentityKey{name: card.CardName, set: card.SetName, grade: card.GradeValue}
				if usedIdentities[key] {
					continue
				}
			}
			seen[pid] = true
			validIDs = append(validIDs, pid)
		}

		// Deduplicate by card identity (name + set + grade) within this suggestion
		validIDs = deduplicateByCardIdentity(validIDs, cardLookup)

		if len(validIDs) < s.minCards {
			continue
		}
		if len(validIDs) > s.maxCards {
			validIDs = validIDs[:s.maxCards]
		}

		// Mark these IDs and identities as used
		for _, pid := range validIDs {
			usedIDs[pid] = true
			if card, ok := cardLookup[pid]; ok {
				usedIdentities[cardIdentityKey{name: card.CardName, set: card.SetName, grade: card.GradeValue}] = true
			}
		}

		// Create the post
		postID := generateID()
		pt := parsePostType(suggestion.PostType)
		if pt != PostType(suggestion.PostType) && s.logger != nil {
			s.logger.Warn(ctx, "social: LLM returned unrecognized postType, defaulting to new_arrivals",
				observability.String("raw", suggestion.PostType),
				observability.String("resolved", string(pt)))
		}
		post := &SocialPost{
			ID:         postID,
			PostType:   pt,
			Status:     PostStatusDraft,
			CoverTitle: suggestion.CoverTitle,
			CardCount:  len(validIDs),
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}

		if err := s.repo.CreatePost(ctx, post); err != nil {
			s.logError(ctx, "create LLM post", pt, err)
			continue
		}

		postCards := make([]PostCard, len(validIDs))
		for i, pid := range validIDs {
			postCards[i] = PostCard{PostID: postID, PurchaseID: pid, SlideOrder: i + 1}
		}
		if err := s.repo.AddPostCards(ctx, postID, postCards); err != nil {
			s.logError(ctx, "add cards to LLM post", pt, err)
			_ = s.repo.DeletePost(ctx, postID) //nolint:errcheck
			continue
		}

		// Generate caption in background
		s.safeGo("generateCaptionAsync", post.ID, func() { s.generateCaptionAsync(post) })
		s.safeGo("generateBackgroundsAsync", post.ID, func() { s.generateBackgroundsAsync(post) })
		created++
	}

	return created, nil
}

func (s *service) ruleBasedGenerate(ctx context.Context) (int, error) {
	created := 0

	snapshots, err := s.repo.GetUnsoldPurchasesWithSnapshots(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch snapshots: %w", err)
	}

	var lastErr error
	types := []PostType{PostTypeNewArrivals, PostTypePriceMovers, PostTypeHotDeals}
	for _, pt := range types {
		if ctx.Err() != nil {
			return created, ctx.Err()
		}
		post, err := s.detectPostType(ctx, pt, snapshots)
		if err != nil {
			if IsInsufficientCards(err) {
				continue // not enough cards — skip silently
			}
			s.logError(ctx, "detection failed", pt, err)
			lastErr = err
			continue
		}
		if post != nil {
			created++
		}
	}

	if created == 0 && lastErr != nil {
		return 0, fmt.Errorf("all detection types failed: %w", lastErr)
	}
	return created, nil
}

func (s *service) detectPostType(ctx context.Context, postType PostType, snapshots []PurchaseSnapshot) (*SocialPost, error) {
	var candidateIDs []string
	var err error

	switch postType {
	case PostTypeNewArrivals:
		candidateIDs, err = s.detectNewArrivals(ctx)
	case PostTypePriceMovers:
		candidateIDs = filterPriceMovers(snapshots)
	case PostTypeHotDeals:
		candidateIDs = filterHotDeals(snapshots)
	default:
		return nil, fmt.Errorf("unsupported post type: %s", postType)
	}
	if err != nil {
		return nil, err
	}

	if len(candidateIDs) < s.minCards {
		return nil, ErrInsufficientCards
	}

	// Deduplicate against existing posts
	existing, err := s.repo.GetPurchaseIDsInExistingPosts(ctx, candidateIDs, postType)
	if err != nil {
		return nil, fmt.Errorf("check existing posts: %w", err)
	}

	var filtered []string
	for _, id := range candidateIDs {
		if !existing[id] {
			filtered = append(filtered, id)
		}
	}

	// Deduplicate by card identity (name + set + grade)
	if len(filtered) > 0 {
		available, err := s.repo.GetAvailableCardsForPosts(ctx)
		if err == nil {
			cardLookup := make(map[string]PostCardDetail, len(available))
			for _, c := range available {
				cardLookup[c.PurchaseID] = c
			}
			filtered = deduplicateByCardIdentity(filtered, cardLookup)
		}
	}

	if len(filtered) < s.minCards {
		return nil, ErrInsufficientCards
	}
	if len(filtered) > s.maxCards {
		filtered = filtered[:s.maxCards]
	}

	// Create the post
	postID := generateID()
	cardCount := len(filtered)
	post := &SocialPost{
		ID:         postID,
		PostType:   postType,
		Status:     PostStatusDraft,
		CoverTitle: buildCoverTitle(postType, cardCount),
		CardCount:  cardCount,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := s.repo.CreatePost(ctx, post); err != nil {
		return nil, fmt.Errorf("create post: %w", err)
	}

	// Add card associations
	cards := make([]PostCard, len(filtered))
	for i, pid := range filtered {
		cards[i] = PostCard{
			PostID:     postID,
			PurchaseID: pid,
			SlideOrder: i + 1,
		}
	}
	if err := s.repo.AddPostCards(ctx, postID, cards); err != nil {
		_ = s.repo.DeletePost(ctx, postID) //nolint:errcheck // best-effort cleanup
		return nil, fmt.Errorf("add post cards: %w", err)
	}

	// Generate AI caption in background — don't block the HTTP request
	s.safeGo("generateCaptionAsync", post.ID, func() { s.generateCaptionAsync(post) })
	s.safeGo("generateBackgroundsAsync", post.ID, func() { s.generateBackgroundsAsync(post) })

	return post, nil
}

func (s *service) detectNewArrivals(ctx context.Context) ([]string, error) {
	since := time.Now().UTC().AddDate(0, 0, -newArrivalsWindow).Format("2006-01-02 15:04:05")
	return s.repo.GetRecentPurchaseIDs(ctx, since)
}

func filterPriceMovers(snapshots []PurchaseSnapshot) []string {
	cutoff := time.Now().UTC().AddDate(0, 0, -maxSnapshotAgeDays).Format("2006-01-02")
	var ids []string
	for _, snap := range snapshots {
		if snap.MedianCents == 0 {
			continue
		}
		// Use DH trend when available, fall back to MM trend for cards without a DH snapshot
		trend := snap.Trend30d
		if trend == 0 && snap.MMTrendPct != 0 {
			trend = snap.MMTrendPct
		}
		// Only require a fresh DH snapshot when relying on it; MM trend is always fresh
		if snap.Trend30d != 0 && snap.SnapshotDate < cutoff {
			continue
		}
		if math.Abs(trend) >= priceChangeThreshold {
			ids = append(ids, snap.PurchaseID)
		}
	}
	return ids
}

func filterHotDeals(snapshots []PurchaseSnapshot) []string {
	cutoff := time.Now().UTC().AddDate(0, 0, -maxSnapshotAgeDays).Format("2006-01-02")
	var ids []string
	for _, snap := range snapshots {
		if snap.SnapshotDate < cutoff || snap.MedianCents == 0 || snap.BuyCostCents == 0 {
			continue
		}
		if float64(snap.BuyCostCents) < float64(snap.MedianCents)*hotDealThreshold {
			ids = append(ids, snap.PurchaseID)
		}
	}
	return ids
}

// deduplicateByCardIdentity removes cards with the same (name, set, grade)
// identity from a post's card list. Also deduplicates by purchase ID.
// When duplicates exist, prefers the card with an image, then higher market value.
func deduplicateByCardIdentity(ids []string, cardLookup map[string]PostCardDetail) []string {
	type cardIdentity struct {
		name  string
		set   string
		grade float64
	}

	best := make(map[cardIdentity]string)
	bestCard := make(map[cardIdentity]PostCardDetail)

	seenPurchase := make(map[string]bool)
	for _, pid := range ids {
		if seenPurchase[pid] {
			continue
		}
		seenPurchase[pid] = true

		card, ok := cardLookup[pid]
		if !ok {
			continue
		}

		key := cardIdentity{name: card.CardName, set: card.SetName, grade: card.GradeValue}
		existing, exists := bestCard[key]
		if !exists {
			best[key] = pid
			bestCard[key] = card
			continue
		}

		if card.FrontImageURL != "" && existing.FrontImageURL == "" {
			best[key] = pid
			bestCard[key] = card
			continue
		}
		if card.FrontImageURL == "" && existing.FrontImageURL != "" {
			continue
		}
		if card.CLValueCents > existing.CLValueCents {
			best[key] = pid
			bestCard[key] = card
		}
	}

	bestSet := make(map[string]bool, len(best))
	for _, pid := range best {
		bestSet[pid] = true
	}

	seenPurchase = make(map[string]bool)
	var result []string
	for _, pid := range ids {
		if seenPurchase[pid] {
			continue
		}
		seenPurchase[pid] = true
		if bestSet[pid] {
			result = append(result, pid)
		}
	}
	return result
}
