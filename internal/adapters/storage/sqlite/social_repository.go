package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/social"
)

var _ social.Repository = (*SocialRepository)(nil)

const socialPostColumns = `id, post_type, status, caption, hashtags, cover_title, card_count, instagram_post_id, error_message, created_at, updated_at, slide_urls, background_urls`

// captionPlaceholder is the sentinel value written by generateCaptionAsync while
// a post's caption is being generated. FetchEligibleDraft excludes these posts.
const captionPlaceholder = "Generating..."

func scanSocialPost(rows *sql.Rows) (social.SocialPost, error) {
	var p social.SocialPost
	var postType, status, createdAt, updatedAt string
	var slideURLsJSON, backgroundURLsJSON sql.NullString
	if err := rows.Scan(&p.ID, &postType, &status, &p.Caption, &p.Hashtags, &p.CoverTitle, &p.CardCount, &p.InstagramPostID, &p.ErrorMessage, &createdAt, &updatedAt, &slideURLsJSON, &backgroundURLsJSON); err != nil {
		return p, err
	}
	p.PostType = social.PostType(postType)
	p.Status = social.PostStatus(status)
	p.CreatedAt = parseSQLiteTime(createdAt)
	p.UpdatedAt = parseSQLiteTime(updatedAt)
	if slideURLsJSON.Valid && slideURLsJSON.String != "" {
		if err := json.Unmarshal([]byte(slideURLsJSON.String), &p.SlideURLs); err != nil {
			return p, fmt.Errorf("unmarshal slide URLs: %w", err)
		}
	}
	if backgroundURLsJSON.Valid && backgroundURLsJSON.String != "" {
		if err := json.Unmarshal([]byte(backgroundURLsJSON.String), &p.BackgroundURLs); err != nil {
			return p, fmt.Errorf("unmarshal background URLs: %w", err)
		}
	}
	return p, nil
}

// SocialRepository implements social.Repository using SQLite.
type SocialRepository struct {
	db *sql.DB
}

// NewSocialRepository creates a new social repository.
func NewSocialRepository(db *sql.DB) *SocialRepository {
	return &SocialRepository{db: db}
}

func (r *SocialRepository) CreatePost(ctx context.Context, post *social.SocialPost) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO social_posts (id, post_type, status, caption, hashtags, cover_title, card_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		post.ID, string(post.PostType), string(post.Status),
		post.Caption, post.Hashtags, post.CoverTitle, post.CardCount,
		post.CreatedAt.Format(time.RFC3339), post.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert social post %s: %w", post.ID, err)
	}
	return nil
}

// GetPost returns the social post with the given ID, or (nil, nil) if no such post exists.
// The nil-on-missing contract is intentional and verified by social_repository_test.go —
// callers treat "no post" as a valid, non-error state.
func (r *SocialRepository) GetPost(ctx context.Context, id string) (*social.SocialPost, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+socialPostColumns+` FROM social_posts WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("query social post %s: %w", id, err)
	}
	defer rows.Close() //nolint:errcheck
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate social post %s: %w", id, err)
		}
		return nil, nil
	}
	p, err := scanSocialPost(rows)
	if err != nil {
		return nil, fmt.Errorf("scan social post %s: %w", id, err)
	}
	return &p, nil
}

func (r *SocialRepository) ListPosts(ctx context.Context, status *social.PostStatus, limit, offset int) ([]social.SocialPost, error) {
	query := `SELECT ` + socialPostColumns + ` FROM social_posts WHERE 1=1`
	var args []any

	if status != nil {
		query += " AND status = ?"
		args = append(args, string(*status))
	}
	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	} else if offset > 0 {
		query += " LIMIT -1"
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query social posts: %w", err)
	}
	return scanRows(ctx, rows, scanSocialPost)
}

func (r *SocialRepository) UpdatePostStatus(ctx context.Context, id string, status social.PostStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update social post status %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s: %w", id, social.ErrPostNotFound)
	}
	return nil
}

func (r *SocialRepository) UpdatePostCaption(ctx context.Context, id string, caption, hashtags string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET caption = ?, hashtags = ?, updated_at = ? WHERE id = ?`,
		caption, hashtags, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update social post caption %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s: %w", id, social.ErrPostNotFound)
	}
	return nil
}

func (r *SocialRepository) SetPublished(ctx context.Context, id string, instagramPostID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET status = ?, instagram_post_id = ?, updated_at = ? WHERE id = ?`,
		string(social.PostStatusPublished), instagramPostID, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("mark social post published %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s: %w", id, social.ErrPostNotFound)
	}
	return nil
}

func (r *SocialRepository) SetPublishing(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET status = ?, error_message = '', updated_at = ? WHERE id = ? AND status IN (?, ?, ?)`,
		string(social.PostStatusPublishing), time.Now().UTC().Format(time.RFC3339),
		id, string(social.PostStatusDraft), string(social.PostStatusFailed), string(social.PostStatusApproved),
	)
	if err != nil {
		return fmt.Errorf("mark social post publishing %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found or not in publishable state: %w", id, social.ErrPostNotFound)
	}
	return nil
}

