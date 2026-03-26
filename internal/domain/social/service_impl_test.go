package social

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseCaption(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		caption  string
		hashtags string
	}{
		{
			name:     "caption with hashtags",
			input:    "Fresh PSA 10 Charizard just landed!\n\n#pokemon #charizard #psa10",
			caption:  "Fresh PSA 10 Charizard just landed!",
			hashtags: "#pokemon #charizard #psa10",
		},
		{
			name:     "no hashtags",
			input:    "Just a caption with no tags",
			caption:  "Just a caption with no tags",
			hashtags: "",
		},
		{
			name:     "empty input",
			input:    "",
			caption:  "",
			hashtags: "",
		},
		{
			name:     "only hashtags",
			input:    "#pokemon #cards",
			caption:  "",
			hashtags: "#pokemon #cards",
		},
		{
			name:     "multiline caption with hashtags",
			input:    "Line one\nLine two\nLine three\n\n#tag1 #tag2",
			caption:  "Line one\nLine two\nLine three",
			hashtags: "#tag1 #tag2",
		},
		{
			name:     "trailing blank lines before hashtags",
			input:    "Caption text\n\n\n#hashtags",
			caption:  "Caption text",
			hashtags: "#hashtags",
		},
		{
			name:     "long caption truncated at word boundary",
			input:    strings.Repeat("word ", 80) + "\n\n#tag",
			caption:  strings.TrimSpace(strings.Repeat("word ", 60)) + "…",
			hashtags: "#tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caption, hashtags := parseCaption(tt.input)
			if caption != tt.caption {
				t.Errorf("caption = %q, want %q", caption, tt.caption)
			}
			if hashtags != tt.hashtags {
				t.Errorf("hashtags = %q, want %q", hashtags, tt.hashtags)
			}
		})
	}
}

