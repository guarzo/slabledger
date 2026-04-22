package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// CLSaleCompRecord represents a stored sales comp.
type CLSaleCompRecord struct {
	GemRateID   string
	Condition   string
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
		`INSERT INTO cl_sales_comps (gem_rate_id, condition, item_id, sale_date, price_cents, platform, listing_type, seller, item_url, slab_serial, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT(gem_rate_id, condition, item_id) DO UPDATE SET
		   price_cents = excluded.price_cents,
		   sale_date = excluded.sale_date,
		   platform = excluded.platform,
		   listing_type = excluded.listing_type,
		   seller = excluded.seller,
		   item_url = excluded.item_url,
		   slab_serial = excluded.slab_serial`,
		rec.GemRateID, rec.Condition, rec.ItemID, rec.SaleDate, rec.PriceCents,
		rec.Platform, rec.ListingType, rec.Seller, rec.ItemURL, rec.SlabSerial,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert sale comp: %w", err)
	}
	return nil
}

// GetSaleComps returns recent sales for a gemRateID and condition, ordered by date descending.
func (s *CLSalesStore) GetSaleComps(ctx context.Context, gemRateID, condition string, limit int) (_ []CLSaleCompRecord, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT gem_rate_id, condition, item_id, sale_date, price_cents, platform, listing_type, seller, item_url, slab_serial
		 FROM cl_sales_comps WHERE gem_rate_id = $1 AND condition = $2 ORDER BY sale_date DESC LIMIT $3`,
		gemRateID, condition, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	var comps []CLSaleCompRecord
	for rows.Next() {
		var c CLSaleCompRecord
		if err := rows.Scan(&c.GemRateID, &c.Condition, &c.ItemID, &c.SaleDate, &c.PriceCents,
			&c.Platform, &c.ListingType, &c.Seller, &c.ItemURL, &c.SlabSerial); err != nil {
			return nil, err
		}
		comps = append(comps, c)
	}
	return comps, rows.Err()
}

// GetLatestSaleDate returns the most recent sale date for a gemRateID and condition, or empty string if none.
func (s *CLSalesStore) GetLatestSaleDate(ctx context.Context, gemRateID, condition string) (string, error) {
	var date sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(sale_date) FROM cl_sales_comps WHERE gem_rate_id = $1 AND condition = $2`, gemRateID, condition,
	).Scan(&date)
	if err != nil {
		return "", err
	}
	return date.String, nil
}

