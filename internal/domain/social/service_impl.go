package social

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	defaultMinCards = 1
	defaultMaxCards = 9

	// Detection thresholds
	priceChangeThreshold = 0.15 // 15% price change for price movers
	hotDealThreshold     = 0.70 // buy cost < 70% of median = hot deal
	maxSnapshotAgeDays   = 7    // skip cards with snapshots older than this
	newArrivalsWindow    = 7    // days to look back for new arrivals
)

// placeholderCaption is set when AI caption generation fails.
// Posts with this caption must not be published.
const placeholderCaption = "(Caption generation failed — click Regenerate to try again)"

type service struct {
	repo          Repository
	llm           ai.LLMProvider
	publisher     Publisher
	tokenProvider InstagramTokenProvider
	logger        observability.Logger
	tracker       ai.AICallTracker
	minCards      int
	maxCards      int
	wg            sync.WaitGroup
}

// NewService creates a new social content service.
func NewService(repo Repository, opts ...ServiceOption) Service {
	s := &service{
		repo:     repo,
		minCards: defaultMinCards,
		maxCards: defaultMaxCards,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

var _ Service = (*service)(nil)

func (s *service) DetectAndGenerate(ctx context.Context) (int, error) {
	// Try LLM-powered generation first if available
	if s.llm != nil {
		created, err := s.llmGenerate(ctx)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "social: LLM generation failed, falling back to rule-based detection — check AI health",
					observability.Err(err))
			}
		} else if created > 0 {
			return created, nil
		}
	}

	// Fallback: rule-based detection
	return s.ruleBasedGenerate(ctx)
}

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

	created := 0
	usedIDs := make(map[string]bool) // prevent cards from appearing in multiple posts

	for _, suggestion := range resp.Posts {
		// Validate and filter purchase IDs (dedup within this suggestion too)
		seen := make(map[string]bool)
		var validIDs []string
		for _, pid := range suggestion.PurchaseIDs {
			if cardMap[pid] && !usedIDs[pid] && !seen[pid] {
				seen[pid] = true
				validIDs = append(validIDs, pid)
			}
		}

		if len(validIDs) < s.minCards {
			continue
		}
		if len(validIDs) > s.maxCards {
			validIDs = validIDs[:s.maxCards]
		}

		// Mark these IDs as used
		for _, pid := range validIDs {
			usedIDs[pid] = true
		}

		// Create the post
		postID := generateID()
		pt := parsePostType(suggestion.PostType)
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
		return nil, nil
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

	if len(filtered) < s.minCards {
		return nil, nil
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
		if snap.SnapshotDate < cutoff || snap.MedianCents == 0 {
			continue
		}
		if math.Abs(snap.Trend30d) >= priceChangeThreshold {
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

// generateCaptionAsync runs caption generation in a background goroutine with its own context.
func (s *service) generateCaptionAsync(post *SocialPost) {
	if s.llm == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cards, err := s.repo.ListPostCards(ctx, post.ID)
	if err != nil {
		s.logError(ctx, "list cards for caption", post.PostType, err)
		return
	}

	userPrompt := buildUserPrompt(post.PostType, cards)
	var result strings.Builder
	var usage ai.TokenUsage
	start := time.Now()

	err = s.llm.StreamCompletion(ctx, ai.CompletionRequest{
		SystemPrompt: captionSystemPrompt,
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: userPrompt},
		},
		MaxTokens: 512,
	}, func(chunk ai.CompletionChunk) {
		result.WriteString(chunk.Delta)
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
	})
	if err != nil {
		ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialCaption, err, start, 0, &usage)
		s.logError(ctx, "generate caption", post.PostType, err)
		errCtx, errCancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.repo.UpdatePostCaption(errCtx, post.ID, placeholderCaption, "") //nolint:errcheck // best-effort
		errCancel()
		return
	}

	ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialCaption, nil, start, 0, &usage)

	// Use a fresh context for DB writes so they succeed even if the LLM
	// context is near its 3-minute deadline.
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dbCancel()

	title, caption, hashtags := parseCaptionResponse(result.String())
	if err := s.repo.UpdatePostCaption(dbCtx, post.ID, caption, hashtags); err != nil {
		if s.logger != nil {
			s.logger.Error(dbCtx, "social: failed to save generated caption",
				observability.String("postId", post.ID),
				observability.Int("captionLen", len(caption)),
				observability.Err(err))
		}
	}

	// Update cover title if the LLM provided one
	if title != "" {
		if err := s.repo.UpdateCoverTitle(dbCtx, post.ID, title); err != nil {
			if s.logger != nil {
				s.logger.Error(dbCtx, "social: failed to save cover title",
					observability.String("postId", post.ID),
					observability.Err(err))
			}
		}
	}

	if s.logger != nil {
		s.logger.Info(ctx, "social caption generated",
			observability.String("postType", string(post.PostType)),
			observability.Int("captionLen", len(caption)))
	}
}

