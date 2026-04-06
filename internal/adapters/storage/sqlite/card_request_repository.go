package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrCardRequestNotFound is returned when a card request submission is not found.
var ErrCardRequestNotFound = errors.New("card request not found")

// ErrCardRequestAlreadyClaimed is returned when a card request has already been claimed for processing.
var ErrCardRequestAlreadyClaimed = errors.New("card request already claimed")

// CardRequestSubmission represents a row in the card_request_submissions table.
type CardRequestSubmission struct {
	ID                  int64      `json:"id"`
	CertNumber          string     `json:"certNumber"`
	Grader              string     `json:"grader"`
	CardName            string     `json:"cardName"`
	SetName             string     `json:"setName"`
	CardNumber          string     `json:"cardNumber"`
	Grade               string     `json:"grade"`
	FrontImageURL       string     `json:"frontImageUrl"`
	Variant             string     `json:"variant"`
	Status              string     `json:"status"`
	CardHedgerRequestID string     `json:"cardhedgerRequestId"`
	SubmittedAt         *time.Time `json:"submittedAt"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

// CardRequestRepository manages the card_request_submissions table.
type CardRequestRepository struct {
	db *sql.DB
}

// NewCardRequestRepository creates a new repository backed by the given database.
func NewCardRequestRepository(db *sql.DB) *CardRequestRepository {
	return &CardRequestRepository{db: db}
}

// TrackMissingCert records a cert whose card has not been linked to a pricing source.
// Only inserts if no row already exists for the (grader, cert) pair.
func (r *CardRequestRepository) TrackMissingCert(ctx context.Context, cert, grader, grade, description string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO card_request_submissions (cert_number, grader, grade, card_name, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(grader, cert_number) DO NOTHING`,
		cert, grader, grade, description,
	)
	return err
}

// EnrichPendingFromPurchases fills card_name, set_name, card_number, and front_image_url
// from the campaign_purchases table for any pending submissions that have a matching
// purchase row with at least one differing non-empty field.
func (r *CardRequestRepository) EnrichPendingFromPurchases(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE card_request_submissions
		 SET card_name = COALESCE(
		       NULLIF((SELECT p.card_name FROM campaign_purchases p WHERE p.cert_number = card_request_submissions.cert_number LIMIT 1), ''),
		       card_name),
		     set_name = COALESCE(
		       NULLIF((SELECT p.set_name FROM campaign_purchases p WHERE p.cert_number = card_request_submissions.cert_number LIMIT 1), ''),
		       set_name),
		     card_number = COALESCE(
		       NULLIF((SELECT p.card_number FROM campaign_purchases p WHERE p.cert_number = card_request_submissions.cert_number LIMIT 1), ''),
		       card_number),
		     front_image_url = COALESCE(
		       NULLIF((SELECT p.front_image_url FROM campaign_purchases p WHERE p.cert_number = card_request_submissions.cert_number AND p.front_image_url != '' LIMIT 1), ''),
		       front_image_url),
		     updated_at = CURRENT_TIMESTAMP
		 WHERE status = 'pending'
		 AND EXISTS (
		   SELECT 1 FROM campaign_purchases p
		   WHERE p.cert_number = card_request_submissions.cert_number
		   AND (
		     (p.card_name != '' AND p.card_name != card_request_submissions.card_name)
		     OR (p.set_name != '' AND p.set_name != card_request_submissions.set_name)
		     OR (p.card_number != '' AND p.card_number != card_request_submissions.card_number)
		     OR (p.front_image_url != '' AND p.front_image_url != card_request_submissions.front_image_url)
		   )
		 )`)
	return err
}

// ListAll returns all card request submissions ordered by status then created_at.
func (r *CardRequestRepository) ListAll(ctx context.Context) (_ []CardRequestSubmission, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, cert_number, grader, card_name, set_name, card_number, grade,
		        front_image_url, variant, status, cardhedger_request_id,
		        submitted_at, created_at, updated_at
		 FROM card_request_submissions
		 ORDER BY
		   CASE status WHEN 'pending' THEN 0 WHEN 'submitted' THEN 1 ELSE 2 END,
		   created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var results []CardRequestSubmission
	for rows.Next() {
		var s CardRequestSubmission
		if err := rows.Scan(
			&s.ID, &s.CertNumber, &s.Grader, &s.CardName, &s.SetName, &s.CardNumber,
			&s.Grade, &s.FrontImageURL, &s.Variant, &s.Status, &s.CardHedgerRequestID,
			&s.SubmittedAt, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetByID returns a single card request submission by ID.
func (r *CardRequestRepository) GetByID(ctx context.Context, id int64) (*CardRequestSubmission, error) {
	var s CardRequestSubmission
	err := r.db.QueryRowContext(ctx,
		`SELECT id, cert_number, grader, card_name, set_name, card_number, grade,
		        front_image_url, variant, status, cardhedger_request_id,
		        submitted_at, created_at, updated_at
		 FROM card_request_submissions WHERE id = ?`, id,
	).Scan(
		&s.ID, &s.CertNumber, &s.Grader, &s.CardName, &s.SetName, &s.CardNumber,
		&s.Grade, &s.FrontImageURL, &s.Variant, &s.Status, &s.CardHedgerRequestID,
		&s.SubmittedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCardRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ClaimForProcessing atomically transitions a submission from "pending" to "processing".
// Returns ErrCardRequestAlreadyClaimed if the row is no longer pending.
func (r *CardRequestRepository) ClaimForProcessing(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE card_request_submissions
		 SET status = 'processing', updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status = 'pending'`, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("id %d: %w", id, ErrCardRequestAlreadyClaimed)
	}
	return nil
}

// RevertClaim transitions a submission from "processing" back to "pending".
func (r *CardRequestRepository) RevertClaim(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE card_request_submissions
		 SET status = 'pending', updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status = 'processing'`, id,
	)
	return err
}

// UpdateSubmitted marks a submission as submitted with the external request ID.
func (r *CardRequestRepository) UpdateSubmitted(ctx context.Context, id int64, requestID string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`UPDATE card_request_submissions
		 SET status = 'submitted', cardhedger_request_id = ?, submitted_at = ?, updated_at = ?
		 WHERE id = ?`,
		requestID, now, now, id,
	)
	return err
}

// CountByStatus returns a map of status -> count.
func (r *CardRequestRepository) CountByStatus(ctx context.Context) (_ map[string]int, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM card_request_submissions GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}