// lookupCondition resolves the CL grade condition for a cert from cl_card_mappings.
// Returns ("", nil) if no mapping exists; returns ("", err) on real DB errors.
func (s *CLSalesStore) lookupCondition(ctx context.Context, certNumber string) (string, error) {
	if certNumber == "" {
		return "", nil
	}
	var condition sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT cl_condition FROM cl_card_mappings WHERE slab_serial = $1`, certNumber,
	).Scan(&condition)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return condition.String, nil
}

// conditionFilter builds a WHERE clause fragment and args for gemRateID + condition.
// When condition is empty (no mapping found), returns a clause that yields no rows
// so we don't accidentally return mixed-grade comps.
//
// startIndex is the 1-based $N position to start numbering from — callers that
// have already appended N arguments pass len(args)+1.
func conditionFilter(gemRateID, condition string, startIndex int) (clause string, args []any) {
	if condition != "" {
		return fmt.Sprintf("gem_rate_id = $%d AND condition = $%d", startIndex, startIndex+1), []any{gemRateID, condition}
	}
	// No condition resolved — return nothing rather than mixing grades
	return fmt.Sprintf("gem_rate_id = $%d AND 1=0", startIndex), []any{gemRateID}
}

// GetCompSummary computes aggregated comp analytics for a gemRateID filtered by grade.
// certNumber resolves the CL condition (grade) from cl_card_mappings so comps are
// grade-specific (e.g., PSA 10 only, not mixed with PSA 9).
// CompsAboveCL and CompsAboveCost are left at 0 — the caller derives them per-purchase
// from PriceCentsList since different purchases may have different CL values and costs.
func (s *CLSalesStore) GetCompSummary(ctx context.Context, gemRateID, certNumber string) (*inventory.CompSummary, error) {
	condition, err := s.lookupCondition(ctx, certNumber)
	if err != nil {
		return nil, fmt.Errorf("lookup condition for cert %s: %w", certNumber, err)
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -90).Format("2006-01-02")

	// Args order: cutoff (x3 for CASE clauses), then conditionFilter args
	args := []any{cutoff, cutoff, cutoff}
	condClause, condArgs := conditionFilter(gemRateID, condition, len(args)+1)
	args = append(args, condArgs...)

	// Aggregation query — threshold-agnostic (no CL or cost comparisons)
	var totalComps, recentComps int
	var highCents, lowCents sql.NullInt64
	var lastSaleDate sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*) as total_comps,
			COUNT(CASE WHEN sale_date >= $1 THEN 1 END) as recent_comps,
			MAX(CASE WHEN sale_date >= $2 THEN price_cents END) as high_cents,
			MIN(CASE WHEN sale_date >= $3 THEN price_cents END) as low_cents,
			MAX(sale_date) as last_sale_date
		FROM cl_sales_comps WHERE `+condClause,
		args...,
	).Scan(&totalComps, &recentComps, &highCents, &lowCents, &lastSaleDate)
	if err != nil {
		return nil, err
	}

	if totalComps == 0 || recentComps == 0 {
		return nil, nil
	}

	// Fetch recent price list for median + trend computation
	recentPrices, saleDates, err := s.fetchRecentPricesAndDates(ctx, gemRateID, condition, cutoff)
	if err != nil {
		return nil, err
	}

	// Clone before computing median — medianInt sorts in place and
	// recentPrices must stay aligned with saleDates for computeTrend.
	medianCents := medianInt(slices.Clone(recentPrices))

	midCutoff := now.AddDate(0, 0, -45).Format("2006-01-02")
	trend := computeTrend(recentPrices, saleDates, midCutoff)

	// Platform breakdown
	platforms, err := s.fetchPlatformBreakdown(ctx, gemRateID, condition, cutoff)
	if err != nil {
		return nil, err
	}

	// recentPrices is sorted by sale_date ASC, so the last element is the most recent sale.
	var lastSaleCents int
	if len(recentPrices) > 0 {
		lastSaleCents = recentPrices[len(recentPrices)-1]
	}

	return &inventory.CompSummary{
		GemRateID:      gemRateID,
		TotalComps:     totalComps,
		RecentComps:    recentComps,
		MedianCents:    medianCents,
		HighestCents:   int(highCents.Int64),
		LowestCents:    int(lowCents.Int64),
		Trend90d:       trend,
		ByPlatform:     platforms,
		LastSaleDate:   lastSaleDate.String,
		LastSaleCents:  lastSaleCents,
		PriceCentsList: recentPrices,
	}, nil
}

// fetchRecentPricesAndDates returns recent prices and their dates (parallel arrays).
func (s *CLSalesStore) fetchRecentPricesAndDates(ctx context.Context, gemRateID, condition, cutoff string) (prices []int, dates []string, err error) {
	args := []any{cutoff}
	condClause, condArgs := conditionFilter(gemRateID, condition, len(args)+1)
	args = append(args, condArgs...)
	rows, err := s.db.QueryContext(ctx,
		`SELECT price_cents, sale_date FROM cl_sales_comps
		 WHERE sale_date >= $1 AND `+condClause+`
		 ORDER BY sale_date ASC`,
		args...,
	)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	for rows.Next() {
		var p int
		var d string
		if err := rows.Scan(&p, &d); err != nil {
			return nil, nil, err
		}
		prices = append(prices, p)
		dates = append(dates, d)
	}
	return prices, dates, rows.Err()
}

