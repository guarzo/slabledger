package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"time"
)

func (ps *PurchaseStore) ListSnapshotPurchasesByStatus(ctx context.Context, status inventory.SnapshotStatus, limit int) ([]inventory.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE snapshot_status = ? ORDER BY updated_at ASC LIMIT ?`
	rows, err := ps.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, fmt.Errorf("query purchases by snapshot status %q: %w", status, err)
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

func (ps *PurchaseStore) UpdatePurchaseSnapshotStatus(ctx context.Context, id string, status inventory.SnapshotStatus, retryCount int) error {
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET snapshot_status = ?, snapshot_retry_count = ?, updated_at = ? WHERE id = ?`,
		status, retryCount, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update snapshot status for purchase %s: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected on update snapshot status: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

func (ps *PurchaseStore) UpdatePurchasePSAFields(ctx context.Context, id string, fields inventory.PSAUpdateFields) error {
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET psa_ship_date = ?, invoice_date = ?, was_refunded = ?,
			front_image_url = ?, back_image_url = ?, purchase_source = ?, psa_listing_title = ?, updated_at = ?
		WHERE id = ?`,
		fields.PSAShipDate, fields.InvoiceDate, fields.WasRefunded,
		fields.FrontImageURL, fields.BackImageURL, fields.PurchaseSource, fields.PSAListingTitle,
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update PSA fields for purchase %s: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected on update PSA fields: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

// ListPurchasesMissingImages returns purchases with empty front_image_url that have cert numbers.
func (ps *PurchaseStore) ListPurchasesMissingImages(ctx context.Context, limit int) ([]struct {
	ID         string
	CertNumber string
}, error) {
	rows, err := ps.db.QueryContext(ctx,
		`SELECT id, cert_number FROM campaign_purchases
		 WHERE (front_image_url = '' OR front_image_url IS NULL)
		 AND cert_number <> ''
		 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query purchases missing images: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []struct {
		ID         string
		CertNumber string
	}
	for rows.Next() {
		var row struct {
			ID         string
			CertNumber string
		}
		if err := rows.Scan(&row.ID, &row.CertNumber); err != nil {
			return nil, fmt.Errorf("scan purchase missing images row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// UpdatePurchaseImageURLs updates the front and back image URLs for a purchase.
// Empty values are treated as no-ops — only non-empty fields are written.
func (ps *PurchaseStore) UpdatePurchaseImageURLs(ctx context.Context, id, frontURL, backURL string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var query string
	var args []any

	switch {
	case frontURL != "" && backURL != "":
		query = `UPDATE campaign_purchases SET front_image_url = ?, back_image_url = ?, updated_at = ? WHERE id = ?`
		args = []any{frontURL, backURL, now, id}
	case frontURL != "":
		query = `UPDATE campaign_purchases SET front_image_url = ?, updated_at = ? WHERE id = ?`
		args = []any{frontURL, now, id}
	case backURL != "":
		query = `UPDATE campaign_purchases SET back_image_url = ?, updated_at = ? WHERE id = ?`
		args = []any{backURL, now, id}
	default:
		return nil // nothing to update
	}

	res, err := ps.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update image URLs for purchase %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}
