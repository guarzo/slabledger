package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/social"
)

var _ social.Repository = (*SocialRepository)(nil)

const socialPostColumns = `id, post_type, status, caption, hashtags, cover_title, card_count, instagram_post_id, error_message, created_at, updated_at, slide_urls, background_urls`

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
	return err
}

func (r *SocialRepository) GetPost(ctx context.Context, id string) (*social.SocialPost, error) {
	var p social.SocialPost
	var postType, status, createdAt, updatedAt string
	var slideURLsJSON, backgroundURLsJSON sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT `+socialPostColumns+` FROM social_posts WHERE id = ?`, id,
	).Scan(&p.ID, &postType, &status, &p.Caption, &p.Hashtags, &p.CoverTitle, &p.CardCount, &p.InstagramPostID, &p.ErrorMessage, &createdAt, &updatedAt, &slideURLsJSON, &backgroundURLsJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return scanRows(ctx, rows, scanSocialPost)
}

func (r *SocialRepository) UpdatePostStatus(ctx context.Context, id string, status social.PostStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found", id)
	}
	return nil
}

func (r *SocialRepository) UpdatePostCaption(ctx context.Context, id string, caption, hashtags string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET caption = ?, hashtags = ?, updated_at = ? WHERE id = ?`,
		caption, hashtags, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found", id)
	}
	return nil
}

func (r *SocialRepository) SetPublished(ctx context.Context, id string, instagramPostID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET status = ?, instagram_post_id = ?, updated_at = ? WHERE id = ?`,
		string(social.PostStatusPublished), instagramPostID, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found", id)
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
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found or not in publishable state", id)
	}
	return nil
}

func (r *SocialRepository) SetError(ctx context.Context, id string, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
		string(social.PostStatusFailed), errorMessage, time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}

func (r *SocialRepository) DeletePost(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM social_posts WHERE id = ?`, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found", id)
	}
	return nil
}

func (r *SocialRepository) AddPostCards(ctx context.Context, postID string, cards []social.PostCard) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO social_post_cards (post_id, purchase_id, slide_order) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close() //nolint:errcheck

	for _, c := range cards {
		if _, err := stmt.ExecContext(ctx, c.PostID, c.PurchaseID, c.SlideOrder); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanPostCardDetail(rows *sql.Rows) (social.PostCardDetail, error) {
	var c social.PostCardDetail
	var createdAt string
	var sold int
	err := rows.Scan(&c.PurchaseID, &c.SlideOrder, &c.CardName, &c.SetName, &c.CardNumber,
		&c.GradeValue, &c.Grader, &c.CertNumber, &c.FrontImageURL, &c.AskingPriceCents,
		&c.CLValueCents, &c.Trend30d, &createdAt, &sold)
	c.CreatedAt = parseSQLiteTime(createdAt)
	c.Sold = sold != 0
	return c, err
}

func (r *SocialRepository) ListPostCards(ctx context.Context, postID string) ([]social.PostCardDetail, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT spc.purchase_id, spc.slide_order,
		        p.card_name, COALESCE(p.set_name, ''), COALESCE(p.card_number, ''),
		        p.grade_value, COALESCE(p.grader, 'PSA'), COALESCE(p.cert_number, ''),
		        COALESCE(p.front_image_url, ''), COALESCE(p.reviewed_price_cents, 0),
		        COALESCE(p.cl_value_cents, 0), COALESCE(p.trend_30d, 0),
		        p.created_at,
		        CASE WHEN cs.purchase_id IS NOT NULL THEN 1 ELSE 0 END as sold
		 FROM social_post_cards spc
		 JOIN campaign_purchases p ON p.id = spc.purchase_id
		 LEFT JOIN campaign_sales cs ON cs.purchase_id = p.id
		 WHERE spc.post_id = ?
		 ORDER BY spc.slide_order`, postID)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, scanPostCardDetail)
}

func (r *SocialRepository) GetRecentPurchaseIDs(ctx context.Context, since string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id FROM campaign_purchases p
		 WHERE p.created_at >= ?
		 AND p.front_image_url != ''
		 AND p.id NOT IN (SELECT purchase_id FROM campaign_sales)
		 ORDER BY p.created_at DESC`, since)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rows *sql.Rows) (string, error) {
		var id string
		err := rows.Scan(&id)
		return id, err
	})
}

func (r *SocialRepository) GetPurchaseIDsInExistingPosts(ctx context.Context, purchaseIDs []string, postType social.PostType) (map[string]bool, error) {
	if len(purchaseIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(purchaseIDs))
	args := make([]any, 0, len(purchaseIDs)+2)
	for i, id := range purchaseIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, string(postType), string(social.PostStatusRejected))

	query := fmt.Sprintf(
		`SELECT DISTINCT spc.purchase_id
		 FROM social_post_cards spc
		 JOIN social_posts sp ON sp.id = spc.post_id
		 WHERE spc.purchase_id IN (%s)
		 AND sp.post_type = ?
		 AND sp.status != ?`,
		strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, rows.Err()
}

func (r *SocialRepository) GetUnsoldPurchasesWithSnapshots(ctx context.Context) ([]social.PurchaseSnapshot, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, p.buy_cost_cents,
		        COALESCE(p.median_cents, 0),
		        COALESCE(p.trend_30d, 0),
		        COALESCE(p.snapshot_date, '')
		 FROM campaign_purchases p
		 WHERE p.id NOT IN (SELECT purchase_id FROM campaign_sales)
		 AND p.front_image_url != ''
		 AND p.median_cents > 0`)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rows *sql.Rows) (social.PurchaseSnapshot, error) {
		var s social.PurchaseSnapshot
		err := rows.Scan(&s.PurchaseID, &s.BuyCostCents, &s.MedianCents, &s.Trend30d, &s.SnapshotDate)
		return s, err
	})
}

func (r *SocialRepository) GetAvailableCardsForPosts(ctx context.Context) ([]social.PostCardDetail, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, 0 as slide_order,
		        p.card_name, COALESCE(p.set_name, ''), COALESCE(p.card_number, ''),
		        p.grade_value, COALESCE(p.grader, 'PSA'), COALESCE(p.cert_number, ''),
		        COALESCE(p.front_image_url, ''), COALESCE(p.reviewed_price_cents, 0),
		        COALESCE(p.cl_value_cents, 0), COALESCE(p.trend_30d, 0),
		        p.created_at,
		        0 as sold
		 FROM campaign_purchases p
		 WHERE p.front_image_url <> ''
		 AND p.id NOT IN (SELECT purchase_id FROM campaign_sales)
		 AND p.id NOT IN (
		     SELECT spc.purchase_id FROM social_post_cards spc
		     JOIN social_posts sp ON sp.id = spc.post_id
		     WHERE sp.status NOT IN (?, ?)
		 )
		 ORDER BY p.created_at DESC`,
		string(social.PostStatusRejected), string(social.PostStatusFailed))
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, scanPostCardDetail)
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
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found", id)
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
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found", id)
	}
	return nil
}

func (r *SocialRepository) UpdateCoverTitle(ctx context.Context, id string, title string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE social_posts SET cover_title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("post %s not found", id)
	}
	return nil
}
