package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// mockDHInstagramClient implements DHInstagramClient for testing.
type mockDHInstagramClient struct {
	EnterpriseAvailableFn     func() bool
	GenerateInstagramPostFn   func(ctx context.Context, scope, strategy, headline string) (int64, error)
	PollInstagramPostStatusFn func(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error)
}

func (m *mockDHInstagramClient) EnterpriseAvailable() bool {
	if m.EnterpriseAvailableFn != nil {
		return m.EnterpriseAvailableFn()
	}
	return true
}

func (m *mockDHInstagramClient) GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error) {
	return m.GenerateInstagramPostFn(ctx, scope, strategy, headline)
}

func (m *mockDHInstagramClient) PollInstagramPostStatus(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
	return m.PollInstagramPostStatusFn(ctx, postID)
}

// mockDHSocialRepo implements DHSocialRepo for testing.
type mockDHSocialRepo struct {
	CreatePostFn        func(ctx context.Context, post *social.SocialPost) error
	UpdatePostCaptionFn func(ctx context.Context, id string, caption, hashtags string) error
	UpdateSlideURLsFn   func(ctx context.Context, id string, urls []string) error
}

func (m *mockDHSocialRepo) CreatePost(ctx context.Context, post *social.SocialPost) error {
	if m.CreatePostFn != nil {
		return m.CreatePostFn(ctx, post)
	}
	return nil
}

func (m *mockDHSocialRepo) UpdatePostCaption(ctx context.Context, id string, caption, hashtags string) error {
	if m.UpdatePostCaptionFn != nil {
		return m.UpdatePostCaptionFn(ctx, id, caption, hashtags)
	}
	return nil
}

func (m *mockDHSocialRepo) UpdateSlideURLs(ctx context.Context, id string, urls []string) error {
	if m.UpdateSlideURLsFn != nil {
		return m.UpdateSlideURLsFn(ctx, id, urls)
	}
	return nil
}

func newTestDHSocialScheduler(dhClient DHInstagramClient, repo DHSocialRepo) *DHSocialScheduler {
	logger := observability.NewNoopLogger()
	cfg := DHSocialSchedulerConfig{
		Hour:         6,
		PollInterval: 1 * time.Millisecond,
		PollTimeout:  100 * time.Millisecond,
	}
	return NewDHSocialScheduler(dhClient, repo, logger, cfg)
}

func TestDHSocialScheduler_Tick(t *testing.T) {
	readySlides := []string{"https://cdn.example.com/slide1.jpg", "https://cdn.example.com/slide2.jpg"}

	tests := []struct {
		name             string
		enterpriseAvail  bool
		generateFn       func(ctx context.Context, scope, strategy, headline string) (int64, error)
		pollFn           func(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error)
		wantPostsCreated int
	}{
		{
			name:             "no enterprise key — skips all",
			enterpriseAvail:  false,
			wantPostsCreated: 0,
		},
		{
			name:            "happy path — all 4 strategies succeed",
			enterpriseAvail: true,
			generateFn: func(_ context.Context, scope, strategy, headline string) (int64, error) {
				return 42, nil
			},
			pollFn: func(_ context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
				return &dh.DHInstagramStatusResponse{
					RenderStatus:   "ready",
					SlideImageURLs: readySlides,
				}, nil
			},
			wantPostsCreated: 4,
		},
		{
			name:            "one strategy fails generate — others still run",
			enterpriseAvail: true,
			generateFn: func(_ context.Context, scope, strategy, headline string) (int64, error) {
				if strategy == "inventory_top_expensive" {
					return 0, errors.New("DH error")
				}
				return 42, nil
			},
			pollFn: func(_ context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
				return &dh.DHInstagramStatusResponse{
					RenderStatus:   "ready",
					SlideImageURLs: readySlides,
				}, nil
			},
			wantPostsCreated: 3,
		},
		{
			name:            "render failed — skips strategy",
			enterpriseAvail: true,
			generateFn: func(_ context.Context, scope, strategy, headline string) (int64, error) {
				return 42, nil
			},
			pollFn: func(_ context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
				return &dh.DHInstagramStatusResponse{RenderStatus: "failed"}, nil
			},
			wantPostsCreated: 0,
		},
		{
			name:            "poll timeout — skips strategy",
			enterpriseAvail: true,
			generateFn: func(_ context.Context, scope, strategy, headline string) (int64, error) {
				return 42, nil
			},
			pollFn: func(_ context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
				// Always "generating" → will timeout
				return &dh.DHInstagramStatusResponse{RenderStatus: "generating"}, nil
			},
			wantPostsCreated: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			postsCreated := 0

			repo := &mockDHSocialRepo{
				CreatePostFn: func(ctx context.Context, post *social.SocialPost) error {
					require.Equal(t, social.PostTypeDHInstagram, post.PostType)
					require.Equal(t, social.PostStatusDraft, post.Status)
					require.NotEmpty(t, post.SlideURLs)
					require.NotEmpty(t, post.CoverTitle)
					postsCreated++
					return nil
				},
			}

			dhClient := &mockDHInstagramClient{
				EnterpriseAvailableFn: func() bool { return tc.enterpriseAvail },
			}
			if tc.generateFn != nil {
				dhClient.GenerateInstagramPostFn = tc.generateFn
			}
			if tc.pollFn != nil {
				dhClient.PollInstagramPostStatusFn = tc.pollFn
			}

			s := newTestDHSocialScheduler(dhClient, repo)
			s.tick(context.Background())

			require.Equal(t, tc.wantPostsCreated, postsCreated)
		})
	}
}

func TestBuildDHCaption(t *testing.T) {
	for _, strategy := range dhInstagramStrategies {
		caption := buildDHCaption(strategy)
		require.NotEmpty(t, caption, "caption for strategy %s should not be empty", strategy.key)
		require.Contains(t, caption, strategy.title, "caption should include strategy title")
	}
}
