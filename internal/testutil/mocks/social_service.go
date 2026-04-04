package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/social"
)

type MockSocialService struct {
	DetectAndGenerateFn func(ctx context.Context) (int, error)
	ListPostsFn         func(ctx context.Context, status *social.PostStatus, limit, offset int) ([]social.SocialPost, error)
	GetPostFn           func(ctx context.Context, id string) (*social.PostDetail, error)
	UpdateCaptionFn     func(ctx context.Context, id string, caption, hashtags string) error
	DeleteFn            func(ctx context.Context, id string) error
	PublishFn           func(ctx context.Context, id string) error
	RegenerateCaptionFn func(ctx context.Context, id string, stream func(ai.StreamEvent)) error
	WaitFn              func()
}

var _ social.Service = (*MockSocialService)(nil)

func (m *MockSocialService) DetectAndGenerate(ctx context.Context) (int, error) {
	if m.DetectAndGenerateFn != nil {
		return m.DetectAndGenerateFn(ctx)
	}
	return 0, nil
}

func (m *MockSocialService) ListPosts(ctx context.Context, status *social.PostStatus, limit, offset int) ([]social.SocialPost, error) {
	if m.ListPostsFn != nil {
		return m.ListPostsFn(ctx, status, limit, offset)
	}
	return []social.SocialPost{}, nil
}

func (m *MockSocialService) GetPost(ctx context.Context, id string) (*social.PostDetail, error) {
	if m.GetPostFn != nil {
		return m.GetPostFn(ctx, id)
	}
	return nil, nil
}

func (m *MockSocialService) UpdateCaption(ctx context.Context, id string, caption, hashtags string) error {
	if m.UpdateCaptionFn != nil {
		return m.UpdateCaptionFn(ctx, id, caption, hashtags)
	}
	return nil
}

func (m *MockSocialService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return nil
}

func (m *MockSocialService) Publish(ctx context.Context, id string) error {
	if m.PublishFn != nil {
		return m.PublishFn(ctx, id)
	}
	return nil
}

func (m *MockSocialService) RegenerateCaption(ctx context.Context, id string, stream func(ai.StreamEvent)) error {
	if m.RegenerateCaptionFn != nil {
		return m.RegenerateCaptionFn(ctx, id, stream)
	}
	return nil
}

func (m *MockSocialService) Wait() {
	if m.WaitFn != nil {
		m.WaitFn()
	}
}
