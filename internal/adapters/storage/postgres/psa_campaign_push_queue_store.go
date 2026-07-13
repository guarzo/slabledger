package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// PSACampaignPushQueueStore persists queued campaign edits and their
// approval/push lifecycle (migration 000017).
type PSACampaignPushQueueStore struct {
	db *sql.DB
}

var _ psacampaign.PushQueueStore = (*PSACampaignPushQueueStore)(nil)

func NewPSACampaignPushQueueStore(db *sql.DB) *PSACampaignPushQueueStore {
	return &PSACampaignPushQueueStore{db: db}
}

// Enqueue inserts a new pending push-queue row.
func (s *PSACampaignPushQueueStore) Enqueue(ctx context.Context, p psacampaign.PushRow) error {
	diff, err := json.Marshal(p.Diff)
	if err != nil {
		return fmt.Errorf("psa_campaign_push_queue: marshal diff: %w", err)
	}
	status := p.Status
	if status == "" {
		status = psacampaign.PushPending
	}
	const q = `
		INSERT INTO psa_campaign_push_queue
			(id, psa_campaign_id, internal_campaign_id, proposed_diff, status, requested_by, approved_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, now(), now())`
	if _, err := s.db.ExecContext(ctx, q,
		p.ID, p.PSACampaignID, nullString(p.InternalCampaignID), string(diff), status,
		nullString(p.RequestedBy), nullString(p.ApprovedBy),
	); err != nil {
		return fmt.Errorf("psa_campaign_push_queue: insert: %w", err)
	}
	return nil
}

// Approve marks a pending row approved. Returns psacampaign.ErrPushNotPending
// if the row is not currently pending.
func (s *PSACampaignPushQueueStore) Approve(ctx context.Context, id, approvedBy string) error {
	const q = `
		UPDATE psa_campaign_push_queue
		   SET status = 'approved', approved_by = $2, updated_at = now()
		 WHERE id = $1 AND status = 'pending'`
	res, err := s.db.ExecContext(ctx, q, id, approvedBy)
	if err != nil {
		return fmt.Errorf("psa_campaign_push_queue: approve: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("psa_campaign_push_queue: approve rows affected: %w", err)
	}
	if n == 0 {
		return psacampaign.ErrPushNotPending
	}
	return nil
}

// Claim atomically transitions row id from approved to pushing, returning
// true if the claim succeeded.
func (s *PSACampaignPushQueueStore) Claim(ctx context.Context, id string) (bool, error) {
	const q = `
		UPDATE psa_campaign_push_queue
		   SET status = 'pushing', updated_at = now()
		 WHERE id = $1 AND status = 'approved'`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return false, fmt.Errorf("psa_campaign_push_queue: claim: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("psa_campaign_push_queue: claim rows affected: %w", err)
	}
	return n == 1, nil
}

// ListByStatus returns all rows matching the given status.
func (s *PSACampaignPushQueueStore) ListByStatus(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error) {
	const q = `
		SELECT id, psa_campaign_id, COALESCE(internal_campaign_id, ''), proposed_diff, status,
		       COALESCE(requested_by, ''), COALESCE(approved_by, '')
		  FROM psa_campaign_push_queue
		 WHERE status = $1
		 ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q, status)
	if err != nil {
		return nil, fmt.Errorf("psa_campaign_push_queue: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []psacampaign.PushRow
	for rows.Next() {
		var r psacampaign.PushRow
		var diff []byte
		if err := rows.Scan(&r.ID, &r.PSACampaignID, &r.InternalCampaignID, &diff, &r.Status, &r.RequestedBy, &r.ApprovedBy); err != nil {
			return nil, fmt.Errorf("psa_campaign_push_queue: scan: %w", err)
		}
		if err := json.Unmarshal(diff, &r.Diff); err != nil {
			return nil, fmt.Errorf("psa_campaign_push_queue: unmarshal diff: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("psa_campaign_push_queue: rows: %w", err)
	}
	return out, nil
}

// MarkResult records the outcome of a push attempt.
func (s *PSACampaignPushQueueStore) MarkResult(ctx context.Context, id string, status psacampaign.PushStatus, resultJSON, errMsg string) error {
	const q = `
		UPDATE psa_campaign_push_queue
		   SET status = $2, result_json = NULLIF($3, '')::jsonb, error = NULLIF($4, ''), updated_at = now()
		 WHERE id = $1`
	res, err := s.db.ExecContext(ctx, q, id, status, resultJSON, errMsg)
	if err != nil {
		return fmt.Errorf("psa_campaign_push_queue: mark result: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("psa_campaign_push_queue: mark result: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("psa_campaign_push_queue: MarkResult: no row with id %q", id)
	}
	return nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
