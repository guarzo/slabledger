package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// MMSaleCompRecord represents a stored sales comp from MarketMovers.
type MMSaleCompRecord struct {
	MMCollectibleID int64
	SaleID          int64
	SaleDate        string
	PriceCents      int
	Platform        string
	ListingType     string
	Seller          string
	SaleURL         string
}

// MMSalesStore manages MarketMovers sales comp persistence and implements CompSummaryProvider.
type MMSalesStore struct {
	db *sql.DB
}

// NewMMSalesStore creates a new MM sales comp store.
func NewMMSalesStore(db *sql.DB) *MMSalesStore {
	return &MMSalesStore{db: db}
}

// UpsertSaleComp inserts or updates an MM sale comp record.
func (s *MMSalesStore) UpsertSaleComp(ctx context.Context, rec MMSaleCompRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mm_sales_comps (mm_collectible_id, sale_id, sale_date, price_cents, platform, listing_type, seller, sale_url, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT(mm_collectible_id, sale_id) DO UPDATE SET
		   price_cents = excluded.price_cents,
		   sale_date = excluded.sale_date,
		   platform = excluded.platform,
		   listing_type = excluded.listing_type,
		   seller = excluded.seller,
		   sale_url = excluded.sale_url`,
		rec.MMCollectibleID, rec.SaleID, rec.SaleDate, rec.PriceCents,
		rec.Platform, rec.ListingType, rec.Seller, rec.SaleURL,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert MM sale comp: %w", err)
	}
	return nil
}

// lookupCollectibleID resolves MM collectible ID from mm_card_mappings by cert number.
func (s *MMSalesStore) lookupCollectibleID(ctx context.Context, certNumber string) (int64, error) {
	if certNumber == "" {
		return 0, nil
	}
	var id sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT mm_collectible_id FROM mm_card_mappings WHERE slab_serial = $1`, certNumber,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return id.Int64, nil
}

// lookupCollectibleIDBatch resolves MM collectible IDs for a batch of cert numbers.
func (s *MMSalesStore) lookupCollectibleIDBatch(ctx context.Context, certs []string) (map[string]int64, error) {
	out := make(map[string]int64, len(certs))
	if len(certs) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT slab_serial, mm_collectible_id FROM mm_card_mappings WHERE slab_serial = ANY($1::text[])`,
		certs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cert string
		var id int64
		if err := rows.Scan(&cert, &id); err != nil {
			return nil, err
		}
		if id > 0 {
			out[cert] = id
		}
	}
	return out, rows.Err()
}

// GetCompSummary returns comp analytics for one cert from MM data.
func (s *MMSalesStore) GetCompSummary(ctx context.Context, _ string, certNumber string) (*inventory.CompSummary, error) {
	cid, err := s.lookupCollectibleID(ctx, certNumber)
	if err != nil {
		return nil, fmt.Errorf("lookup MM collectible for cert %s: %w", certNumber, err)
	}
	if cid == 0 {
		return nil, nil
	}
	return s.compSummaryForCollectible(ctx, cid)
}

// GetCompSummariesByKeys is the batch form.
func (s *MMSalesStore) GetCompSummariesByKeys(ctx context.Context, keys []inventory.CompKey) (map[inventory.CompKey]*inventory.CompSummary, error) {
	out := make(map[inventory.CompKey]*inventory.CompSummary)
	if len(keys) == 0 {
		return out, nil
	}

	certs := make([]string, 0, len(keys))
	for _, k := range keys {
		if k.CertNumber != "" {
			certs = append(certs, k.CertNumber)
		}
	}
	certToCollectible, err := s.lookupCollectibleIDBatch(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("batch lookup MM collectibles: %w", err)
	}

	// Group keys by collectible ID to avoid duplicate queries.
	cidToKeys := make(map[int64][]inventory.CompKey)
	var cids []int64
	for _, k := range keys {
		cid, ok := certToCollectible[k.CertNumber]
		if !ok || cid == 0 {
			continue
		}
		if _, seen := cidToKeys[cid]; !seen {
			cids = append(cids, cid)
		}
		cidToKeys[cid] = append(cidToKeys[cid], k)
	}
	if len(cids) == 0 {
		return out, nil
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -90).Format("2006-01-02")
	midCutoff := now.AddDate(0, 0, -45).Format("2006-01-02")

	// Batch aggregate query.
	aggRows, err := s.db.QueryContext(ctx,
		`SELECT mm_collectible_id,
			COUNT(*) AS total_comps,
			COUNT(CASE WHEN sale_date >= $2 THEN 1 END) AS recent_comps,
			MAX(CASE WHEN sale_date >= $2 THEN price_cents END) AS high_cents,
			MIN(CASE WHEN sale_date >= $2 THEN price_cents END) AS low_cents,
			MAX(sale_date) AS last_sale_date
		FROM mm_sales_comps
		WHERE mm_collectible_id = ANY($1::bigint[])
		GROUP BY mm_collectible_id`,
		cids, cutoff)
	if err != nil {
		return nil, fmt.Errorf("MM aggregate: %w", err)
	}
	type aggResult struct {
		totalComps   int
		recentComps  int
		highCents    int
		lowCents     int
		lastSaleDate string
	}
	aggs := make(map[int64]*aggResult)
	for aggRows.Next() {
		var cid int64
		var a aggResult
		var highCents, lowCents sql.NullInt64
		var lastSaleDate sql.NullString
		if err := aggRows.Scan(&cid, &a.totalComps, &a.recentComps, &highCents, &lowCents, &lastSaleDate); err != nil {
			_ = aggRows.Close()
			return nil, fmt.Errorf("scan MM aggregate: %w", err)
		}
		if a.totalComps == 0 || a.recentComps == 0 {
			continue
		}
		a.highCents = int(highCents.Int64)
		a.lowCents = int(lowCents.Int64)
		a.lastSaleDate = lastSaleDate.String
		aggs[cid] = &a
	}
	if err := aggRows.Err(); err != nil {
		_ = aggRows.Close()
		return nil, fmt.Errorf("iterate MM aggregate: %w", err)
	}
	_ = aggRows.Close()

	if len(aggs) == 0 {
		return out, nil
	}

	// Fetch recent prices for median/trend/platform.
	activeCIDs := make([]int64, 0, len(aggs))
	for cid := range aggs {
		activeCIDs = append(activeCIDs, cid)
	}

	priceRows, err := s.db.QueryContext(ctx,
		`SELECT mm_collectible_id, price_cents, sale_date, platform
		FROM mm_sales_comps
		WHERE mm_collectible_id = ANY($1::bigint[]) AND sale_date >= $2
		ORDER BY mm_collectible_id, sale_date`,
		activeCIDs, cutoff)
	if err != nil {
		return nil, fmt.Errorf("MM recent prices: %w", err)
	}
	byCollectible := make(map[int64][]batchPriceRow)
	for priceRows.Next() {
		var cid int64
		var pr batchPriceRow
		if err := priceRows.Scan(&cid, &pr.priceCents, &pr.saleDate, &pr.platform); err != nil {
			_ = priceRows.Close()
			return nil, fmt.Errorf("scan MM prices: %w", err)
		}
		byCollectible[cid] = append(byCollectible[cid], pr)
	}
	if err := priceRows.Err(); err != nil {
		_ = priceRows.Close()
		return nil, fmt.Errorf("iterate MM prices: %w", err)
	}
	_ = priceRows.Close()

	// Compute summaries and fan out to keys.
	for cid, agg := range aggs {
		prs := byCollectible[cid]
		if len(prs) == 0 {
			continue
		}
		prices := make([]int, len(prs))
		dates := make([]string, len(prs))
		for i, pr := range prs {
			prices[i] = pr.priceCents
			dates[i] = pr.saleDate
		}
		sum := &inventory.CompSummary{
			TotalComps:     agg.totalComps,
			RecentComps:    agg.recentComps,
			HighestCents:   agg.highCents,
			LowestCents:    agg.lowCents,
			LastSaleDate:   agg.lastSaleDate,
			MedianCents:    medianInt(slices.Clone(prices)),
			Trend90d:       computeTrend(prices, dates, midCutoff),
			ByPlatform:     platformBreakdownFromRows(prs),
			LastSaleCents:  prs[len(prs)-1].priceCents,
			PriceCentsList: prices,
		}
		for _, k := range cidToKeys[cid] {
			cs := *sum
			out[k] = &cs
		}
	}
	return out, nil
}

// compSummaryForCollectible builds a CompSummary for a single MM collectible ID.
func (s *MMSalesStore) compSummaryForCollectible(ctx context.Context, cid int64) (*inventory.CompSummary, error) {
	now := time.Now()
	cutoff := now.AddDate(0, 0, -90).Format("2006-01-02")

	var totalComps, recentComps int
	var highCents, lowCents sql.NullInt64
	var lastSaleDate sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*) AS total_comps,
			COUNT(CASE WHEN sale_date >= $1 THEN 1 END) AS recent_comps,
			MAX(CASE WHEN sale_date >= $1 THEN price_cents END) AS high_cents,
			MIN(CASE WHEN sale_date >= $1 THEN price_cents END) AS low_cents,
			MAX(sale_date) AS last_sale_date
		FROM mm_sales_comps WHERE mm_collectible_id = $2`,
		cutoff, cid,
	).Scan(&totalComps, &recentComps, &highCents, &lowCents, &lastSaleDate)
	if err != nil {
		return nil, err
	}
	if totalComps == 0 || recentComps == 0 {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT price_cents, sale_date, platform FROM mm_sales_comps
		 WHERE mm_collectible_id = $1 AND sale_date >= $2
		 ORDER BY sale_date ASC`,
		cid, cutoff)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var prs []batchPriceRow
	for rows.Next() {
		var pr batchPriceRow
		if err := rows.Scan(&pr.priceCents, &pr.saleDate, &pr.platform); err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	prices := make([]int, len(prs))
	dates := make([]string, len(prs))
	for i, pr := range prs {
		prices[i] = pr.priceCents
		dates[i] = pr.saleDate
	}

	midCutoff := now.AddDate(0, 0, -45).Format("2006-01-02")

	var lastSaleCents int
	if len(prices) > 0 {
		lastSaleCents = prices[len(prices)-1]
	}

	return &inventory.CompSummary{
		TotalComps:     totalComps,
		RecentComps:    recentComps,
		MedianCents:    medianInt(slices.Clone(prices)),
		HighestCents:   int(highCents.Int64),
		LowestCents:    int(lowCents.Int64),
		Trend90d:       computeTrend(prices, dates, midCutoff),
		ByPlatform:     platformBreakdownFromRows(prs),
		LastSaleDate:   lastSaleDate.String,
		LastSaleCents:  lastSaleCents,
		PriceCentsList: prices,
	}, nil
}

var _ inventory.CompSummaryProvider = (*MMSalesStore)(nil)