// fetchPlatformBreakdown returns per-platform comp stats for recent comps.
func (s *CLSalesStore) fetchPlatformBreakdown(ctx context.Context, gemRateID, condition, cutoff string) (_ []inventory.PlatformBreakdown, err error) {
	args := []any{cutoff}
	condClause, condArgs := conditionFilter(gemRateID, condition, len(args)+1)
	args = append(args, condArgs...)
	rows, err := s.db.QueryContext(ctx,
		`SELECT platform, price_cents FROM cl_sales_comps
		 WHERE sale_date >= $1 AND `+condClause+`
		 ORDER BY platform`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	platPrices := make(map[string][]int)
	var platOrder []string
	for rows.Next() {
		var platform string
		var price int
		if err := rows.Scan(&platform, &price); err != nil {
			return nil, err
		}
		if _, ok := platPrices[platform]; !ok {
			platOrder = append(platOrder, platform)
		}
		platPrices[platform] = append(platPrices[platform], price)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var result []inventory.PlatformBreakdown
	for _, plat := range platOrder {
		prices := platPrices[plat]
		sort.Ints(prices)
		result = append(result, inventory.PlatformBreakdown{
			Platform:    plat,
			SaleCount:   len(prices),
			MedianCents: medianInt(prices),
			HighCents:   prices[len(prices)-1],
			LowCents:    prices[0],
		})
	}
	return result, nil
}

// GetCompSummariesByKeys is the batch form of GetCompSummary. It issues three
// SQL queries total regardless of the number of keys: (1) resolve conditions
// from cl_card_mappings, (2) aggregate counts/extrema, (3) fetch recent price
// lists for median/trend. Keys whose cert has no mapping, or whose variant has
// no recent comps, are absent from the returned map.
func (s *CLSalesStore) GetCompSummariesByKeys(ctx context.Context, keys []inventory.CompKey) (map[inventory.CompKey]*inventory.CompSummary, error) {
	out := make(map[inventory.CompKey]*inventory.CompSummary)
	if len(keys) == 0 {
		return out, nil
	}

	// 1. Batch-resolve conditions for all cert numbers.
	certs := make([]string, 0, len(keys))
	for _, k := range keys {
		if k.CertNumber != "" {
			certs = append(certs, k.CertNumber)
		}
	}
	conditions, err := s.lookupConditionsBatch(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("batch lookup conditions: %w", err)
	}

	// Reduce keys to unique (gemRateID, condition) pairs we actually want to query,
	// and remember the mapping back to input keys.
	type pair struct{ gemRateID, condition string }
	pairToKeys := make(map[pair][]inventory.CompKey)
	gemIDs := make([]string, 0, len(keys))
	conds := make([]string, 0, len(keys))
	for _, k := range keys {
		cond, ok := conditions[k.CertNumber]
		if !ok || cond == "" {
			continue
		}
		p := pair{gemRateID: k.GemRateID, condition: cond}
		if _, seen := pairToKeys[p]; !seen {
			gemIDs = append(gemIDs, k.GemRateID)
			conds = append(conds, cond)
		}
		pairToKeys[p] = append(pairToKeys[p], k)
	}
	if len(pairToKeys) == 0 {
		return out, nil
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -90).Format("2006-01-02")
	midCutoff := now.AddDate(0, 0, -45).Format("2006-01-02")

	// 2. Aggregation: one row per (gem_rate_id, condition) pair.
	aggQuery := `
		SELECT gem_rate_id, condition,
			COUNT(*) AS total_comps,
			COUNT(CASE WHEN sale_date >= $3 THEN 1 END) AS recent_comps,
			MAX(CASE WHEN sale_date >= $3 THEN price_cents END) AS high_cents,
			MIN(CASE WHEN sale_date >= $3 THEN price_cents END) AS low_cents,
			MAX(sale_date) AS last_sale_date
		FROM cl_sales_comps
		WHERE (gem_rate_id, condition) IN (SELECT * FROM UNNEST($1::text[], $2::text[]))
		GROUP BY gem_rate_id, condition`
	rows, err := s.db.QueryContext(ctx, aggQuery, gemIDs, conds, cutoff)
	if err != nil {
		return nil, fmt.Errorf("aggregate: %w", err)
	}
	aggs := make(map[pair]*inventory.CompSummary)
	for rows.Next() {
		var p pair
		var totalComps, recentComps int
		var highCents, lowCents sql.NullInt64
		var lastSaleDate sql.NullString
		if err := rows.Scan(&p.gemRateID, &p.condition, &totalComps, &recentComps, &highCents, &lowCents, &lastSaleDate); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan aggregate: %w", err)
		}
		if totalComps == 0 || recentComps == 0 {
			continue
		}
		sum := &inventory.CompSummary{
			GemRateID:    p.gemRateID,
			TotalComps:   totalComps,
			RecentComps:  recentComps,
			HighestCents: int(highCents.Int64),
			LowestCents:  int(lowCents.Int64),
			LastSaleDate: lastSaleDate.String,
		}
		aggs[p] = sum
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate aggregate: %w", err)
	}
	_ = rows.Close()

	if len(aggs) == 0 {
		return out, nil
	}

	// 3. Recent prices + sale dates for every matched pair (for median + trend).
	priceQuery := `
		SELECT gem_rate_id, condition, price_cents, sale_date, platform
		FROM cl_sales_comps
		WHERE (gem_rate_id, condition) IN (SELECT * FROM UNNEST($1::text[], $2::text[]))
		  AND sale_date >= $3
		ORDER BY gem_rate_id, condition, sale_date`
	priceRows, err := s.db.QueryContext(ctx, priceQuery, gemIDs, conds, cutoff)
	if err != nil {
		return nil, fmt.Errorf("recent prices: %w", err)
	}
	byPair := make(map[pair][]batchPriceRow)
	for priceRows.Next() {
		var p pair
		var pr batchPriceRow
		if err := priceRows.Scan(&p.gemRateID, &p.condition, &pr.priceCents, &pr.saleDate, &pr.platform); err != nil {
			_ = priceRows.Close()
			return nil, fmt.Errorf("scan recent prices: %w", err)
		}
		byPair[p] = append(byPair[p], pr)
	}
	if err := priceRows.Err(); err != nil {
		_ = priceRows.Close()
		return nil, fmt.Errorf("iterate recent prices: %w", err)
	}
	_ = priceRows.Close()

	// Compute median, trend, platform breakdown, attach PriceCentsList.
	for p, sum := range aggs {
		prs := byPair[p]
		if len(prs) == 0 {
			continue
		}
		prices := make([]int, len(prs))
		dates := make([]string, len(prs))
		for i, pr := range prs {
			prices[i] = pr.priceCents
			dates[i] = pr.saleDate
		}
		sum.MedianCents = medianInt(slices.Clone(prices))
		sum.Trend90d = computeTrend(prices, dates, midCutoff)
		sum.ByPlatform = platformBreakdownFromRows(prs)
		sum.PriceCentsList = prices
		sum.LastSaleCents = prs[len(prs)-1].priceCents

		// Fan out to every input key that resolved to this (gemRateID, condition).
		for _, k := range pairToKeys[p] {
			// Return independent copies so callers can mutate CompsAboveCL/CompsAboveCost.
			cs := *sum
			out[k] = &cs
		}
	}
	return out, nil
}

// lookupConditionsBatch resolves CL conditions for a slice of cert numbers in one query.
// Returns a map keyed by cert number; missing certs are absent from the map.
func (s *CLSalesStore) lookupConditionsBatch(ctx context.Context, certs []string) (map[string]string, error) {
	out := make(map[string]string, len(certs))
	if len(certs) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT slab_serial, cl_condition FROM cl_card_mappings WHERE slab_serial = ANY($1::text[])`,
		certs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cert string
		var cond sql.NullString
		if err := rows.Scan(&cert, &cond); err != nil {
			return nil, err
		}
		out[cert] = cond.String
	}
	return out, rows.Err()
}

// Compile-time check: CLSalesStore implements CompSummaryProvider.
var _ inventory.CompSummaryProvider = (*CLSalesStore)(nil)
