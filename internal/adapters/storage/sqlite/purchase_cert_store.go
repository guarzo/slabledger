package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// GetPurchasesByGraderAndCertNumbers retrieves multiple purchases by grader and cert numbers.
// Large inputs are chunked to stay within SQLite's parameter limit.
// Returns a map keyed by cert number.
func (ps *PurchaseStore) GetPurchasesByGraderAndCertNumbers(ctx context.Context, grader string, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if len(certNumbers) == 0 {
		return make(map[string]*inventory.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*inventory.Purchase, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)+1)
		args = append(args, grader)
		for i, cn := range chunk {
			placeholders[i] = "?"
			args = append(args, cn)
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases
			WHERE grader = ? AND cert_number IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchases by grader/cert chunk: %w", err)
		}

		purchases, err := scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
			var p inventory.Purchase
			err := scanPurchase(rs, &p)
			return p, err
		})
		if err != nil {
			return nil, fmt.Errorf("scan purchases by grader/cert chunk: %w", err)
		}
		for i := range purchases {
			result[purchases[i].CertNumber] = &purchases[i]
		}
	}
	return result, nil
}

// GetPurchasesByCertNumbers retrieves purchases by cert numbers across all graders.
// Large inputs are chunked to stay within SQLite's parameter limit.
// If the same cert number exists under multiple graders, the last scanned row wins;
// use GetPurchasesByGraderAndCertNumbers when grader context is available.
func (ps *PurchaseStore) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if len(certNumbers) == 0 {
		return make(map[string]*inventory.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*inventory.Purchase, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, cn := range chunk {
			placeholders[i] = "?"
			args[i] = cn
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases
			WHERE cert_number IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchases by cert chunk: %w", err)
		}

		purchases, err := scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
			var p inventory.Purchase
			err := scanPurchase(rs, &p)
			return p, err
		})
		if err != nil {
			return nil, fmt.Errorf("scan purchases by cert chunk: %w", err)
		}
		for i := range purchases {
			result[purchases[i].CertNumber] = &purchases[i]
		}
	}

	return result, nil
}

// GetPurchasesByIDs retrieves multiple purchases by their IDs in a single query.
// Large inputs are chunked to stay within SQLite's parameter limit.
func (ps *PurchaseStore) GetPurchasesByIDs(ctx context.Context, ids []string) (map[string]*inventory.Purchase, error) {
	if len(ids) == 0 {
		return make(map[string]*inventory.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*inventory.Purchase, len(ids))

	for start := 0; start < len(ids); start += chunkSize {
		end := min(start+chunkSize, len(ids))
		chunk := ids[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE id IN (` + strings.Join(placeholders, ",") + `)`
		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchases by IDs chunk: %w", err)
		}

		purchases, err := scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
			var p inventory.Purchase
			err := scanPurchase(rs, &p)
			return p, err
		})
		if err != nil {
			return nil, fmt.Errorf("scan purchases by IDs chunk: %w", err)
		}
		for i := range purchases {
			result[purchases[i].ID] = &purchases[i]
		}
	}

	return result, nil
}

// SetReceivedAt records when a purchase was received from grading.
func (ps *PurchaseStore) SetReceivedAt(ctx context.Context, purchaseID string, receivedAt time.Time) error {
	return ps.execAndExpectRow(ctx, "set received_at",
		`UPDATE campaign_purchases SET received_at = ?, updated_at = ? WHERE id = ?`,
		receivedAt, time.Now().UTC(), purchaseID,
	)
}

// GetPurchaseIDByCertNumber returns the purchase ID for a given cert number.
// If multiple purchases share the cert number under different graders, an arbitrary
// row is returned; callers with grader context should use GetPurchasesByGraderAndCertNumbers.
func (ps *PurchaseStore) GetPurchaseIDByCertNumber(ctx context.Context, certNumber string) (string, error) {
	var id string
	err := ps.db.QueryRowContext(ctx,
		`SELECT id FROM campaign_purchases WHERE cert_number = ?`, certNumber,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// GetPurchaseIDsByCertNumbers returns a cert→purchaseID map for the given certs.
// Certs with no matching purchase are simply absent from the map. Inputs are
// chunked to stay within SQLite's parameter limit. If a cert exists under
// multiple graders, an arbitrary row's ID is returned (matching the single-cert
// variant's behavior).
func (ps *PurchaseStore) GetPurchaseIDsByCertNumbers(ctx context.Context, certNumbers []string) (map[string]string, error) {
	if len(certNumbers) == 0 {
		return make(map[string]string), nil
	}

	const chunkSize = 500
	result := make(map[string]string, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, cn := range chunk {
			placeholders[i] = "?"
			args[i] = cn
		}

		query := `SELECT id, cert_number FROM campaign_purchases WHERE cert_number IN (` + strings.Join(placeholders, ",") + `)`
		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchase IDs by cert chunk: %w", err)
		}
		for rows.Next() {
			var id, cert string
			if err := rows.Scan(&id, &cert); err != nil {
				_ = rows.Close() //nolint:errcheck
				return nil, fmt.Errorf("scan purchase IDs by cert chunk: %w", err)
			}
			result[cert] = id
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close() //nolint:errcheck
			return nil, fmt.Errorf("rows err in purchase IDs by cert chunk: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close rows in purchase IDs by cert chunk: %w", err)
		}
	}

	return result, nil
}

// DeletePurchase removes a purchase and its associated sales within a transaction.
func (ps *PurchaseStore) DeletePurchase(ctx context.Context, id string) (retErr error) {
	tx, err := ps.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback() //nolint:errcheck // best-effort; error logged via retErr
		}
	}()

	// Delete any sales associated with this purchase
	if _, retErr = tx.ExecContext(ctx,
		`DELETE FROM campaign_sales WHERE purchase_id = ?`, id,
	); retErr != nil {
		return retErr
	}

	// Delete the purchase
	result, err := tx.ExecContext(ctx, `DELETE FROM campaign_purchases WHERE id = ?`, id)
	if err != nil {
		retErr = fmt.Errorf("delete purchase: %w", err)
		return retErr
	}
	n, err := result.RowsAffected()
	if err != nil {
		retErr = fmt.Errorf("check rows affected: %w", err)
		return retErr
	}
	if n == 0 {
		retErr = inventory.ErrPurchaseNotFound
		return retErr
	}

	return tx.Commit()
}
