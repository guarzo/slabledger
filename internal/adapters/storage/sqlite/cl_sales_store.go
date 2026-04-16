package sqlite

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
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		time.Now().UTC().Format(time.RFC3339),
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
		 FROM cl_sales_comps WHERE gem_rate_id = ? AND condition = ? ORDER BY sale_date DESC LIMIT ?`,
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
		`SELECT MAX(sale_date) FROM cl_sales_comps WHERE gem_rate_id = ? AND condition = ?`, gemRateID, condition,
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
		`SELECT cl_condition FROM cl_card_mappings WHERE slab_serial = ?`, certNumber,
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
func conditionFilter(gemRateID, condition string) (clause string, args []any) {
	if condition != "" {
		return "gem_rate_id = ? AND condition = ?", []any{gemRateID, condition}
	}
	// No condition resolved — return nothing rather than mixing grades
	return "gem_rate_id = ? AND 1=0", []any{gemRateID}
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

	condClause, condArgs := conditionFilter(gemRateID, condition)

	// Aggregation query — threshold-agnostic (no CL or cost comparisons)
	var totalComps, recentComps int
	var highCents, lowCents sql.NullInt64
	var lastSaleDate sql.NullString
	args := append([]any{cutoff, cutoff, cutoff}, condArgs...)
	err = s.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*) as total_comps,
			COUNT(CASE WHEN sale_date >= ? THEN 1 END) as recent_comps,
			MAX(CASE WHEN sale_date >= ? THEN price_cents END) as high_cents,
			MIN(CASE WHEN sale_date >= ? THEN price_cents END) as low_cents,
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
	condClause, condArgs := conditionFilter(gemRateID, condition)
	args := append([]any{cutoff}, condArgs...)
	rows, err := s.db.QueryContext(ctx,
		`SELECT price_cents, sale_date FROM cl_sales_comps
		 WHERE sale_date >= ? AND `+condClause+`
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
	condClause, condArgs := conditionFilter(gemRateID, condition)
	args := append([]any{cutoff}, condArgs...)
	rows, err := s.db.QueryContext(ctx,
		`SELECT platform, price_cents FROM cl_sales_comps
		 WHERE sale_date >= ? AND `+condClause+`
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

// medianInt returns the median of an int slice, or 0 if empty.
// Sorts the input in place.
func medianInt(vals []int) int {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sort.Ints(vals)
	if n%2 == 1 {
		return vals[n/2]
	}
	return (vals[n/2-1] + vals[n/2]) / 2
}

// computeTrend compares median of earlier half vs recent half of the 90-day window.
// midCutoff splits the window: dates < midCutoff are "earlier", >= midCutoff are "recent".
func computeTrend(prices []int, dates []string, midCutoff string) float64 {
	var earlier, recent []int
	for i, d := range dates {
		if d < midCutoff {
			earlier = append(earlier, prices[i])
		} else {
			recent = append(recent, prices[i])
		}
	}
	if len(earlier) == 0 || len(recent) == 0 {
		return 0
	}
	medEarlier := medianInt(earlier)
	medRecent := medianInt(recent)
	if medEarlier == 0 {
		return 0
	}
	return float64(medRecent-medEarlier) / float64(medEarlier)
}

// Compile-time check: CLSalesStore implements CompSummaryProvider.
var _ inventory.CompSummaryProvider = (*CLSalesStore)(nil)