func (s *service) Publish(ctx context.Context, id string) error {
	if s.publisher == nil || s.tokenProvider == nil {
		return ErrNotConfigured
	}

	// Check for placeholder caption before publishing.
	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}
	if post == nil {
		return ErrPostNotFound
	}
	if post.Caption == placeholderCaption || post.Caption == "" {
		return ErrNotPublishable
	}

	// Atomically set to "publishing" — prevents double-publish.
	// Only draft and failed posts can be published.
	if err := s.repo.SetPublishing(ctx, id); err != nil {
		return err
	}

	// Run the actual Instagram publish in a background goroutine.
	s.safeGo("publishAsync", id, func() { s.publishAsync(id) })

	return nil
}

func (s *service) publishAsync(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Fetch post first (need SlideURLs)
	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		s.setPublishError(ctx, id, fmt.Sprintf("get post: %v", err))
		return
	}
	if post == nil {
		s.setPublishError(ctx, id, "post not found")
		return
	}

	cards, err := s.repo.ListPostCards(ctx, id)
	if err != nil {
		s.setPublishError(ctx, id, fmt.Sprintf("list post cards: %v", err))
		return
	}

	// Prefer rendered slide URLs; fall back to raw card images for legacy posts
	var imageURLs []string
	if len(post.SlideURLs) > 0 {
		imageURLs = post.SlideURLs
	} else {
		for _, c := range cards {
			if c.FrontImageURL != "" {
				imageURLs = append(imageURLs, c.FrontImageURL)
			}
		}
	}
	if len(imageURLs) == 0 {
		s.setPublishError(ctx, id, "no card images available for publishing")
		return
	}

	caption := post.Caption
	if post.Hashtags != "" {
		caption += "\n\n" + post.Hashtags
	}

	token, igUserID, err := s.tokenProvider.GetToken(ctx)
	if err != nil {
		s.setPublishError(ctx, id, fmt.Sprintf("get Instagram token: %v", err))
		return
	}

	result, err := s.publisher.PublishCarousel(ctx, token, igUserID, imageURLs, caption)

	// Use a fresh context for terminal DB writes so the publish timeout
	// doesn't prevent persisting the final state.
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dbCancel()

	if err != nil {
		s.setPublishError(dbCtx, id, fmt.Sprintf("publish to Instagram: %v", err))
		return
	}

	if err := s.repo.SetPublished(dbCtx, id, result.InstagramPostID); err != nil {
		// The Instagram post was published successfully but we failed to record it.
		// Use setPublishError so the user sees the situation and avoids re-publishing.
		s.setPublishError(dbCtx, id, fmt.Sprintf(
			"published to Instagram (post %s) but failed to update DB: %v — do NOT re-publish",
			result.InstagramPostID, err))
		return
	}

	if s.logger != nil {
		s.logger.Info(dbCtx, "social post published to Instagram",
			observability.String("postId", id),
			observability.String("instagramPostId", result.InstagramPostID))
	}
}

func (s *service) setPublishError(ctx context.Context, id, errMsg string) {
	if s.logger != nil {
		s.logger.Error(ctx, "social publish failed",
			observability.String("postId", id),
			observability.String("error", errMsg))
	}
	if dbErr := s.repo.SetError(ctx, id, errMsg); dbErr != nil && s.logger != nil {
		s.logger.Error(ctx, "social: failed to persist publish error — post stuck in publishing state",
			observability.String("postId", id),
			observability.Err(dbErr))
	}
}

func (s *service) RegenerateCaption(ctx context.Context, id string, stream func(ai.StreamEvent)) error {
	if s.llm == nil {
		return fmt.Errorf("AI caption generation not configured")
	}

	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}
	if post == nil {
		return fmt.Errorf("post not found")
	}

	cards, err := s.repo.ListPostCards(ctx, id)
	if err != nil {
		return fmt.Errorf("list post cards: %w", err)
	}

	userPrompt := buildUserPrompt(post.PostType, cards)
	var result strings.Builder
	var usage ai.TokenUsage
	start := time.Now()

	err = s.llm.StreamCompletion(ctx, ai.CompletionRequest{
		SystemPrompt: captionSystemPrompt,
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: userPrompt},
		},
		MaxTokens: 512,
	}, func(chunk ai.CompletionChunk) {
		result.WriteString(chunk.Delta)
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
	})
	if err != nil {
		ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialCaption, err, start, 0, &usage)
		return fmt.Errorf("stream caption: %w", err)
	}

	ai.RecordCall(ctx, s.tracker, s.logger, ai.OpSocialCaption, nil, start, 0, &usage)

	title, caption, hashtags := parseCaptionResponse(result.String())
	if updateErr := s.repo.UpdatePostCaption(ctx, id, caption, hashtags); updateErr != nil {
		return fmt.Errorf("save caption: %w", updateErr)
	}
	if title != "" {
		if err := s.repo.UpdateCoverTitle(ctx, id, title); err != nil {
			return fmt.Errorf("save cover title: %w", err)
		}
	}

	// Send the parsed result as a single "done" event with all fields in JSON content
	resultJSON, err := json.Marshal(map[string]string{
		"caption":  caption,
		"hashtags": hashtags,
		"title":    title,
	})
	if err != nil {
		return fmt.Errorf("marshal caption result: %w", err)
	}
	stream(ai.StreamEvent{Type: ai.EventDone, Content: string(resultJSON)})
	return nil
}

