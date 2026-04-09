package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// PendingItemsRepository implements campaigns.PendingItemRepository using SQLite.
type PendingItemsRepository struct {
	db *sql.DB
}

// NewPendingItemsRepository creates a new SQLite pending items repository.
func NewPendingItemsRepository(db *sql.DB) *PendingItemsRepository {
	return &PendingItemsRepository{db: db}
}

var _ campaigns.PendingItemRepository = (*PendingItemsRepository)(nil)

// SavePendingItems upserts pending items by cert_number.
// Resolved items (resolved_at IS NOT NULL) are skipped.
func (r *PendingItemsRepository) SavePendingItems(ctx context.Context, items []campaigns.PendingItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO psa_pending_items (id, cert_number, card_name, set_name, card_number, grade, buy_cost_cents, purchase_date, status, candidates, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cert_number) DO UPDATE SET
			status = excluded.status,
			candidates = excluded.candidates,
			buy_cost_cents = excluded.buy_cost_cents,
			card_name = excluded.card_name,
			set_name = excluded.set_name,
			card_number = excluded.card_number,
			grade = excluded.grade,
			purchase_date = excluded.purchase_date,
			source = excluded.source
		WHERE resolved_at IS NULL
	`)
	if err != nil {
		return err
	}
	defer stmt.Close() //nolint:errcheck

	for _, item := range items {
		candidatesJSON, err := json.Marshal(item.Candidates)
		if err != nil {
			candidatesJSON = []byte("[]")
		}
		if _, err := stmt.ExecContext(ctx,
			item.ID, item.CertNumber, item.CardName, item.SetName,
			item.CardNumber, item.Grade, item.BuyCostCents, item.PurchaseDate,
			item.Status, string(candidatesJSON), item.Source,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListPendingItems returns all unresolved pending items, ordered by created_at DESC.
func (r *PendingItemsRepository) ListPendingItems(ctx context.Context) ([]campaigns.PendingItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, cert_number, card_name, set_name, card_number, grade,
		       buy_cost_cents, purchase_date, status, candidates, source, created_at
		FROM psa_pending_items
		WHERE resolved_at IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var items []campaigns.PendingItem
	for rows.Next() {
		var item campaigns.PendingItem
		var candidatesJSON string
		if err := rows.Scan(
			&item.ID, &item.CertNumber, &item.CardName, &item.SetName,
			&item.CardNumber, &item.Grade, &item.BuyCostCents, &item.PurchaseDate,
			&item.Status, &candidatesJSON, &item.Source, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(candidatesJSON), &item.Candidates); err != nil {
			item.Candidates = nil
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ResolvePendingItem marks a pending item as resolved with the given campaign ID.
func (r *PendingItemsRepository) ResolvePendingItem(ctx context.Context, id string, campaignID string) error {
	return r.resolvePendingItem(ctx, id, campaignID)
}

// DismissPendingItem marks a pending item as dismissed (resolved with empty campaign ID).
func (r *PendingItemsRepository) DismissPendingItem(ctx context.Context, id string) error {
	return r.resolvePendingItem(ctx, id, "")
}

func (r *PendingItemsRepository) resolvePendingItem(ctx context.Context, id string, campaignID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE psa_pending_items
		SET resolved_at = ?, resolved_campaign_id = ?
		WHERE id = ? AND resolved_at IS NULL
	`, time.Now().UTC(), campaignID, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrPendingItemNotFound
	}
	return nil
}

// CountPendingItems returns the number of unresolved pending items.
func (r *PendingItemsRepository) CountPendingItems(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM psa_pending_items WHERE resolved_at IS NULL
	`).Scan(&count)
	return count, err
}
