package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newTestDHSocialScheduler(dhClient DHInstagramClient, repo DHSocialRepo) *DHSocialScheduler {
	cfg := DHSocialSchedulerConfig{
		Hour:         6,
		PollInterval: 1 * time.Millisecond,
		PollTimeout:  100 * time.Millisecond,
	}
	return NewDHSocialScheduler(dhClient, repo, nopLogger{}, cfg)
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

			repo := &mocks.MockDHSocialRepo{
				CreatePostFn: func(ctx context.Context, post *social.SocialPost) error {
					require.NotEmpty(t, post.ID, "post.ID must be set before CreatePost")
					require.False(t, post.CreatedAt.IsZero(), "post.CreatedAt must be set")
					require.False(t, post.UpdatedAt.IsZero(), "post.UpdatedAt must be set")
					require.Equal(t, social.PostTypeDHInstagram, post.PostType)
					require.Equal(t, social.PostStatusDraft, post.Status)
					require.NotEmpty(t, post.CoverTitle)
					require.NotEmpty(t, post.Caption)
					postsCreated++
					return nil
				},
				UpdateSlideURLsFn: func(ctx context.Context, id string, urls []string) error {
					require.NotEmpty(t, id)
					require.NotEmpty(t, urls)
					return nil
				},
			}

			dhClient := &mocks.MockDHInstagramClient{
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
