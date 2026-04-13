package social

import "context"

// mockSocialRepo is a test double for social.Repository using the Fn-field pattern.
// Set any Fn field to override the default no-op behavior for that method.
type mockSocialRepo struct {
	GetPostFn                   func(ctx context.Context, id string) (*SocialPost, error)
	CreatePostFn                func(ctx context.Context, post *SocialPost) error
	UpdatePostStatusFn          func(ctx context.Context, id string, status PostStatus) error
	DeletePostFn                func(ctx context.Context, id string) error
	ListPostCardsFn             func(ctx context.Context, postID string) ([]PostCardDetail, error)
	GetRecentPurchaseIDsFn      func(ctx context.Context, since string) ([]string, error)
	GetPurchaseIDsInExistingFn  func(ctx context.Context, ids []string, pt PostType) (map[string]bool, error)
	GetUnsoldPurchasesFn        func(ctx context.Context) ([]PurchaseSnapshot, error)
	UpdatePostCaptionFn         func(ctx context.Context, id, caption, hashtags string) error
	AddPostCardsFn              func(ctx context.Context, postID string, cards []PostCard) error
	ListPostsFn                 func(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error)
	SetPublishedFn              func(ctx context.Context, id, igPostID string) error
	SetPublishingFn             func(ctx context.Context, id string) error
	SetErrorFn                  func(ctx context.Context, id, errMsg string) error
	GetAvailableCardsForPostsFn func(ctx context.Context) ([]PostCardDetail, error)
	UpdateSlideURLsFn           func(ctx context.Context, id string, urls []string) error
	UpdateCoverTitleFn          func(ctx context.Context, id string, title string) error
	UpdateBackgroundURLsFn      func(ctx context.Context, id string, urls []string) error
}

var _ Repository = (*mockSocialRepo)(nil)

func (m *mockSocialRepo) GetPost(ctx context.Context, id string) (*SocialPost, error) {
	if m.GetPostFn != nil {
		return m.GetPostFn(ctx, id)
	}
	return nil, nil
}

func (m *mockSocialRepo) CreatePost(ctx context.Context, post *SocialPost) error {
	if m.CreatePostFn != nil {
		return m.CreatePostFn(ctx, post)
	}
	return nil
}

func (m *mockSocialRepo) UpdatePostStatus(ctx context.Context, id string, status PostStatus) error {
	if m.UpdatePostStatusFn != nil {
		return m.UpdatePostStatusFn(ctx, id, status)
	}
	return nil
}

func (m *mockSocialRepo) DeletePost(ctx context.Context, id string) error {
	if m.DeletePostFn != nil {
		return m.DeletePostFn(ctx, id)
	}
	return nil
}

func (m *mockSocialRepo) ListPostCards(ctx context.Context, postID string) ([]PostCardDetail, error) {
	if m.ListPostCardsFn != nil {
		return m.ListPostCardsFn(ctx, postID)
	}
	return nil, nil
}

func (m *mockSocialRepo) GetRecentPurchaseIDs(ctx context.Context, since string) ([]string, error) {
	if m.GetRecentPurchaseIDsFn != nil {
		return m.GetRecentPurchaseIDsFn(ctx, since)
	}
	return nil, nil
}

func (m *mockSocialRepo) GetPurchaseIDsInExistingPosts(ctx context.Context, ids []string, pt PostType) (map[string]bool, error) {
	if m.GetPurchaseIDsInExistingFn != nil {
		return m.GetPurchaseIDsInExistingFn(ctx, ids, pt)
	}
	return nil, nil
}

func (m *mockSocialRepo) GetUnsoldPurchasesWithSnapshots(ctx context.Context) ([]PurchaseSnapshot, error) {
	if m.GetUnsoldPurchasesFn != nil {
		return m.GetUnsoldPurchasesFn(ctx)
	}
	return nil, nil
}

func (m *mockSocialRepo) UpdatePostCaption(ctx context.Context, id, caption, hashtags string) error {
	if m.UpdatePostCaptionFn != nil {
		return m.UpdatePostCaptionFn(ctx, id, caption, hashtags)
	}
	return nil
}

func (m *mockSocialRepo) AddPostCards(ctx context.Context, postID string, cards []PostCard) error {
	if m.AddPostCardsFn != nil {
		return m.AddPostCardsFn(ctx, postID, cards)
	}
	return nil
}

func (m *mockSocialRepo) ListPosts(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error) {
	if m.ListPostsFn != nil {
		return m.ListPostsFn(ctx, status, limit, offset)
	}
	return nil, nil
}

func (m *mockSocialRepo) SetPublished(ctx context.Context, id, igPostID string) error {
	if m.SetPublishedFn != nil {
		return m.SetPublishedFn(ctx, id, igPostID)
	}
	return nil
}

func (m *mockSocialRepo) SetPublishing(ctx context.Context, id string) error {
	if m.SetPublishingFn != nil {
		return m.SetPublishingFn(ctx, id)
	}
	return nil
}

func (m *mockSocialRepo) SetError(ctx context.Context, id, errMsg string) error {
	if m.SetErrorFn != nil {
		return m.SetErrorFn(ctx, id, errMsg)
	}
	return nil
}

func (m *mockSocialRepo) GetAvailableCardsForPosts(ctx context.Context) ([]PostCardDetail, error) {
	if m.GetAvailableCardsForPostsFn != nil {
		return m.GetAvailableCardsForPostsFn(ctx)
	}
	return nil, nil
}

func (m *mockSocialRepo) UpdateSlideURLs(ctx context.Context, id string, urls []string) error {
	if m.UpdateSlideURLsFn != nil {
		return m.UpdateSlideURLsFn(ctx, id, urls)
	}
	return nil
}

func (m *mockSocialRepo) UpdateCoverTitle(ctx context.Context, id string, title string) error {
	if m.UpdateCoverTitleFn != nil {
		return m.UpdateCoverTitleFn(ctx, id, title)
	}
	return nil
}

func (m *mockSocialRepo) UpdateBackgroundURLs(ctx context.Context, id string, urls []string) error {
	if m.UpdateBackgroundURLsFn != nil {
		return m.UpdateBackgroundURLsFn(ctx, id, urls)
	}
	return nil
}
