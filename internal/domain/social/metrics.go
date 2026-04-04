package social

import (
	"context"
	"time"
)

// PostMetrics holds engagement metrics for a published Instagram post.
type PostMetrics struct {
	ID          int       `json:"id"`
	PostID      string    `json:"postId"`
	Impressions int       `json:"impressions"`
	Reach       int       `json:"reach"`
	Likes       int       `json:"likes"`
	Comments    int       `json:"comments"`
	Saves       int       `json:"saves"`
	Shares      int       `json:"shares"`
	PolledAt    time.Time `json:"polledAt"`
}

// MetricsSummary is a published post's latest metrics plus its post type.
type MetricsSummary struct {
	PostID      string    `json:"postId"`
	PostType    PostType  `json:"postType"`
	CoverTitle  string    `json:"coverTitle"`
	Impressions int       `json:"impressions"`
	Reach       int       `json:"reach"`
	Likes       int       `json:"likes"`
	Comments    int       `json:"comments"`
	Saves       int       `json:"saves"`
	Shares      int       `json:"shares"`
	PublishedAt time.Time `json:"publishedAt"`
}

// MetricsRepository stores and retrieves Instagram post engagement metrics.
type MetricsRepository interface {
	SaveMetrics(ctx context.Context, m *PostMetrics) error
	GetMetrics(ctx context.Context, postID string) ([]PostMetrics, error)
	GetMetricsSummary(ctx context.Context) ([]MetricsSummary, error)
}

// PublishedPost is the minimal info needed by the metrics poller.
type PublishedPost struct {
	PostID          string
	InstagramPostID string
	PublishedAt     time.Time
}
