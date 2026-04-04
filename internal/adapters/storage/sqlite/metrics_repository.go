package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/social"
)

// MetricsRepository implements social.MetricsRepository using SQLite.
type MetricsRepository struct {
	db *sql.DB
}

// NewMetricsRepository creates a new metrics repository.
func NewMetricsRepository(db *sql.DB) *MetricsRepository {
	return &MetricsRepository{db: db}
}

func (r *MetricsRepository) SaveMetrics(ctx context.Context, m *social.PostMetrics) error {
	if m == nil {
		return fmt.Errorf("cannot save nil metrics")
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO instagram_post_metrics (post_id, impressions, reach, likes, comments, saves, shares, polled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.PostID, m.Impressions, m.Reach, m.Likes, m.Comments, m.Saves, m.Shares,
		m.PolledAt.Format(time.RFC3339))
	return err
}

func (r *MetricsRepository) GetMetrics(ctx context.Context, postID string) ([]social.PostMetrics, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, post_id, impressions, reach, likes, comments, saves, shares, polled_at
		 FROM instagram_post_metrics WHERE post_id = ? ORDER BY polled_at DESC`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []social.PostMetrics
	for rows.Next() {
		var m social.PostMetrics
		var polledAt string
		if err := rows.Scan(&m.ID, &m.PostID, &m.Impressions, &m.Reach, &m.Likes,
			&m.Comments, &m.Saves, &m.Shares, &polledAt); err != nil {
			return nil, err
		}
		var parseErr error
		m.PolledAt, parseErr = time.Parse(time.RFC3339, polledAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse polled_at for post %s: %w", m.PostID, parseErr)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *MetricsRepository) GetMetricsSummary(ctx context.Context) ([]social.MetricsSummary, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT m.post_id, sp.post_type, sp.cover_title,
		        m.impressions, m.reach, m.likes, m.comments, m.saves, m.shares,
		        sp.updated_at
		 FROM instagram_post_metrics m
		 JOIN social_posts sp ON sp.id = m.post_id
		 WHERE sp.status = ?
		   AND m.id = (SELECT MAX(m2.id) FROM instagram_post_metrics m2 WHERE m2.post_id = m.post_id)
		 ORDER BY sp.updated_at DESC`,
		string(social.PostStatusPublished))
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []social.MetricsSummary
	for rows.Next() {
		var s social.MetricsSummary
		var publishedAt string
		if err := rows.Scan(&s.PostID, &s.PostType, &s.CoverTitle,
			&s.Impressions, &s.Reach, &s.Likes, &s.Comments, &s.Saves, &s.Shares,
			&publishedAt); err != nil {
			return nil, err
		}
		var parseErr error
		s.PublishedAt, parseErr = time.Parse(time.RFC3339, publishedAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse published_at for post %s: %w", s.PostID, parseErr)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *MetricsRepository) GetPublishedPostIDs(ctx context.Context, since time.Time) ([]social.PublishedPost, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, instagram_post_id, updated_at
		 FROM social_posts
		 WHERE status = ? AND instagram_post_id != '' AND updated_at >= ?
		 ORDER BY updated_at DESC`,
		string(social.PostStatusPublished), since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []social.PublishedPost
	for rows.Next() {
		var p social.PublishedPost
		var updatedAt string
		if err := rows.Scan(&p.PostID, &p.InstagramPostID, &updatedAt); err != nil {
			return nil, err
		}
		var parseErr error
		p.PublishedAt, parseErr = time.Parse(time.RFC3339, updatedAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse updated_at for post %s: %w", p.PostID, parseErr)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
