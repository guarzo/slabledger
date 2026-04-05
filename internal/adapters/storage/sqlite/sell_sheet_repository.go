package sqlite

import (
	"context"
	"fmt"
	"strings"
)

// GetSellSheetItems returns all purchase IDs on the user's sell sheet.
func (r *CampaignsRepository) GetSellSheetItems(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT purchase_id FROM sell_sheet_items WHERE user_id = ? ORDER BY added_at`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("get sell sheet items: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan sell sheet item: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AddSellSheetItems adds purchase IDs to the user's sell sheet (idempotent).
func (r *CampaignsRepository) AddSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO sell_sheet_items (user_id, purchase_id) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare add sell sheet: %w", err)
	}
	defer stmt.Close() //nolint:errcheck

	for _, id := range purchaseIDs {
		if _, err := stmt.ExecContext(ctx, userID, id); err != nil {
			return fmt.Errorf("add sell sheet item %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// RemoveSellSheetItems removes specific purchase IDs from the user's sell sheet.
func (r *CampaignsRepository) RemoveSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(purchaseIDs))
	args := make([]any, 0, len(purchaseIDs)+1)
	args = append(args, userID)
	for i, id := range purchaseIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`DELETE FROM sell_sheet_items WHERE user_id = ? AND purchase_id IN (%s)`,
		strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("remove sell sheet items: %w", err)
	}
	return nil
}

// ClearSellSheet removes all items from the user's sell sheet.
func (r *CampaignsRepository) ClearSellSheet(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM sell_sheet_items WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("clear sell sheet: %w", err)
	}
	return nil
}
