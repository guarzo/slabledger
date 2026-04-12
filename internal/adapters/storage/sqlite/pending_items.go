package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// PendingItemsRepository implements inventory.PendingItemRepository using SQLite.
type PendingItemsRepository struct {
	db *sql.DB
}

// NewPendingItemsRepository creates a new SQLite pending items repository.
func NewPendingItemsRepository(db *sql.DB) *PendingItemsRepository {
	return &PendingItemsRepository{db: db}
}

var _ inventory.PendingItemRepository = (*PendingItemsRepository)(nil)

// SavePendingItems upserts pending items by cert_number.
// If an unresolved row already exists for the cert, it is updated in place.
// Resolved items (resolved_at IS NOT NULL) are left untouched.
func (r *PendingItemsRepository) SavePendingItems(ctx context.Context, items []inventory.PendingItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	updateStmt, err := tx.PrepareContext(ctx, `
		UPDATE psa_pending_items SET
			status = ?, candidates = ?, buy_cost_cents = ?,
			card_name = ?, set_name = ?, card_number = ?,
			grade = ?, purchase_date = ?, source = ?
		WHERE cert_number = ? AND resolved_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("prepare update pending items: %w", err)
	}
	defer updateStmt.Close() //nolint:errcheck

	insertStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO psa_pending_items (id, cert_number, card_name, set_name, card_number, grade, buy_cost_cents, purchase_date, status, candidates, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert pending items: %w", err)
	}
	defer insertStmt.Close() //nolint:errcheck

	for _, item := range items {
		candidatesJSON, err := json.Marshal(item.Candidates)
		if err != nil {
			candidatesJSON = []byte("[]")
		}

		// Try to update an existing unresolved row first.
		res, err := updateStmt.ExecContext(ctx,
			item.Status, string(candidatesJSON), item.BuyCostCents,
			item.CardName, item.SetName, item.CardNumber,
			item.Grade, item.PurchaseDate, item.Source,
			item.CertNumber,
		)
		if err != nil {
			return fmt.Errorf("update pending item: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("check rows affected: %w", err)
		}
		if n > 0 {
			continue // updated existing unresolved row
		}

		// No unresolved row exists — insert a new one.
		if _, err := insertStmt.ExecContext(ctx,
			item.ID, item.CertNumber, item.CardName, item.SetName,
			item.CardNumber, item.Grade, item.BuyCostCents, item.PurchaseDate,
			item.Status, string(candidatesJSON), item.Source,
		); err != nil {
			return fmt.Errorf("insert pending item: %w", err)
		}
	}
	return tx.Commit()
}

// ListPendingItems returns all unresolved pending items, ordered by created_at DESC.
func (r *PendingItemsRepository) ListPendingItems(ctx context.Context) ([]inventory.PendingItem, error) {
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

	var items []inventory.PendingItem
	for rows.Next() {
		var item inventory.PendingItem
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
	if items == nil {
		items = []inventory.PendingItem{}
	}
	return items, rows.Err()
}

// GetPendingItemByID returns a single unresolved pending item by ID.
func (r *PendingItemsRepository) GetPendingItemByID(ctx context.Context, id string) (*inventory.PendingItem, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, cert_number, card_name, set_name, card_number, grade,
		       buy_cost_cents, purchase_date, status, candidates, source, created_at
		FROM psa_pending_items
		WHERE id = ? AND resolved_at IS NULL
	`, id)

	var item inventory.PendingItem
	var candidatesJSON string
	if err := row.Scan(
		&item.ID, &item.CertNumber, &item.CardName, &item.SetName,
		&item.CardNumber, &item.Grade, &item.BuyCostCents, &item.PurchaseDate,
		&item.Status, &candidatesJSON, &item.Source, &item.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, inventory.ErrPendingItemNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(candidatesJSON), &item.Candidates); err != nil {
		item.Candidates = nil
	}
	return &item, nil
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
		return fmt.Errorf("update pending item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrPendingItemNotFound
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
