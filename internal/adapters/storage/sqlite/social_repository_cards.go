package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/social"
)

// This file holds the card- and purchase-query half of SocialRepository: the
// methods that join social_posts with campaign_purchases to select eligible
// cards or to record which purchases belong to which post. Post-level CRUD
// lives in social_repository.go.

func (r *SocialRepository) AddPostCards(ctx context.Context, postID string, cards []social.PostCard) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for post cards %s: %w", postID, err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO social_post_cards (post_id, purchase_id, slide_order) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert post card: %w", err)
	}
	defer stmt.Close() //nolint:errcheck

	// postID (the function parameter) is the source of truth — ignore any
	// PostID set on the incoming PostCard values so mismatched callers can't
	// silently write rows under the wrong parent post.
	for _, c := range cards {
		if _, err := stmt.ExecContext(ctx, postID, c.PurchaseID, c.SlideOrder); err != nil {
			return fmt.Errorf("insert post card %s/%s: %w", postID, c.PurchaseID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit post cards %s: %w", postID, err)
	}
	return nil
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
		return nil, fmt.Errorf("query post cards for %s: %w", postID, err)
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
		return nil, fmt.Errorf("query recent purchase IDs since %s: %w", since, err)
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
		return nil, fmt.Errorf("query existing post membership: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan existing post purchase ID: %w", err)
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
		        COALESCE(p.mm_trend_pct, 0),
		        COALESCE(p.snapshot_date, '')
		 FROM campaign_purchases p
		 WHERE p.id NOT IN (SELECT purchase_id FROM campaign_sales)
		 AND p.front_image_url != ''
		 AND (p.median_cents > 0 OR p.mm_trend_pct != 0)`)
	if err != nil {
		return nil, fmt.Errorf("query unsold snapshots: %w", err)
	}
	return scanRows(ctx, rows, func(rows *sql.Rows) (social.PurchaseSnapshot, error) {
		var s social.PurchaseSnapshot
		err := rows.Scan(&s.PurchaseID, &s.BuyCostCents, &s.MedianCents, &s.Trend30d, &s.MMTrendPct, &s.SnapshotDate)
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
		return nil, fmt.Errorf("query available cards for posts: %w", err)
	}
	return scanRows(ctx, rows, scanPostCardDetail)
}
