package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- mock LLM provider ---

type mockLLMProvider struct {
	// Response to stream back as a single complete chunk.
	response string
	err      error
}

func (m *mockLLMProvider) StreamCompletion(_ context.Context, _ ai.CompletionRequest, stream func(ai.CompletionChunk)) error {
	if m.err != nil {
		return m.err
	}
	stream(ai.CompletionChunk{
		Delta: m.response,
		Usage: &ai.TokenUsage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
		Done:  true,
	})
	return nil
}

// --- mock AI call tracker ---

type mockAITracker struct {
	calls []*ai.AICallRecord
}

func (m *mockAITracker) RecordAICall(_ context.Context, call *ai.AICallRecord) error {
	m.calls = append(m.calls, call)
	return nil
}

func (m *mockAITracker) GetAIUsage(_ context.Context) (*ai.AIUsageStats, error) {
	return &ai.AIUsageStats{}, nil
}

// --- helper to build a service with LLM configured ---

func newLLMService(repo Repository, llm *mockLLMProvider, tracker *mockAITracker) *service {
	s := &service{
		repo:     repo,
		llm:      llm,
		logger:   observability.NewNoopLogger(),
		minCards: defaultMinCards,
		maxCards: defaultMaxCards,
	}
	if tracker != nil {
		s.tracker = tracker
	}
	return s
}

// makePurchaseCards returns n PostCardDetail records with unique IDs.
func makePurchaseCards(n int) []PostCardDetail {
	cards := make([]PostCardDetail, n)
	for i := range cards {
		cards[i] = PostCardDetail{
			PurchaseID: fmt.Sprintf("p%d", i),
			CardName:   "Charizard",
			SetName:    "Base Set",
			GradeValue: 9.0,
		}
	}
	return cards
}

// validSuggestionJSON builds the minimal JSON that llmGenerate expects.
func validSuggestionJSON(purchaseIDs []string) string {
	type suggestion struct {
		PostType    string   `json:"postType"`
		CoverTitle  string   `json:"coverTitle"`
		PurchaseIDs []string `json:"purchaseIds"`
	}
	type resp struct {
		Posts []suggestion `json:"posts"`
	}
	b, _ := json.Marshal(resp{Posts: []suggestion{{
		PostType:    "new_arrivals",
		CoverTitle:  "Fresh Pulls",
		PurchaseIDs: purchaseIDs,
	}}})
	return string(b)
}

// --- Tests for llmGenerate ---

func TestLLMGenerate_HappyPath(t *testing.T) {
	ctx := context.Background()
	cards := makePurchaseCards(3)
	ids := []string{cards[0].PurchaseID, cards[1].PurchaseID, cards[2].PurchaseID}
	createdPosts := 0

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return cards, nil
		},
		CreatePostFn: func(_ context.Context, _ *SocialPost) error {
			createdPosts++
			return nil
		},
		AddPostCardsFn: func(_ context.Context, _ string, _ []PostCard) error { return nil },
		ListPostCardsFn: func(_ context.Context, _ string) ([]PostCardDetail, error) {
			return cards, nil
		},
		UpdatePostCaptionFn: func(_ context.Context, _, _, _ string) error { return nil },
		UpdateCoverTitleFn:  func(_ context.Context, _ string, _ string) error { return nil },
	}

	llm := &mockLLMProvider{response: validSuggestionJSON(ids)}
	tracker := &mockAITracker{}
	svc := newLLMService(repo, llm, tracker)

	n, err := svc.llmGenerate(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("created posts: got %d, want 1", n)
	}
	if createdPosts != 1 {
		t.Errorf("repo.CreatePost calls: got %d, want 1", createdPosts)
	}
	// Tracker should have been called with social_suggestion operation.
	if len(tracker.calls) == 0 {
		t.Error("expected tracker to be called, but it was not")
	} else if tracker.calls[0].Operation != ai.OpSocialSuggestion {
		t.Errorf("tracker operation: got %q, want %q", tracker.calls[0].Operation, ai.OpSocialSuggestion)
	}
}

func TestLLMGenerate_ReturnsZeroWhenTooFewCards(t *testing.T) {
	ctx := context.Background()

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return []PostCardDetail{}, nil // 0 cards < minCards=1
		},
	}
	svc := newLLMService(repo, &mockLLMProvider{}, nil)

	n, err := svc.llmGenerate(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 posts created, got %d", n)
	}
}

func TestLLMGenerate_GetAvailableCardsError(t *testing.T) {
	ctx := context.Background()

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return nil, errors.New("db down")
		},
	}
	svc := newLLMService(repo, &mockLLMProvider{}, nil)

	_, err := svc.llmGenerate(ctx)

	if err == nil {
		t.Error("expected error from GetAvailableCardsForPosts, got nil")
	}
}

func TestLLMGenerate_LLMError(t *testing.T) {
	ctx := context.Background()
	cards := makePurchaseCards(2)

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return cards, nil
		},
	}
	llm := &mockLLMProvider{err: errors.New("LLM unavailable")}
	tracker := &mockAITracker{}
	svc := newLLMService(repo, llm, tracker)

	_, err := svc.llmGenerate(ctx)

	if err == nil {
		t.Error("expected error from LLM, got nil")
	}
	// Tracker should still be called even on error.
	if len(tracker.calls) == 0 {
		t.Error("expected tracker to be called on LLM error, but it was not")
	}
}