func TestFilterPriceMovers(t *testing.T) {
	recent := time.Now().UTC().Format("2006-01-02")
	old := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02")

	tests := []struct {
		name      string
		snapshots []PurchaseSnapshot
		wantCount int
	}{
		{
			name: "above threshold included",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", MedianCents: 1000, Trend30d: 0.20, SnapshotDate: recent},
			},
			wantCount: 1,
		},
		{
			name: "below threshold excluded",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", MedianCents: 1000, Trend30d: 0.10, SnapshotDate: recent},
			},
			wantCount: 0,
		},
		{
			name: "negative trend included",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", MedianCents: 1000, Trend30d: -0.20, SnapshotDate: recent},
			},
			wantCount: 1,
		},
		{
			name: "at threshold boundary included",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", MedianCents: 1000, Trend30d: 0.15, SnapshotDate: recent},
			},
			wantCount: 1,
		},
		{
			name: "old snapshot excluded",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", MedianCents: 1000, Trend30d: 0.20, SnapshotDate: old},
			},
			wantCount: 0,
		},
		{
			name: "zero median excluded",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", MedianCents: 0, Trend30d: 0.20, SnapshotDate: recent},
			},
			wantCount: 0,
		},
		{
			name:      "empty input",
			snapshots: nil,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterPriceMovers(tt.snapshots)
			if len(result) != tt.wantCount {
				t.Errorf("got %d IDs, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestFilterHotDeals(t *testing.T) {
	recent := time.Now().UTC().Format("2006-01-02")

	tests := []struct {
		name      string
		snapshots []PurchaseSnapshot
		wantCount int
	}{
		{
			name: "hot deal — buy cost under 70% of median",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", BuyCostCents: 600, MedianCents: 1000, SnapshotDate: recent},
			},
			wantCount: 1,
		},
		{
			name: "not a deal — buy cost at 70% boundary",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", BuyCostCents: 700, MedianCents: 1000, SnapshotDate: recent},
			},
			wantCount: 0,
		},
		{
			name: "not a deal — buy cost above 70%",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", BuyCostCents: 800, MedianCents: 1000, SnapshotDate: recent},
			},
			wantCount: 0,
		},
		{
			name: "zero buy cost excluded",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", BuyCostCents: 0, MedianCents: 1000, SnapshotDate: recent},
			},
			wantCount: 0,
		},
		{
			name: "zero median excluded",
			snapshots: []PurchaseSnapshot{
				{PurchaseID: "p1", BuyCostCents: 500, MedianCents: 0, SnapshotDate: recent},
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterHotDeals(tt.snapshots)
			if len(result) != tt.wantCount {
				t.Errorf("got %d IDs, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestParseCaptionResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		title    string
		caption  string
		hashtags string
	}{
		{
			name:     "valid JSON response",
			input:    `{"title":"Moonbreon & Friends","caption":"This PSA 10 Umbreon is stunning.","hashtags":"#CardYeti #PSAgraded #Moonbreon"}`,
			title:    "Moonbreon & Friends",
			caption:  "This PSA 10 Umbreon is stunning.",
			hashtags: "#CardYeti #PSAgraded #Moonbreon",
		},
		{
			name:     "JSON with markdown fences",
			input:    "```json\n{\"title\":\"Hot Deals\",\"caption\":\"Great value.\",\"hashtags\":\"#CardYeti\"}\n```",
			title:    "Hot Deals",
			caption:  "Great value.",
			hashtags: "#CardYeti",
		},
		{
			name:     "fallback to text parsing when not JSON",
			input:    "Fresh PSA 10 Charizard just landed!\n\n#pokemon #charizard #psa10",
			title:    "",
			caption:  "Fresh PSA 10 Charizard just landed!",
			hashtags: "#pokemon #charizard #psa10",
		},
		{
			name:     "JSON missing title field",
			input:    `{"caption":"Just a caption.","hashtags":"#tags"}`,
			title:    "",
			caption:  "Just a caption.",
			hashtags: "#tags",
		},
		{
			name:     "empty input",
			input:    "",
			title:    "",
			caption:  "",
			hashtags: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, caption, hashtags := parseCaptionResponse(tt.input)
			if title != tt.title {
				t.Errorf("title = %q, want %q", title, tt.title)
			}
			if caption != tt.caption {
				t.Errorf("caption = %q, want %q", caption, tt.caption)
			}
			if hashtags != tt.hashtags {
				t.Errorf("hashtags = %q, want %q", hashtags, tt.hashtags)
			}
		})
	}
}

// --- Mock types for service-level tests ---

type mockSocialRepo struct {
	getPostFunc                  func(ctx context.Context, id string) (*SocialPost, error)
	createPostFunc               func(ctx context.Context, post *SocialPost) error
	updatePostStatusFunc         func(ctx context.Context, id string, status PostStatus) error
	deletePostFunc               func(ctx context.Context, id string) error
	listPostCardsFunc            func(ctx context.Context, postID string) ([]PostCardDetail, error)
	getRecentPurchaseIDsFunc     func(ctx context.Context, since string) ([]string, error)
	getPurchaseIDsInExistingFunc func(ctx context.Context, ids []string, pt PostType) (map[string]bool, error)
	getUnsoldPurchasesFunc       func(ctx context.Context) ([]PurchaseSnapshot, error)
	updatePostCaptionFunc        func(ctx context.Context, id, caption, hashtags string) error
	addPostCardsFunc             func(ctx context.Context, postID string, cards []PostCard) error
	listPostsFunc                func(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error)
	setPublishedFunc             func(ctx context.Context, id, igPostID string) error
	setPublishingFunc            func(ctx context.Context, id string) error
	updateSlideURLsFunc          func(ctx context.Context, id string, urls []string) error
	updateCoverTitleFunc         func(ctx context.Context, id string, title string) error
	updateBackgroundURLsFunc     func(ctx context.Context, id string, urls []string) error
}

func (m *mockSocialRepo) GetPost(ctx context.Context, id string) (*SocialPost, error) {
	if m.getPostFunc != nil {
		return m.getPostFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockSocialRepo) CreatePost(ctx context.Context, post *SocialPost) error {
	if m.createPostFunc != nil {
		return m.createPostFunc(ctx, post)
	}
	return nil
}

func (m *mockSocialRepo) UpdatePostStatus(ctx context.Context, id string, status PostStatus) error {
	if m.updatePostStatusFunc != nil {
		return m.updatePostStatusFunc(ctx, id, status)
	}
	return nil
}

func (m *mockSocialRepo) DeletePost(ctx context.Context, id string) error {
	if m.deletePostFunc != nil {
		return m.deletePostFunc(ctx, id)
	}
	return nil
}

func (m *mockSocialRepo) ListPostCards(ctx context.Context, postID string) ([]PostCardDetail, error) {
	if m.listPostCardsFunc != nil {
		return m.listPostCardsFunc(ctx, postID)
	}
	return nil, nil
}

func (m *mockSocialRepo) GetRecentPurchaseIDs(ctx context.Context, since string) ([]string, error) {
	if m.getRecentPurchaseIDsFunc != nil {
		return m.getRecentPurchaseIDsFunc(ctx, since)
	}
	return nil, nil
}

func (m *mockSocialRepo) GetPurchaseIDsInExistingPosts(ctx context.Context, ids []string, pt PostType) (map[string]bool, error) {
	if m.getPurchaseIDsInExistingFunc != nil {
		return m.getPurchaseIDsInExistingFunc(ctx, ids, pt)
	}
	return nil, nil
}

func (m *mockSocialRepo) GetUnsoldPurchasesWithSnapshots(ctx context.Context) ([]PurchaseSnapshot, error) {
	if m.getUnsoldPurchasesFunc != nil {
		return m.getUnsoldPurchasesFunc(ctx)
	}
	return nil, nil
}

func (m *mockSocialRepo) UpdatePostCaption(ctx context.Context, id, caption, hashtags string) error {
	if m.updatePostCaptionFunc != nil {
		return m.updatePostCaptionFunc(ctx, id, caption, hashtags)
	}
	return nil
}

func (m *mockSocialRepo) AddPostCards(ctx context.Context, postID string, cards []PostCard) error {
	if m.addPostCardsFunc != nil {
		return m.addPostCardsFunc(ctx, postID, cards)
	}
	return nil
}

func (m *mockSocialRepo) ListPosts(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error) {
	if m.listPostsFunc != nil {
		return m.listPostsFunc(ctx, status, limit, offset)
	}
	return nil, nil
}

func (m *mockSocialRepo) SetPublished(ctx context.Context, id, igPostID string) error {
	if m.setPublishedFunc != nil {
		return m.setPublishedFunc(ctx, id, igPostID)
	}
	return nil
}

func (m *mockSocialRepo) SetPublishing(ctx context.Context, id string) error {
	if m.setPublishingFunc != nil {
		return m.setPublishingFunc(ctx, id)
	}
	return nil
}

func (m *mockSocialRepo) SetError(_ context.Context, _, _ string) error { return nil }

func (m *mockSocialRepo) GetAvailableCardsForPosts(_ context.Context) ([]PostCardDetail, error) {
	return nil, nil
}

func (m *mockSocialRepo) UpdateSlideURLs(ctx context.Context, id string, urls []string) error {
	if m.updateSlideURLsFunc != nil {
		return m.updateSlideURLsFunc(ctx, id, urls)
	}
	return nil
}

func (m *mockSocialRepo) UpdateCoverTitle(ctx context.Context, id string, title string) error {
	if m.updateCoverTitleFunc != nil {
		return m.updateCoverTitleFunc(ctx, id, title)
	}
	return nil
}

func (m *mockSocialRepo) UpdateBackgroundURLs(ctx context.Context, id string, urls []string) error {
	if m.updateBackgroundURLsFunc != nil {
		return m.updateBackgroundURLsFunc(ctx, id, urls)
	}
	return nil
}

type mockPublisher struct{}

func (m *mockPublisher) PublishCarousel(_ context.Context, _, _ string, _ []string, _ string) (*PublishResultInfo, error) {
	return &PublishResultInfo{InstagramPostID: "ig_123"}, nil
}

type mockTokenProvider struct{}

func (m *mockTokenProvider) GetToken(_ context.Context) (string, string, error) {
	return "token", "user123", nil
}

// --- Service-level tests ---

func TestPublish_AlreadyPublished(t *testing.T) {
	repo := &mockSocialRepo{}
	repo.getPostFunc = func(_ context.Context, _ string) (*SocialPost, error) {
		return &SocialPost{ID: "p1", Caption: "A real caption"}, nil
	}
	// SetPublishing returns error for already-published posts (mock default returns nil,
	// so override to simulate the repo rejecting it)
	repo.setPublishingFunc = func(_ context.Context, _ string) error {
		return fmt.Errorf("post not in publishable state")
	}
	svc := NewService(repo, WithPublisher(&mockPublisher{}, &mockTokenProvider{}))
	err := svc.Publish(context.Background(), "p1")
	if err == nil {
		t.Error("expected error for non-publishable post")
	}
}

func TestPublish_DraftSucceeds(t *testing.T) {
	published := false
	repo := &mockSocialRepo{}
	repo.getPostFunc = func(_ context.Context, _ string) (*SocialPost, error) {
		return &SocialPost{ID: "p1", Caption: "A real caption"}, nil
	}
	repo.setPublishingFunc = func(_ context.Context, _ string) error {
		published = true
		return nil
	}
	svc := NewService(repo, WithPublisher(&mockPublisher{}, &mockTokenProvider{}))
	err := svc.Publish(context.Background(), "p1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	svc.Wait() // wait for background publishAsync goroutine
	if !published {
		t.Error("expected SetPublishing to be called")
	}
}

func TestPublish_PlaceholderCaptionBlocked(t *testing.T) {
	repo := &mockSocialRepo{}
	repo.getPostFunc = func(_ context.Context, _ string) (*SocialPost, error) {
		return &SocialPost{ID: "p1", Caption: placeholderCaption}, nil
	}
	setPublishingCalled := false
	repo.setPublishingFunc = func(_ context.Context, _ string) error {
		setPublishingCalled = true
		return nil
	}
	svc := NewService(repo, WithPublisher(&mockPublisher{}, &mockTokenProvider{}))
	err := svc.Publish(context.Background(), "p1")
	if err == nil {
		t.Error("expected error when caption is placeholder")
	}
	if setPublishingCalled {
		t.Error("SetPublishing must not be called when caption is a placeholder")
	}
}

func TestPublish_EmptyCaptionBlocked(t *testing.T) {
	repo := &mockSocialRepo{}
	repo.getPostFunc = func(_ context.Context, _ string) (*SocialPost, error) {
		return &SocialPost{ID: "p1", Caption: ""}, nil
	}
	setPublishingCalled := false
	repo.setPublishingFunc = func(_ context.Context, _ string) error {
		setPublishingCalled = true
		return nil
	}
	svc := NewService(repo, WithPublisher(&mockPublisher{}, &mockTokenProvider{}))
	err := svc.Publish(context.Background(), "p1")
	if err == nil {
		t.Error("expected error when caption is empty")
	}
	if setPublishingCalled {
		t.Error("SetPublishing must not be called when caption is empty")
	}
}

func TestPublish_NoPublisherConfigured(t *testing.T) {
	repo := &mockSocialRepo{}
	svc := NewService(repo)
	err := svc.Publish(context.Background(), "p1")
	if err == nil {
		t.Error("expected error when publisher not configured")
	}
}

func TestDeduplicateByCardIdentity(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		cards   []PostCardDetail
		wantIDs []string
	}{
		{
			name: "no duplicates — all kept",
			ids:  []string{"p1", "p2"},
			cards: []PostCardDetail{
				{PurchaseID: "p1", CardName: "Charizard", SetName: "Base Set", GradeValue: 10},
				{PurchaseID: "p2", CardName: "Pikachu", SetName: "Jungle", GradeValue: 9},
			},
			wantIDs: []string{"p1", "p2"},
		},
		{
			name: "duplicate identity — second removed",
			ids:  []string{"p1", "p2", "p3"},
			cards: []PostCardDetail{
				{PurchaseID: "p1", CardName: "Charizard", SetName: "Base Set", GradeValue: 10},
				{PurchaseID: "p2", CardName: "Charizard", SetName: "Base Set", GradeValue: 10},
				{PurchaseID: "p3", CardName: "Pikachu", SetName: "Jungle", GradeValue: 9},
			},
			wantIDs: []string{"p1", "p3"},
		},
		{
			name: "same name different grade — both kept",
			ids:  []string{"p1", "p2"},
			cards: []PostCardDetail{
				{PurchaseID: "p1", CardName: "Charizard", SetName: "Base Set", GradeValue: 10},
				{PurchaseID: "p2", CardName: "Charizard", SetName: "Base Set", GradeValue: 9},
			},
			wantIDs: []string{"p1", "p2"},
		},
		{
			name: "same name different set — both kept",
			ids:  []string{"p1", "p2"},
			cards: []PostCardDetail{
				{PurchaseID: "p1", CardName: "Charizard", SetName: "Base Set", GradeValue: 10},
				{PurchaseID: "p2", CardName: "Charizard", SetName: "Evolutions", GradeValue: 10},
			},
			wantIDs: []string{"p1", "p2"},
		},
		{
			name: "duplicate purchase ID — second removed",
			ids:  []string{"p1", "p1", "p2"},
			cards: []PostCardDetail{
				{PurchaseID: "p1", CardName: "Charizard", SetName: "Base Set", GradeValue: 10},
				{PurchaseID: "p2", CardName: "Pikachu", SetName: "Jungle", GradeValue: 9},
			},
			wantIDs: []string{"p1", "p2"},
		},
		{
			name:    "empty input",
			ids:     nil,
			cards:   nil,
			wantIDs: nil,
		},
		{
			name: "tiebreaker — prefer card with image",
			ids:  []string{"p1", "p2"},
			cards: []PostCardDetail{
				{PurchaseID: "p1", CardName: "Charizard", SetName: "Base Set", GradeValue: 10, FrontImageURL: ""},
				{PurchaseID: "p2", CardName: "Charizard", SetName: "Base Set", GradeValue: 10, FrontImageURL: "http://img.jpg"},
			},
			wantIDs: []string{"p2"},
		},
		{
			name: "tiebreaker — prefer higher CL value",
			ids:  []string{"p1", "p2"},
			cards: []PostCardDetail{
				{PurchaseID: "p1", CardName: "Charizard", SetName: "Base Set", GradeValue: 10, FrontImageURL: "http://a.jpg", CLValueCents: 3000},
				{PurchaseID: "p2", CardName: "Charizard", SetName: "Base Set", GradeValue: 10, FrontImageURL: "http://b.jpg", CLValueCents: 5000},
			},
			wantIDs: []string{"p2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cardLookup := make(map[string]PostCardDetail, len(tt.cards))
			for _, c := range tt.cards {
				cardLookup[c.PurchaseID] = c
			}
			got := deduplicateByCardIdentity(tt.ids, cardLookup)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got %d IDs %v, want %d IDs %v", len(got), got, len(tt.wantIDs), tt.wantIDs)
			}
			for i, id := range got {
				if id != tt.wantIDs[i] {
					t.Errorf("got[%d] = %q, want %q", i, id, tt.wantIDs[i])
				}
			}
		})
	}
}
