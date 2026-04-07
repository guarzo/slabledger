package sqlite

import (
	"context"
	"fmt"
	"strings"
)

// GetSellSheetItems returns all purchase IDs on the global sell sheet.
func (r *CampaignsRepository) GetSellSheetItems(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT purchase_id FROM sell_sheet_items ORDER BY added_at`)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sell sheet items: %w", err)
	}
	return ids, nil
}

// AddSellSheetItems adds purchase IDs to the global sell sheet (idempotent).
func (r *CampaignsRepository) AddSellSheetItems(ctx context.Context, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO sell_sheet_items (purchase_id) VALUES (?)`)
	if err != nil {
		return fmt.Errorf("prepare add sell sheet: %w", err)
	}
	defer stmt.Close() //nolint:errcheck

	for _, id := range purchaseIDs {
		if _, err := stmt.ExecContext(ctx, id); err != nil {
			return fmt.Errorf("add sell sheet item %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// RemoveSellSheetItems removes specific purchase IDs from the global sell sheet.
func (r *CampaignsRepository) RemoveSellSheetItems(ctx context.Context, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(purchaseIDs))
	args := make([]any, 0, len(purchaseIDs))
	for i, id := range purchaseIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`DELETE FROM sell_sheet_items WHERE purchase_id IN (%s)`,
		strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("remove sell sheet items: %w", err)
	}
	return nil
}

// ClearSellSheet removes all items from the global sell sheet.
func (r *CampaignsRepository) ClearSellSheet(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sell_sheet_items`)
	if err != nil {
		return fmt.Errorf("clear sell sheet: %w", err)
	}
	return nil
}
