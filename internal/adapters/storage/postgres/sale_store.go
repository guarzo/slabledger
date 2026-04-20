package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
	`
	_, err := ss.db.ExecContext(ctx, query,
		s.ID, s.PurchaseID, string(s.SaleChannel), s.SalePriceCents,
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
	query := `SELECT ` + saleColumns + ` FROM campaign_sales WHERE purchase_id = $1`
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

	// Chunk to stay under Postgres's parameter limit.
	// Matches the chunkSize used by GetPurchasesByCertNumbers / GetPurchasesByIDs.
	const chunkSize = 500
	result := make(map[string]*inventory.Sale, len(purchaseIDs))

	for start := 0; start < len(purchaseIDs); start += chunkSize {
		end := min(start+chunkSize, len(purchaseIDs))
		chunk := purchaseIDs[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}

		query := `SELECT ` + saleColumns + ` FROM campaign_sales WHERE purchase_id IN (` + strings.Join(placeholders, ",") + `)`
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
			err = fmt.Errorf("close sales rows in purchase ids chunk: %w", cerr)
		}
	}()
	for rows.Next() {
		s, scanErr := scanSale(rows)
		if scanErr != nil {
			return fmt.Errorf("scan sale row in purchase ids chunk: %w", scanErr)
		}
		into[s.PurchaseID] = &s
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate sales rows in purchase ids chunk: %w", err)
	}
	return nil
}

func (ss *SaleStore) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error) {
	query := `
		SELECT ` + saleColumns + `
		FROM campaign_sales
		WHERE purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = $1)
		ORDER BY sale_date DESC
		LIMIT $2 OFFSET $3
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
	result, err := ss.db.ExecContext(ctx, `DELETE FROM campaign_sales WHERE id = $1`, saleID)
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
	result, err := ss.db.ExecContext(ctx, `DELETE FROM campaign_sales WHERE purchase_id = $1`, purchaseID)
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
