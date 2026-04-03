package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// saleColumns is the canonical column list for campaign_sales queries.
const saleColumns = `id, purchase_id, sale_channel, sale_price_cents, sale_fee_cents,
	sale_date, days_to_sell, net_profit_cents, created_at, updated_at,
	last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
	active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
	original_list_price_cents, price_reductions, days_listed, sold_at_asking_price,
	was_cracked, order_id`

// scanSale scans a single Sale row matching saleColumns order.
func scanSale(scanner interface{ Scan(dest ...any) error }) (campaigns.Sale, error) {
	var s campaigns.Sale
	err := scanner.Scan(
		&s.ID, &s.PurchaseID, &s.SaleChannel, &s.SalePriceCents, &s.SaleFeeCents,
		&s.SaleDate, &s.DaysToSell, &s.NetProfitCents, &s.CreatedAt, &s.UpdatedAt,
		&s.LastSoldCents, &s.LowestListCents, &s.ConservativeCents, &s.MedianCents,
		&s.ActiveListings, &s.SalesLast30d, &s.Trend30d, &s.SnapshotDate, &s.SnapshotJSON,
		&s.OriginalListPriceCents, &s.PriceReductions, &s.DaysListed, &s.SoldAtAskingPrice,
		&s.WasCracked, &s.OrderID,
	)
	return s, err
}

// --- Sale CRUD ---

func (r *CampaignsRepository) CreateSale(ctx context.Context, s *campaigns.Sale) error {
	query := `
		INSERT INTO campaign_sales (` + saleColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		s.ID, s.PurchaseID, s.SaleChannel, s.SalePriceCents,
		s.SaleFeeCents, s.SaleDate, s.DaysToSell, s.NetProfitCents,
		s.CreatedAt, s.UpdatedAt,
		s.LastSoldCents, s.LowestListCents, s.ConservativeCents, s.MedianCents,
		s.ActiveListings, s.SalesLast30d, s.Trend30d, s.SnapshotDate, s.SnapshotJSON,
		s.OriginalListPriceCents, s.PriceReductions, s.DaysListed, s.SoldAtAskingPrice,
		s.WasCracked, s.OrderID,
	)
	if err != nil && isUniqueConstraintError(err) {
		return campaigns.ErrDuplicateSale
	}
	return err
}

func (r *CampaignsRepository) GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*campaigns.Sale, error) {
	query := `SELECT ` + saleColumns + ` FROM campaign_sales WHERE purchase_id = ?`
	s, err := scanSale(r.db.QueryRowContext(ctx, query, purchaseID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, campaigns.ErrSaleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *CampaignsRepository) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Sale, error) {
	query := `
		SELECT ` + saleColumns + `
		FROM campaign_sales
		WHERE purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = ?)
		ORDER BY sale_date DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, campaignID, limit, offset)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (campaigns.Sale, error) {
		return scanSale(rs)
	})
}