// SetError marks a post as failed with the given error message. Returns
// social.ErrPostNotFound if no post with the given id exists — callers in the
// publish-failure path should treat that case as "post was deleted
// concurrently, nothing to update" rather than as a stuck-in-publishing
// condition.
func (r *SocialRepository) SetError(ctx context.Context, id string, errorMessage string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
		string(social.PostStatusFailed), errorMessage, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("set social post error %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s: %w", id, social.ErrPostNotFound)
	}
	return nil
}

func (r *SocialRepository) DeletePost(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM social_posts WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("delete social post %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s: %w", id, social.ErrPostNotFound)
	}
	return nil
}

func (r *SocialRepository) UpdateSlideURLs(ctx context.Context, id string, urls []string) error {
	urlsJSON, err := json.Marshal(urls)
	if err != nil {
		return fmt.Errorf("marshal slide URLs: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET slide_urls = ?, updated_at = ? WHERE id = ?`,
		string(urlsJSON), time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update slide URLs for post %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s: %w", id, social.ErrPostNotFound)
	}
	return nil
}

func (r *SocialRepository) UpdateBackgroundURLs(ctx context.Context, id string, urls []string) error {
	urlsJSON, err := json.Marshal(urls)
	if err != nil {
		return fmt.Errorf("marshal background URLs: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET background_urls = ?, updated_at = ? WHERE id = ?`,
		string(urlsJSON), time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update background URLs for post %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s: %w", id, social.ErrPostNotFound)
	}
	return nil
}

func (r *SocialRepository) UpdateCoverTitle(ctx context.Context, id string, title string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET cover_title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update cover title for post %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s: %w", id, social.ErrPostNotFound)
	}
	return nil
}

// CountPublishedToday returns the number of posts published on the current calendar day
// in UTC. updated_at is used as a proxy for publish time — SetPublished stamps it
// at publish time. date('now') operates in UTC, matching the UTC-stored timestamps.
func (r *SocialRepository) CountPublishedToday(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM social_posts
		 WHERE status = 'published'
		 AND date(updated_at) = date('now')`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count published today: %w", err)
	}
	return count, nil
}

// FetchEligibleDraft returns the oldest eligible draft post with its card details.
// Eligibility criteria:
//   - status = 'draft'
//   - caption is not empty and not the placeholder value 'Generating...'
//   - created_at is within the last 7 days (avoids stale drafts)
//
// Returns nil, nil if no eligible draft exists.
func (r *SocialRepository) FetchEligibleDraft(ctx context.Context) (*social.PostDetail, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+socialPostColumns+`
		 FROM social_posts
		 WHERE status = 'draft'
		 AND caption != ''
		 AND caption != ?
		 AND datetime(created_at) >= datetime('now', '-7 days')
		 ORDER BY created_at ASC
		 LIMIT 1`,
		captionPlaceholder,
	)

	var p social.SocialPost
	var postType, status, createdAt, updatedAt string
	var slideURLsJSON, backgroundURLsJSON sql.NullString
	if err := row.Scan(&p.ID, &postType, &status, &p.Caption, &p.Hashtags, &p.CoverTitle,
		&p.CardCount, &p.InstagramPostID, &p.ErrorMessage, &createdAt, &updatedAt,
		&slideURLsJSON, &backgroundURLsJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan eligible draft: %w", err)
	}
	p.PostType = social.PostType(postType)
	p.Status = social.PostStatus(status)
	p.CreatedAt = parseSQLiteTime(createdAt)
	p.UpdatedAt = parseSQLiteTime(updatedAt)
	if slideURLsJSON.Valid && slideURLsJSON.String != "" {
		if err := json.Unmarshal([]byte(slideURLsJSON.String), &p.SlideURLs); err != nil {
			return nil, fmt.Errorf("unmarshal slide URLs: %w", err)
		}
	}
	if backgroundURLsJSON.Valid && backgroundURLsJSON.String != "" {
		if err := json.Unmarshal([]byte(backgroundURLsJSON.String), &p.BackgroundURLs); err != nil {
			return nil, fmt.Errorf("unmarshal background URLs: %w", err)
		}
	}

	cards, err := r.ListPostCards(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("list cards for draft: %w", err)
	}
	return &social.PostDetail{SocialPost: p, Cards: cards}, nil
}
