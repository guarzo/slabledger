package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func (ps *PurchaseStore) ListSnapshotPurchasesByStatus(ctx context.Context, status inventory.SnapshotStatus, limit int) ([]inventory.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE snapshot_status = $1 ORDER BY updated_at ASC LIMIT $2`
	rows, err := ps.db.QueryContext(ctx, query, string(status), limit)
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
		`UPDATE campaign_purchases SET snapshot_status = $1, snapshot_retry_count = $2, updated_at = $3 WHERE id = $4`,
		string(status), retryCount, time.Now(), id,
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
		`UPDATE campaign_purchases SET psa_ship_date = $1, invoice_date = $2, was_refunded = $3,
			front_image_url = $4, back_image_url = $5, purchase_source = $6, psa_listing_title = $7, updated_at = $8
		WHERE id = $9`,
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
		 LIMIT $1`, limit)
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
		query = `UPDATE campaign_purchases SET front_image_url = $1, back_image_url = $2, updated_at = $3 WHERE id = $4`
		args = []any{frontURL, backURL, now, id}
	case frontURL != "":
		query = `UPDATE campaign_purchases SET front_image_url = $1, updated_at = $2 WHERE id = $3`
		args = []any{frontURL, now, id}
	case backURL != "":
		query = `UPDATE campaign_purchases SET back_image_url = $1, updated_at = $2 WHERE id = $3`
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
