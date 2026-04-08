package social

import "context"

// Repository defines persistence operations for social posts.
type Repository interface {
	CreatePost(ctx context.Context, post *SocialPost) error
	GetPost(ctx context.Context, id string) (*SocialPost, error)
	ListPosts(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error)
	UpdatePostStatus(ctx context.Context, id string, status PostStatus) error
	UpdatePostCaption(ctx context.Context, id string, caption, hashtags string) error
	SetPublished(ctx context.Context, id string, instagramPostID string) error
	SetPublishing(ctx context.Context, id string) error
	SetError(ctx context.Context, id string, errorMessage string) error
	DeletePost(ctx context.Context, id string) error

	AddPostCards(ctx context.Context, postID string, cards []PostCard) error
	ListPostCards(ctx context.Context, postID string) ([]PostCardDetail, error)

	// GetRecentPurchaseIDs returns purchase IDs created since the given time.
	GetRecentPurchaseIDs(ctx context.Context, since string) ([]string, error)

	// GetPurchaseIDsInExistingPosts returns the subset of purchaseIDs already in a non-rejected post of the given type.
	GetPurchaseIDsInExistingPosts(ctx context.Context, purchaseIDs []string, postType PostType) (map[string]bool, error)

	// GetUnsoldPurchasesWithSnapshots returns purchase IDs + snapshot data for detection queries.
	GetUnsoldPurchasesWithSnapshots(ctx context.Context) ([]PurchaseSnapshot, error)

	// GetAvailableCardsForPosts returns unsold purchases with images that aren't in an existing post.
	GetAvailableCardsForPosts(ctx context.Context) ([]PostCardDetail, error)

	// UpdateSlideURLs stores the rendered slide image URLs for a post.
	UpdateSlideURLs(ctx context.Context, id string, urls []string) error

	// UpdateCoverTitle updates the cover title of a post.
	UpdateCoverTitle(ctx context.Context, id string, title string) error

	// UpdateBackgroundURLs stores the AI-generated background image URLs for a post.
	UpdateBackgroundURLs(ctx context.Context, id string, urls []string) error
}

// PurchaseSnapshot holds the fields needed for price mover and hot deal detection.
type PurchaseSnapshot struct {
	PurchaseID   string
	BuyCostCents int
	MedianCents  int
	Trend30d     float64
	MMTrendPct   float64 // Market Movers 30-day trend % — used as fallback when Trend30d is 0
	SnapshotDate string
}
