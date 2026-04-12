package social

import "time"

// PostType identifies the kind of social post.
type PostType string

const (
	PostTypeNewArrivals PostType = "new_arrivals"
	PostTypePriceMovers PostType = "price_movers"
	PostTypeHotDeals    PostType = "hot_deals"
	PostTypeDHInstagram PostType = "dh_instagram"
)

// PostStatus tracks the lifecycle of a social post draft.
type PostStatus string

const (
	PostStatusDraft      PostStatus = "draft"
	PostStatusPublishing PostStatus = "publishing"
	PostStatusPublished  PostStatus = "published"
	PostStatusFailed     PostStatus = "failed"

	// Deprecated — kept for backward compatibility with existing data.
	PostStatusApproved PostStatus = "approved"
	PostStatusRejected PostStatus = "rejected"
)

// SocialPost represents a social media content draft.
type SocialPost struct {
	ID              string     `json:"id"`
	PostType        PostType   `json:"postType"`
	Status          PostStatus `json:"status"`
	Caption         string     `json:"caption"`
	Hashtags        string     `json:"hashtags"`
	CoverTitle      string     `json:"coverTitle"`
	CardCount       int        `json:"cardCount"`
	InstagramPostID string     `json:"instagramPostId,omitempty"`
	ErrorMessage    string     `json:"errorMessage,omitempty"`
	SlideURLs       []string   `json:"slideUrls,omitempty"`
	BackgroundURLs  []string   `json:"backgroundUrls,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// PostCard links a purchase to a social post with ordering.
type PostCard struct {
	PostID     string `json:"postId"`
	PurchaseID string `json:"purchaseId"`
	SlideOrder int    `json:"slideOrder"`
}

// PostCardDetail is PostCard enriched with purchase data for API responses.
type PostCardDetail struct {
	PurchaseID       string    `json:"purchaseId"`
	SlideOrder       int       `json:"slideOrder"`
	CardName         string    `json:"cardName"`
	SetName          string    `json:"setName"`
	CardNumber       string    `json:"cardNumber"`
	GradeValue       float64   `json:"gradeValue"`
	Grader           string    `json:"grader"`
	CertNumber       string    `json:"certNumber"`
	FrontImageURL    string    `json:"frontImageUrl"`
	AskingPriceCents int       `json:"askingPriceCents"`
	CLValueCents     int       `json:"clValueCents"`
	Trend30d         float64   `json:"trend30d"`
	CreatedAt        time.Time `json:"createdAt"`
	Sold             bool      `json:"sold"`
}

// PostDetail is a SocialPost with its associated card details.
type PostDetail struct {
	SocialPost
	Cards []PostCardDetail `json:"cards"`
}