func TestLLMGenerate_InvalidJSONResponse(t *testing.T) {
	ctx := context.Background()
	cards := makePurchaseCards(2)

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return cards, nil
		},
	}
	llm := &mockLLMProvider{response: "not valid json at all"}
	svc := newLLMService(repo, llm, nil)

	_, err := svc.llmGenerate(ctx)

	if err == nil {
		t.Error("expected JSON parse error, got nil")
	}
}

func TestLLMGenerate_SkipsSuggestionWithInvalidPurchaseIDs(t *testing.T) {
	ctx := context.Background()
	cards := makePurchaseCards(2)

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return cards, nil
		},
	}
	// The LLM response references purchase IDs that don't exist in available cards.
	llm := &mockLLMProvider{response: validSuggestionJSON([]string{"nonexistent-id-1", "nonexistent-id-2"})}
	svc := newLLMService(repo, llm, nil)

	n, err := svc.llmGenerate(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All IDs are invalid — suggestion should be skipped.
	if n != 0 {
		t.Errorf("expected 0 posts (all IDs invalid), got %d", n)
	}
}

func TestLLMGenerate_CreatePostError_SkipsSuggestion(t *testing.T) {
	ctx := context.Background()
	cards := makePurchaseCards(3)
	ids := []string{cards[0].PurchaseID, cards[1].PurchaseID, cards[2].PurchaseID}

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return cards, nil
		},
		CreatePostFn: func(_ context.Context, _ *SocialPost) error {
			return errors.New("db error")
		},
	}
	llm := &mockLLMProvider{response: validSuggestionJSON(ids)}
	svc := newLLMService(repo, llm, nil)

	n, err := svc.llmGenerate(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// CreatePost failed → no post created.
	if n != 0 {
		t.Errorf("expected 0 posts (CreatePost error), got %d", n)
	}
}

func TestLLMGenerate_AddPostCardsError_DeletesPost(t *testing.T) {
	ctx := context.Background()
	cards := makePurchaseCards(3)
	ids := []string{cards[0].PurchaseID, cards[1].PurchaseID, cards[2].PurchaseID}
	deleted := false

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return cards, nil
		},
		CreatePostFn: func(_ context.Context, _ *SocialPost) error { return nil },
		AddPostCardsFn: func(_ context.Context, _ string, _ []PostCard) error {
			return errors.New("add cards failed")
		},
		DeletePostFn: func(_ context.Context, _ string) error {
			deleted = true
			return nil
		},
	}
	llm := &mockLLMProvider{response: validSuggestionJSON(ids)}
	svc := newLLMService(repo, llm, nil)

	n, err := svc.llmGenerate(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 posts (AddPostCards error), got %d", n)
	}
	if !deleted {
		t.Error("expected DeletePost to be called after AddPostCards failure, but it was not")
	}
}

// --- Tests for streamCaption ---

func TestStreamCaption_HappyPath(t *testing.T) {
	ctx := context.Background()
	captionJSON := `{"title":"Fire Pack","caption":"Hot new arrivals!","hashtags":"#pokemon"}`

	llm := &mockLLMProvider{response: captionJSON}
	tracker := &mockAITracker{}
	svc := &service{
		llm:    llm,
		logger: observability.NewNoopLogger(),
		repo:   &mockSocialRepo{},
	}
	svc.tracker = tracker

	result, err := svc.streamCaption(ctx, "user prompt")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result, got empty")
	}
	// Tracker should record the social_caption operation.
	if len(tracker.calls) == 0 {
		t.Error("expected tracker to be called")
	} else if tracker.calls[0].Operation != ai.OpSocialCaption {
		t.Errorf("tracker operation: got %q, want %q", tracker.calls[0].Operation, ai.OpSocialCaption)
	}
}

func TestStreamCaption_LLMError(t *testing.T) {
	ctx := context.Background()
	llm := &mockLLMProvider{err: errors.New("connection reset")}
	tracker := &mockAITracker{}
	svc := &service{
		llm:     llm,
		logger:  observability.NewNoopLogger(),
		repo:    &mockSocialRepo{},
		tracker: tracker,
	}

	_, err := svc.streamCaption(ctx, "user prompt")

	if err == nil {
		t.Error("expected error from LLM, got nil")
	}
	// Tracker must still be called even on error.
	if len(tracker.calls) == 0 {
		t.Error("expected tracker to be called even on LLM error")
	}
}

// --- Tests for RegenerateCaption ---