func (s *service) ListPosts(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error) {
	return s.repo.ListPosts(ctx, status, limit, offset)
}

func (s *service) GetPost(ctx context.Context, id string) (*PostDetail, error) {
	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, nil
	}

	cards, err := s.repo.ListPostCards(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list post cards: %w", err)
	}

	return &PostDetail{
		SocialPost: *post,
		Cards:      cards,
	}, nil
}

func (s *service) UpdateCaption(ctx context.Context, id string, caption, hashtags string) error {
	return s.repo.UpdatePostCaption(ctx, id, caption, hashtags)
}

func (s *service) Delete(ctx context.Context, id string) error {
	return s.repo.DeletePost(ctx, id)
}

func (s *service) logError(ctx context.Context, msg string, pt PostType, err error) {
	if s.logger != nil {
		s.logger.Error(ctx, "social: "+msg,
			observability.String("postType", string(pt)),
			observability.Err(err))
	}
}

// safeGo runs fn in a background goroutine with panic recovery.
func (s *service) safeGo(name, postID string, fn func()) {
	s.wg.Go(func() {
		defer func() {
			if r := recover(); r != nil && s.logger != nil {
				s.logger.Error(context.Background(), "social: "+name+" panicked",
					observability.String("postId", postID),
					observability.String("panic", fmt.Sprintf("%v", r)))
			}
		}()
		fn()
	})
}

func (s *service) Wait() { s.wg.Wait() }

// parseCaption splits AI output into caption text and hashtags line.
func parseCaption(raw string) (caption, hashtags string) {
	raw = strings.TrimSpace(raw)
	lines := strings.Split(raw, "\n")

	hashtagLine := -1
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" && strings.HasPrefix(trimmed, "#") {
			hashtagLine = i
			break
		}
		if trimmed != "" {
			break
		}
	}

	if hashtagLine >= 0 {
		caption = strings.TrimSpace(strings.Join(lines[:hashtagLine], "\n"))
		hashtags = strings.TrimSpace(lines[hashtagLine])
	} else {
		caption = raw
	}

	caption = truncateCaption(caption)
	return caption, hashtags
}

// truncateCaption limits caption length for Instagram (≤300 runes).
// Truncates at the last word boundary if needed. Uses rune-based
// indexing to avoid splitting multi-byte characters (emoji).
func truncateCaption(caption string) string {
	const maxCaptionLen = 300
	runes := []rune(caption)
	if len(runes) <= maxCaptionLen {
		return caption
	}
	truncated := string(runes[:maxCaptionLen])
	if idx := strings.LastIndex(truncated, " "); idx > len(string(runes[:maxCaptionLen/2])) {
		truncated = truncated[:idx]
	}
	return strings.TrimSpace(truncated) + "…"
}

// captionResponse is the expected JSON structure from the LLM.
type captionResponse struct {
	Title    string `json:"title"`
	Caption  string `json:"caption"`
	Hashtags string `json:"hashtags"`
}

// parseCaptionResponse extracts title, caption, and hashtags from LLM output.
// Tries JSON parse first; falls back to text splitting (no title) if JSON fails.
func parseCaptionResponse(raw string) (title, caption, hashtags string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}

	cleaned := sanitizeLLMJSON(stripMarkdownFences(raw))

	var resp captionResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err == nil && resp.Caption != "" {
		return strings.TrimSpace(resp.Title), truncateCaption(strings.TrimSpace(resp.Caption)), strings.TrimSpace(resp.Hashtags)
	}

	// Fallback: text-based parsing (no title) — use fence-stripped string
	caption, hashtags = parseCaption(cleaned)
	return "", caption, hashtags
}

// stripMarkdownFences removes ```json / ``` wrappers from LLM output.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// sanitizeLLMJSON fixes common JSON issues from LLM output, such as
// literal newlines and tabs inside string values that break json.Unmarshal.
func sanitizeLLMJSON(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			buf.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			buf.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			buf.WriteByte(ch)
			continue
		}
		if inString {
			switch ch {
			case '\n':
				buf.WriteString(`\n`)
			case '\r':
				buf.WriteString(`\r`)
			case '\t':
				buf.WriteString(`\t`)
			default:
				buf.WriteByte(ch)
			}
		} else {
			buf.WriteByte(ch)
		}
	}
	return buf.String()
}

// generateID creates a unique ID using UUID v4.
func generateID() string {
	return uuid.NewString()
}
