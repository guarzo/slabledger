package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SaleStore implements inventory.SaleRepository operations.
type SaleStore struct {
	base
}

// NewSaleStore creates a new Sale store.
func NewSaleStore(db *sql.DB, logger observability.Logger) *SaleStore {
	return &SaleStore{base{db: db, logger: logger}}
}

var _ inventory.SaleRepository = (*SaleStore)(nil)

func (ss *SaleStore) CreateSale(ctx context.Context, s *inventory.Sale) error {
	query := `
		INSERT INTO campaign_sales (` + saleColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := ss.db.ExecContext(ctx, query,
		s.ID, s.PurchaseID, s.SaleChannel, s.SalePriceCents,
		s.SaleFeeCents, s.SaleDate, s.DaysToSell, s.NetProfitCents,
		s.CreatedAt, s.UpdatedAt,
		s.LastSoldCents, s.LowestListCents, s.ConservativeCents, s.MedianCents,
		s.ActiveListings, s.SalesLast30d, s.Trend30d, s.SnapshotDate, s.SnapshotJSON,
		s.OriginalListPriceCents, s.PriceReductions, s.DaysListed, s.SoldAtAskingPrice,
		s.WasCracked, s.OrderID,
	)
	if err != nil && isUniqueConstraintError(err) {
		return inventory.ErrDuplicateSale
	}
	if err != nil {
		return fmt.Errorf("create sale: %w", err)
	}
	return nil
}

func (ss *SaleStore) GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*inventory.Sale, error) {
	query := `SELECT ` + saleColumns + ` FROM campaign_sales WHERE purchase_id = ?`
	s, err := scanSale(ss.db.QueryRowContext(ctx, query, purchaseID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, inventory.ErrSaleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (ss *SaleStore) GetSalesByPurchaseIDs(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Sale, error) {
	if len(purchaseIDs) == 0 {
		return map[string]*inventory.Sale{}, nil
	}

	// Chunk to stay under SQLite's parameter limit (default 999, newer 32766).
	// Matches the chunkSize used by GetPurchasesByCertNumbers / GetPurchasesByIDs.
	const chunkSize = 500
	result := make(map[string]*inventory.Sale, len(purchaseIDs))

	for start := 0; start < len(purchaseIDs); start += chunkSize {
		end := min(start+chunkSize, len(purchaseIDs))
		chunk := purchaseIDs[start:end]

		placeholders := make([]byte, 0, len(chunk)*2-1)
		args := make([]any, len(chunk))
		for i, id := range chunk {
			if i > 0 {
				placeholders = append(placeholders, ',')
			}
			placeholders = append(placeholders, '?')
			args[i] = id
		}

		query := `SELECT ` + saleColumns + ` FROM campaign_sales WHERE purchase_id IN (` + string(placeholders) + `)`
		if err := ss.scanSalesChunk(ctx, query, args, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (ss *SaleStore) scanSalesChunk(ctx context.Context, query string, args []any, into map[string]*inventory.Sale) (err error) {
	rows, err := ss.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query sales by purchase ids chunk: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()
	for rows.Next() {
		s, scanErr := scanSale(rows)
		if scanErr != nil {
			return scanErr
		}
		into[s.PurchaseID] = &s
	}
	return rows.Err()
}

func (ss *SaleStore) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error) {
	query := `
		SELECT ` + saleColumns + `
		FROM campaign_sales
		WHERE purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = ?)
		ORDER BY sale_date DESC
		LIMIT ? OFFSET ?
	`
	rows, err := ss.db.QueryContext(ctx, query, campaignID, limit, offset)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Sale, error) {
		return scanSale(rs)
	})
}

func (ss *SaleStore) DeleteSale(ctx context.Context, saleID string) error {
	result, err := ss.db.ExecContext(ctx, `DELETE FROM campaign_sales WHERE id = ?`, saleID)
	if err != nil {
		return fmt.Errorf("delete sale: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return inventory.ErrSaleNotFound
	}
	return nil
}

func (ss *SaleStore) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	result, err := ss.db.ExecContext(ctx, `DELETE FROM campaign_sales WHERE purchase_id = ?`, purchaseID)
	if err != nil {
		return fmt.Errorf("delete sale by purchase id: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return inventory.ErrSaleNotFound
	}
	return nil
}
