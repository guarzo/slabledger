package sqlite

import (
	"context"
	"database/sql"
	"time"
)

// CLSaleCompRecord represents a stored sales comp.
type CLSaleCompRecord struct {
	GemRateID   string
	ItemID      string
	SaleDate    string
	PriceCents  int
	Platform    string
	ListingType string
	Seller      string
	ItemURL     string
	SlabSerial  string
}

// CLSalesStore manages Card Ladder sales comp persistence.
type CLSalesStore struct {
	db *sql.DB
}

// NewCLSalesStore creates a new sales comp store.
func NewCLSalesStore(db *sql.DB) *CLSalesStore {
	return &CLSalesStore{db: db}
}

// UpsertSaleComp inserts or updates a sale comp record.
func (s *CLSalesStore) UpsertSaleComp(ctx context.Context, rec CLSaleCompRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cl_sales_comps (gem_rate_id, item_id, sale_date, price_cents, platform, listing_type, seller, item_url, slab_serial, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(gem_rate_id, item_id) DO UPDATE SET
		   price_cents = excluded.price_cents,
		   sale_date = excluded.sale_date,
		   platform = excluded.platform,
		   listing_type = excluded.listing_type,
		   seller = excluded.seller,
		   item_url = excluded.item_url,
		   slab_serial = excluded.slab_serial`,
		rec.GemRateID, rec.ItemID, rec.SaleDate, rec.PriceCents,
		rec.Platform, rec.ListingType, rec.Seller, rec.ItemURL, rec.SlabSerial,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// GetSaleComps returns recent sales for a gemRateID, ordered by date descending.
func (s *CLSalesStore) GetSaleComps(ctx context.Context, gemRateID string, limit int) ([]CLSaleCompRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT gem_rate_id, item_id, sale_date, price_cents, platform, listing_type, seller, item_url, slab_serial
		 FROM cl_sales_comps WHERE gem_rate_id = ? ORDER BY sale_date DESC LIMIT ?`,
		gemRateID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var comps []CLSaleCompRecord
	for rows.Next() {
		var c CLSaleCompRecord
		if err := rows.Scan(&c.GemRateID, &c.ItemID, &c.SaleDate, &c.PriceCents,
			&c.Platform, &c.ListingType, &c.Seller, &c.ItemURL, &c.SlabSerial); err != nil {
			return nil, err
		}
		comps = append(comps, c)
	}
	return comps, rows.Err()
}

// GetLatestSaleDate returns the most recent sale date for a gemRateID, or empty string if none.
func (s *CLSalesStore) GetLatestSaleDate(ctx context.Context, gemRateID string) (string, error) {
	var date sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(sale_date) FROM cl_sales_comps WHERE gem_rate_id = ?`, gemRateID,
	).Scan(&date)
	if err != nil {
		return "", err
	}
	return date.String, nil
}
