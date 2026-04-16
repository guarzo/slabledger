//go:build ignore

package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// MockDHInstagramClient is a test double for scheduler.DHInstagramClient.
type MockDHInstagramClient struct {
	EnterpriseAvailableFn     func() bool
	GenerateInstagramPostFn   func(ctx context.Context, scope, strategy, headline string) (int64, error)
	PollInstagramPostStatusFn func(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error)
}

func (m *MockDHInstagramClient) EnterpriseAvailable() bool {
	if m.EnterpriseAvailableFn != nil {
		return m.EnterpriseAvailableFn()
	}
	return true
}

func (m *MockDHInstagramClient) GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error) {
	if m.GenerateInstagramPostFn != nil {
		return m.GenerateInstagramPostFn(ctx, scope, strategy, headline)
	}
	return 0, nil
}

func (m *MockDHInstagramClient) PollInstagramPostStatus(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
	if m.PollInstagramPostStatusFn != nil {
		return m.PollInstagramPostStatusFn(ctx, postID)
	}
	return &dh.DHInstagramStatusResponse{RenderStatus: "ready"}, nil
}

// MockDHSocialRepo is a test double for scheduler.DHSocialRepo.
type MockDHSocialRepo struct {
	CreatePostFn        func(ctx context.Context, post *social.SocialPost) error
	UpdatePostCaptionFn func(ctx context.Context, id string, caption, hashtags string) error
	UpdateSlideURLsFn   func(ctx context.Context, id string, urls []string) error
}

func (m *MockDHSocialRepo) CreatePost(ctx context.Context, post *social.SocialPost) error {
	if m.CreatePostFn != nil {
		return m.CreatePostFn(ctx, post)
	}
	return nil
}

func (m *MockDHSocialRepo) UpdatePostCaption(ctx context.Context, id string, caption, hashtags string) error {
	if m.UpdatePostCaptionFn != nil {
		return m.UpdatePostCaptionFn(ctx, id, caption, hashtags)
	}
	return nil
}

func (m *MockDHSocialRepo) UpdateSlideURLs(ctx context.Context, id string, urls []string) error {
	if m.UpdateSlideURLsFn != nil {
		return m.UpdateSlideURLsFn(ctx, id, urls)
	}
	return nil
}