func TestRegenerateCaption_HappyPath(t *testing.T) {
	ctx := context.Background()
	captionJSON := `{"title":"Fire Pack","caption":"Hot new arrivals!","hashtags":"#pokemon"}`

	repo := &mockSocialRepo{
		GetPostFn: func(_ context.Context, _ string) (*SocialPost, error) {
			return &SocialPost{ID: "p1", PostType: PostTypeNewArrivals}, nil
		},
		ListPostCardsFn: func(_ context.Context, _ string) ([]PostCardDetail, error) {
			return makePurchaseCards(2), nil
		},
		UpdatePostCaptionFn: func(_ context.Context, _, _, _ string) error { return nil },
		UpdateCoverTitleFn:  func(_ context.Context, _ string, _ string) error { return nil },
	}
	llm := &mockLLMProvider{response: captionJSON}
	svc := &service{
		llm:    llm,
		logger: observability.NewNoopLogger(),
		repo:   repo,
	}

	var events []ai.StreamEvent
	err := svc.RegenerateCaption(ctx, "p1", func(e ai.StreamEvent) {
		events = append(events, e)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should emit a single EventDone.
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != ai.EventDone {
		t.Errorf("event type: got %q, want %q", events[0].Type, ai.EventDone)
	}
	// EventDone content should be JSON with caption/hashtags/title.
	var result map[string]string
	if err := json.Unmarshal([]byte(events[0].Content), &result); err != nil {
		t.Fatalf("EventDone content is not valid JSON: %v", err)
	}
	if result["caption"] == "" {
		t.Error("expected non-empty caption in EventDone content")
	}
}

func TestRegenerateCaption_NoLLM_ReturnsError(t *testing.T) {
	ctx := context.Background()
	svc := &service{
		llm:  nil,
		repo: &mockSocialRepo{},
	}

	err := svc.RegenerateCaption(ctx, "p1", func(_ ai.StreamEvent) {})

	if err == nil {
		t.Error("expected error when no LLM configured, got nil")
	}
}

func TestRegenerateCaption_PostNotFound(t *testing.T) {
	ctx := context.Background()
	repo := &mockSocialRepo{
		GetPostFn: func(_ context.Context, _ string) (*SocialPost, error) {
			return nil, nil // post not found
		},
	}
	svc := &service{
		llm:  &mockLLMProvider{},
		repo: repo,
	}

	err := svc.RegenerateCaption(ctx, "missing", func(_ ai.StreamEvent) {})

	if err == nil {
		t.Error("expected error for missing post, got nil")
	}
}

func TestRegenerateCaption_LLMError(t *testing.T) {
	ctx := context.Background()
	repo := &mockSocialRepo{
		GetPostFn: func(_ context.Context, _ string) (*SocialPost, error) {
			return &SocialPost{ID: "p1", PostType: PostTypeNewArrivals}, nil
		},
		ListPostCardsFn: func(_ context.Context, _ string) ([]PostCardDetail, error) {
			return makePurchaseCards(2), nil
		},
	}
	svc := &service{
		llm:    &mockLLMProvider{err: errors.New("LLM down")},
		logger: observability.NewNoopLogger(),
		repo:   repo,
	}

	err := svc.RegenerateCaption(ctx, "p1", func(_ ai.StreamEvent) {})

	if err == nil {
		t.Error("expected error from LLM failure, got nil")
	}
}

// --- Tests for DetectAndGenerate (top-level dispatcher) ---

func TestDetectAndGenerate_UsesLLMWhenConfigured(t *testing.T) {
	ctx := context.Background()
	cards := makePurchaseCards(3)
	ids := []string{cards[0].PurchaseID, cards[1].PurchaseID, cards[2].PurchaseID}

	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return cards, nil
		},
		CreatePostFn:        func(_ context.Context, _ *SocialPost) error { return nil },
		AddPostCardsFn:      func(_ context.Context, _ string, _ []PostCard) error { return nil },
		ListPostCardsFn:     func(_ context.Context, _ string) ([]PostCardDetail, error) { return nil, nil },
		UpdatePostCaptionFn: func(_ context.Context, _, _, _ string) error { return nil },
	}
	llm := &mockLLMProvider{response: validSuggestionJSON(ids)}
	svc := newLLMService(repo, llm, nil)

	n, err := svc.DetectAndGenerate(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 post created, got %d", n)
	}
}

func TestDetectAndGenerate_FallsBackToRuleBasedOnLLMError(t *testing.T) {
	ctx := context.Background()
	cards := makePurchaseCards(3)

	ruleBasedCalled := false
	repo := &mockSocialRepo{
		GetAvailableCardsForPostsFn: func(_ context.Context) ([]PostCardDetail, error) {
			return cards, nil
		},
		GetUnsoldPurchasesFn: func(_ context.Context) ([]PurchaseSnapshot, error) {
			ruleBasedCalled = true
			return nil, nil // rule-based returns 0 posts — no snapshots
		},
		GetRecentPurchaseIDsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
	}
	// LLM errors out → should fall back to rule-based.
	llm := &mockLLMProvider{err: errors.New("LLM unavailable")}
	svc := newLLMService(repo, llm, nil)

	_, err := svc.DetectAndGenerate(ctx)

	if err != nil {
		t.Fatalf("unexpected error (fallback should be transparent): %v", err)
	}
	if !ruleBasedCalled {
		t.Error("expected rule-based generate to be called as fallback, but it was not")
	}
}
