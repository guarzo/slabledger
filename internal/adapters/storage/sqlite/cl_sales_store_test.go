package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Pure function tests (no DB required) ---

func TestCLSales_medianInt(t *testing.T) {
	tests := []struct {
		name string
		vals []int
		want int
	}{
		{"empty slice", nil, 0},
		{"single element", []int{500}, 500},
		{"odd count", []int{300, 100, 200}, 200},
		{"even count", []int{100, 200, 300, 400}, 250},
		{"even count rounding down", []int{100, 201}, 150},
		{"already sorted", []int{10, 20, 30, 40, 50}, 30},
		{"reverse sorted", []int{50, 40, 30, 20, 10}, 30},
		{"duplicates", []int{100, 100, 100}, 100},
		{"two elements", []int{100, 200}, 150},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := medianInt(tc.vals)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCLSales_computeTrend(t *testing.T) {
	midCutoff := "2026-02-15"

	tests := []struct {
		name   string
		prices []int
		dates  []string
		want   float64
	}{
		{
			name:   "no earlier data",
			prices: []int{1000, 2000},
			dates:  []string{"2026-02-15", "2026-02-20"},
			want:   0,
		},
		{
			name:   "no recent data",
			prices: []int{1000, 2000},
			dates:  []string{"2026-01-10", "2026-01-20"},
			want:   0,
		},
		{
			name:   "increasing trend",
			prices: []int{1000, 1200, 2000, 2200},
			dates:  []string{"2026-01-10", "2026-01-20", "2026-02-20", "2026-02-25"},
			want:   float64(2100-1100) / float64(1100), // median recent=2100, earlier=1100
		},
		{
			name:   "decreasing trend",
			prices: []int{2000, 2200, 1000, 1200},
			dates:  []string{"2026-01-10", "2026-01-20", "2026-02-20", "2026-02-25"},
			want:   float64(1100-2100) / float64(2100),
		},
		{
			name:   "stable trend",
			prices: []int{1000, 1000},
			dates:  []string{"2026-01-10", "2026-02-20"},
			want:   0,
		},
		{
			name:   "earlier median is zero",
			prices: []int{0, 1000},
			dates:  []string{"2026-01-10", "2026-02-20"},
			want:   0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeTrend(tc.prices, tc.dates, midCutoff)
			assert.InDelta(t, tc.want, got, 0.0001)
		})
	}
}

func TestCLSales_conditionFilter(t *testing.T) {
	tests := []struct {
		name       string
		gemRateID  string
		condition  string
		wantClause string
		wantArgs   []any
	}{
		{
			name:       "with condition",
			gemRateID:  "gem-1",
			condition:  "NM-MT 8",
			wantClause: "gem_rate_id = ? AND condition = ?",
			wantArgs:   []any{"gem-1", "NM-MT 8"},
		},
		{
			name:       "empty condition returns guard clause",
			gemRateID:  "gem-1",
			condition:  "",
			wantClause: "gem_rate_id = ? AND 1=0",
			wantArgs:   []any{"gem-1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clause, args := conditionFilter(tc.gemRateID, tc.condition)
			assert.Equal(t, tc.wantClause, clause)
			assert.Equal(t, tc.wantArgs, args)
		})
	}
}

// --- Database integration tests ---

func seedCLCardMapping(t *testing.T, db *DB, slabSerial, gemRateID, condition string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO cl_card_mappings (slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition) VALUES (?, ?, ?, ?)`,
		slabSerial, "col-"+slabSerial, gemRateID, condition,
	)
	require.NoError(t, err)
}

func newCLSaleComp(gemRateID, condition, itemID, saleDate string, priceCents int, platform string) CLSaleCompRecord {
	return CLSaleCompRecord{
		GemRateID:   gemRateID,
		Condition:   condition,
		ItemID:      itemID,
		SaleDate:    saleDate,
		PriceCents:  priceCents,
		Platform:    platform,
		ListingType: "auction",
		Seller:      "test-seller",
		ItemURL:     "https://example.com/" + itemID,
		SlabSerial:  "slab-" + itemID,
	}
}

func TestCLSales_UpsertSaleComp(t *testing.T) {
	t.Run("insert new record", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		rec := newCLSaleComp("cls-gem-1", "GEM-MT 10", "cls-item-1", "2026-03-01", 50000, "eBay")
		err := store.UpsertSaleComp(ctx, rec)
		require.NoError(t, err)

		comps, err := store.GetSaleComps(ctx, "cls-gem-1", "GEM-MT 10", 10)
		require.NoError(t, err)
		require.Len(t, comps, 1)
		assert.Equal(t, 50000, comps[0].PriceCents)
		assert.Equal(t, "eBay", comps[0].Platform)
		assert.Equal(t, "cls-item-1", comps[0].ItemID)
	})

	t.Run("upsert updates existing record on conflict", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		// Insert initial record
		rec := newCLSaleComp("cls-gem-1", "GEM-MT 10", "cls-item-1", "2026-03-01", 50000, "eBay")
		require.NoError(t, store.UpsertSaleComp(ctx, rec))

		// Same gemRateID + condition + itemID → should update
		rec = newCLSaleComp("cls-gem-1", "GEM-MT 10", "cls-item-1", "2026-03-05", 55000, "TCGPlayer")
		err := store.UpsertSaleComp(ctx, rec)
		require.NoError(t, err)

		comps, err := store.GetSaleComps(ctx, "cls-gem-1", "GEM-MT 10", 10)
		require.NoError(t, err)
		require.Len(t, comps, 1)
		assert.Equal(t, 55000, comps[0].PriceCents)
		assert.Equal(t, "TCGPlayer", comps[0].Platform)
		assert.Contains(t, comps[0].SaleDate, "2026-03-05")
	})

	t.Run("different itemID inserts new row", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		rec1 := newCLSaleComp("cls-gem-1", "GEM-MT 10", "cls-item-1", "2026-03-01", 50000, "eBay")
		require.NoError(t, store.UpsertSaleComp(ctx, rec1))

		rec2 := newCLSaleComp("cls-gem-1", "GEM-MT 10", "cls-item-2", "2026-03-02", 48000, "eBay")
		err := store.UpsertSaleComp(ctx, rec2)
		require.NoError(t, err)

		comps, err := store.GetSaleComps(ctx, "cls-gem-1", "GEM-MT 10", 10)
		require.NoError(t, err)
		assert.Len(t, comps, 2)
	})

	t.Run("different condition inserts new row", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		rec8 := newCLSaleComp("cls-gem-1", "NM-MT 8", "cls-item-1", "2026-03-01", 30000, "eBay")
		require.NoError(t, store.UpsertSaleComp(ctx, rec8))

		rec10 := newCLSaleComp("cls-gem-1", "GEM-MT 10", "cls-item-1", "2026-03-01", 50000, "eBay")
		require.NoError(t, store.UpsertSaleComp(ctx, rec10))

		// Both conditions coexist independently
		comps8, err := store.GetSaleComps(ctx, "cls-gem-1", "NM-MT 8", 10)
		require.NoError(t, err)
		require.Len(t, comps8, 1)
		assert.Equal(t, 30000, comps8[0].PriceCents)

		comps10, err := store.GetSaleComps(ctx, "cls-gem-1", "GEM-MT 10", 10)
		require.NoError(t, err)
		require.Len(t, comps10, 1)
		assert.Equal(t, 50000, comps10[0].PriceCents)
	})
}

func TestCLSales_GetSaleComps(t *testing.T) {
	ctx := context.Background()

	t.Run("empty result for unknown gemRateID", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)

		comps, err := store.GetSaleComps(ctx, "cls-nonexistent", "GEM-MT 10", 10)
		require.NoError(t, err)
		assert.Empty(t, comps)
	})

	// seedComps inserts n sale comps for the given gemRateID and condition.
	seedComps := func(t *testing.T, store *CLSalesStore, gemRateID, condition string, n int) {
		t.Helper()
		for i := 1; i <= n; i++ {
			rec := newCLSaleComp(gemRateID, condition,
				fmt.Sprintf("cls-ord-%d", i),
				fmt.Sprintf("2026-03-%02d", i),
				10000*i, "eBay")
			require.NoError(t, store.UpsertSaleComp(ctx, rec))
		}
	}

	t.Run("returns results ordered by date descending", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)

		seedComps(t, store, "cls-gem-order", "GEM-MT 10", 5)

		comps, err := store.GetSaleComps(ctx, "cls-gem-order", "GEM-MT 10", 10)
		require.NoError(t, err)
		require.Len(t, comps, 5)
		// Most recent first
		assert.Contains(t, comps[0].SaleDate, "2026-03-05")
		assert.Contains(t, comps[4].SaleDate, "2026-03-01")
	})

	t.Run("respects limit", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)

		seedComps(t, store, "cls-gem-order", "GEM-MT 10", 5)

		comps, err := store.GetSaleComps(ctx, "cls-gem-order", "GEM-MT 10", 3)
		require.NoError(t, err)
		assert.Len(t, comps, 3)
		assert.Contains(t, comps[0].SaleDate, "2026-03-05")
	})

	t.Run("filters by condition", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)

		seedComps(t, store, "cls-gem-order", "GEM-MT 10", 5)
		rec := newCLSaleComp("cls-gem-order", "NM 7", "cls-ord-nm7", "2026-03-01", 5000, "eBay")
		require.NoError(t, store.UpsertSaleComp(ctx, rec))

		comps10, err := store.GetSaleComps(ctx, "cls-gem-order", "GEM-MT 10", 10)
		require.NoError(t, err)
		assert.Len(t, comps10, 5)

		comps7, err := store.GetSaleComps(ctx, "cls-gem-order", "NM 7", 10)
		require.NoError(t, err)
		assert.Len(t, comps7, 1)
	})
}

func TestCLSales_GetLatestSaleDate(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty for no data", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)

		date, err := store.GetLatestSaleDate(ctx, "cls-nodata", "GEM-MT 10")
		require.NoError(t, err)
		assert.Equal(t, "", date)
	})

	// seedLatestDates inserts records for the given gemRateID, condition, and dates.
	seedLatestDates := func(t *testing.T, store *CLSalesStore, gemRateID, condition string, dates []string) {
		t.Helper()
		for i, d := range dates {
			rec := newCLSaleComp(gemRateID, condition,
				fmt.Sprintf("cls-lat-%d", i), d, 10000, "eBay")
			require.NoError(t, store.UpsertSaleComp(ctx, rec))
		}
	}

	t.Run("returns latest date", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)

		seedLatestDates(t, store, "cls-gem-latest", "GEM-MT 10", []string{"2026-01-10", "2026-03-15", "2026-02-20"})

		date, err := store.GetLatestSaleDate(ctx, "cls-gem-latest", "GEM-MT 10")
		require.NoError(t, err)
		assert.Equal(t, "2026-03-15", date)
	})

	t.Run("scoped to condition", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)

		// Seed both conditions in same DB
		seedLatestDates(t, store, "cls-gem-latest", "GEM-MT 10", []string{"2026-01-10", "2026-03-15", "2026-02-20"})

		rec := newCLSaleComp("cls-gem-latest", "NM-MT 8", "cls-lat-8", "2026-04-01", 8000, "eBay")
		require.NoError(t, store.UpsertSaleComp(ctx, rec))

		date, err := store.GetLatestSaleDate(ctx, "cls-gem-latest", "NM-MT 8")
		require.NoError(t, err)
		assert.Equal(t, "2026-04-01", date)

		// Original condition still returns its own max
		date10, err := store.GetLatestSaleDate(ctx, "cls-gem-latest", "GEM-MT 10")
		require.NoError(t, err)
		assert.Equal(t, "2026-03-15", date10)
	})
}

func TestCLSales_lookupCondition(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewCLSalesStore(db.DB)
	ctx := context.Background()

	t.Run("empty cert returns empty", func(t *testing.T) {
		cond, err := store.lookupCondition(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, "", cond)
	})

	t.Run("no mapping returns empty", func(t *testing.T) {
		cond, err := store.lookupCondition(ctx, "cls-unknown-cert")
		require.NoError(t, err)
		assert.Equal(t, "", cond)
	})

	t.Run("returns mapped condition", func(t *testing.T) {
		seedCLCardMapping(t, db, "cls-cert-100", "cls-gem-lk", "GEM-MT 10")

		cond, err := store.lookupCondition(ctx, "cls-cert-100")
		require.NoError(t, err)
		assert.Equal(t, "GEM-MT 10", cond)
	})
}

func TestCLSales_GetCompSummary(t *testing.T) {
	// Fixed anchor at today's midnight for deterministic date arithmetic.
	// Must be "today" because GetCompSummary SQL uses date('now', '-90 days').
	now := time.Now()
	anchor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	t.Run("returns nil when no comps exist", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		gemRateID := "cls-gem-summary"
		certNumber := "cls-cert-summary"
		seedCLCardMapping(t, db, certNumber, gemRateID, "GEM-MT 10")

		summary, err := store.GetCompSummary(ctx, gemRateID, certNumber)
		require.NoError(t, err)
		assert.Nil(t, summary)
	})

	t.Run("returns nil when all comps are older than 90 days", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		gemRateID := "cls-gem-summary"
		condition := "GEM-MT 10"
		certNumber := "cls-cert-summary"
		seedCLCardMapping(t, db, certNumber, gemRateID, condition)

		oldDate := anchor.AddDate(0, 0, -100).Format("2006-01-02")
		rec := newCLSaleComp(gemRateID, condition, "cls-old-1", oldDate, 40000, "eBay")
		require.NoError(t, store.UpsertSaleComp(ctx, rec))

		summary, err := store.GetCompSummary(ctx, gemRateID, certNumber)
		require.NoError(t, err)
		assert.Nil(t, summary)
	})

	t.Run("returns summary with recent comps", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		gemRateID := "cls-gem-summary"
		condition := "GEM-MT 10"
		certNumber := "cls-cert-summary"
		seedCLCardMapping(t, db, certNumber, gemRateID, condition)

		// Insert old comp (>90 days) to verify TotalComps includes it
		oldDate := anchor.AddDate(0, 0, -100).Format("2006-01-02")
		oldRec := newCLSaleComp(gemRateID, condition, "cls-old-1", oldDate, 40000, "eBay")
		require.NoError(t, store.UpsertSaleComp(ctx, oldRec))

		// Seed comps spread across the 90-day window for trend computation
		// Earlier half (days 60–80 ago)
		earlierDates := []struct {
			daysAgo int
			cents   int
		}{
			{80, 40000},
			{70, 42000},
			{60, 44000},
		}
		// Recent half (days 0–40 ago)
		recentDates := []struct {
			daysAgo int
			cents   int
		}{
			{40, 50000},
			{30, 52000},
			{20, 54000},
			{10, 56000},
			{5, 58000},
		}

		for i, ed := range earlierDates {
			d := anchor.AddDate(0, 0, -ed.daysAgo).Format("2006-01-02")
			rec := newCLSaleComp(gemRateID, condition,
				fmt.Sprintf("cls-sum-e%d", i), d, ed.cents, "eBay")
			require.NoError(t, store.UpsertSaleComp(ctx, rec))
		}
		for i, rd := range recentDates {
			platform := "eBay"
			if i%2 == 0 {
				platform = "TCGPlayer"
			}
			d := anchor.AddDate(0, 0, -rd.daysAgo).Format("2006-01-02")
			rec := newCLSaleComp(gemRateID, condition,
				fmt.Sprintf("cls-sum-r%d", i), d, rd.cents, platform)
			require.NoError(t, store.UpsertSaleComp(ctx, rec))
		}

		summary, err := store.GetCompSummary(ctx, gemRateID, certNumber)
		require.NoError(t, err)
		require.NotNil(t, summary)

		assert.Equal(t, gemRateID, summary.GemRateID)
		// TotalComps includes the old comp (>90d) + 3 earlier + 5 recent = 9
		assert.Equal(t, 9, summary.TotalComps)
		// RecentComps = only those within 90 days = 8 (3 earlier + 5 recent)
		assert.Equal(t, 8, summary.RecentComps)
		// MedianCents of 8 recent prices: 40000,42000,44000,50000,52000,54000,56000,58000
		// sorted: 40000,42000,44000,50000,52000,54000,56000,58000 → median = (50000+52000)/2 = 51000
		assert.Equal(t, 51000, summary.MedianCents)
		assert.Equal(t, 58000, summary.HighestCents)
		assert.Equal(t, 40000, summary.LowestCents)
		// Trend should be positive (recent prices > earlier prices)
		assert.Greater(t, summary.Trend90d, 0.0)
		// PriceCentsList should have all recent prices
		assert.Len(t, summary.PriceCentsList, 8)
		// LastSaleDate should be the most recent
		assert.NotEmpty(t, summary.LastSaleDate)
		// LastSaleCents should be the price of the most recent sale (daysAgo=5, 58000)
		assert.Equal(t, 58000, summary.LastSaleCents)
		// ByPlatform should have both eBay and TCGPlayer
		assert.GreaterOrEqual(t, len(summary.ByPlatform), 2)
	})

	t.Run("returns nil when cert has no mapping (empty condition)", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		gemRateID := "cls-gem-summary"
		certNumber := "cls-cert-summary"
		seedCLCardMapping(t, db, certNumber, gemRateID, "GEM-MT 10")

		// Use a cert number that has no mapping — condition resolves to ""
		// conditionFilter returns "1=0" guard, so no rows match
		summary, err := store.GetCompSummary(ctx, gemRateID, "cls-unmapped-cert")
		require.NoError(t, err)
		assert.Nil(t, summary)
	})

	t.Run("returns nil with empty certNumber", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		store := NewCLSalesStore(db.DB)
		ctx := context.Background()

		gemRateID := "cls-gem-summary"
		certNumber := "cls-cert-summary"
		seedCLCardMapping(t, db, certNumber, gemRateID, "GEM-MT 10")

		// lookupCondition returns "" for empty cert → conditionFilter guard
		summary, err := store.GetCompSummary(ctx, gemRateID, "")
		require.NoError(t, err)
		assert.Nil(t, summary)
	})
}

func TestCLSales_GetCompSummary_platformBreakdown(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewCLSalesStore(db.DB)
	ctx := context.Background()

	gemRateID := "cls-gem-plat"
	condition := "GEM-MT 10"
	certNumber := "cls-cert-plat"
	seedCLCardMapping(t, db, certNumber, gemRateID, condition)

	now := time.Now()
	anchor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// 3 eBay sales and 2 TCGPlayer sales, all recent
	sales := []struct {
		itemID   string
		cents    int
		platform string
		daysAgo  int
	}{
		{"cls-plat-1", 50000, "eBay", 10},
		{"cls-plat-2", 52000, "eBay", 8},
		{"cls-plat-3", 54000, "eBay", 6},
		{"cls-plat-4", 60000, "TCGPlayer", 5},
		{"cls-plat-5", 62000, "TCGPlayer", 3},
	}
	for _, s := range sales {
		d := anchor.AddDate(0, 0, -s.daysAgo).Format("2006-01-02")
		rec := newCLSaleComp(gemRateID, condition, s.itemID, d, s.cents, s.platform)
		require.NoError(t, store.UpsertSaleComp(ctx, rec))
	}

	summary, err := store.GetCompSummary(ctx, gemRateID, certNumber)
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Len(t, summary.ByPlatform, 2)

	// Find platform entries by name
	platMap := make(map[string]int)
	for i, bp := range summary.ByPlatform {
		platMap[bp.Platform] = i
	}

	ebayIdx, ok := platMap["eBay"]
	require.True(t, ok, "expected eBay in platform breakdown")
	ebay := summary.ByPlatform[ebayIdx]
	assert.Equal(t, 3, ebay.SaleCount)
	assert.Equal(t, 52000, ebay.MedianCents) // median of 50000,52000,54000
	assert.Equal(t, 54000, ebay.HighCents)
	assert.Equal(t, 50000, ebay.LowCents)

	tcgIdx, ok := platMap["TCGPlayer"]
	require.True(t, ok, "expected TCGPlayer in platform breakdown")
	tcg := summary.ByPlatform[tcgIdx]
	assert.Equal(t, 2, tcg.SaleCount)
	assert.Equal(t, 61000, tcg.MedianCents) // median of 60000,62000
	assert.Equal(t, 62000, tcg.HighCents)
	assert.Equal(t, 60000, tcg.LowCents)
}
