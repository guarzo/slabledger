package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLSalesStore_GetCompSummariesByKeys(t *testing.T) {
	db := setupTestDB(t)
	store := NewCLSalesStore(db.DB)
	ctx := context.Background()

	// Clean up cl_card_mappings and cl_sales_comps for our test keys.
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM cl_card_mappings WHERE slab_serial IN ('cert-a','cert-b','cert-missing')`)
		_, _ = db.ExecContext(ctx, `DELETE FROM cl_sales_comps WHERE gem_rate_id IN ('gem-a','gem-b','gem-missing')`)
	})

	// Seed two card variants (different gemRateIDs) with cl_card_mappings + comps.
	// gem-a / cert-a → PSA 10, 5 sales over last 30 days
	// gem-b / cert-b → PSA 9,  3 sales over last 30 days
	_, err := db.ExecContext(ctx,
		`INSERT INTO cl_card_mappings (slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition)
		 VALUES ('cert-a', 'coll-a', 'gem-a', 'PSA 10'),
		        ('cert-b', 'coll-b', 'gem-b', 'PSA 9')`)
	require.NoError(t, err)

	// Insert sales comps. Spread gem-a across distinct dates so ORDER BY sale_date ASC
	// is deterministic, allowing us to assert LastSaleCents unambiguously.
	// Prices: 9000 (Apr 16), 11000 (Apr 17), 12000 (Apr 18), 10000 (Apr 19), 13000 (Apr 20).
	// Sorted: [9000, 10000, 11000, 12000, 13000] → median=11000, last=13000, high=13000, low=9000.
	now := "2026-04-20"
	for _, row := range [][]any{
		{"gem-a", "item-a-1", "2026-04-16", 9000, "ebay", "PSA 10"},
		{"gem-a", "item-a-2", "2026-04-17", 11000, "ebay", "PSA 10"},
		{"gem-a", "item-a-3", "2026-04-18", 12000, "ebay", "PSA 10"},
		{"gem-a", "item-a-4", "2026-04-19", 10000, "ebay", "PSA 10"},
		{"gem-a", "item-a-5", now, 13000, "ebay", "PSA 10"},
		{"gem-b", "item-b-1", now, 4000, "ebay", "PSA 9"},
		{"gem-b", "item-b-2", now, 5000, "ebay", "PSA 9"},
		{"gem-b", "item-b-3", now, 4500, "ebay", "PSA 9"},
	} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO cl_sales_comps
			 (gem_rate_id, item_id, sale_date, price_cents, platform, condition)
			 VALUES ($1, $2, $3, $4, $5, $6)`, row...)
		require.NoError(t, err)
	}

	keys := []inventory.CompKey{
		{GemRateID: "gem-a", CertNumber: "cert-a"},
		{GemRateID: "gem-b", CertNumber: "cert-b"},
		{GemRateID: "gem-missing", CertNumber: "cert-missing"}, // absent in mappings → skipped
	}
	got, err := store.GetCompSummariesByKeys(ctx, keys)
	require.NoError(t, err)

	require.Len(t, got, 2)

	a := got[inventory.CompKey{GemRateID: "gem-a", CertNumber: "cert-a"}]
	require.NotNil(t, a)
	assert.Equal(t, 5, a.TotalComps)
	assert.Equal(t, 5, a.RecentComps)
	assert.Equal(t, 11000, a.MedianCents)
	assert.Equal(t, 13000, a.HighestCents)
	assert.Equal(t, 9000, a.LowestCents)
	assert.Equal(t, 13000, a.LastSaleCents) // 2026-04-20 row is the most recent
	assert.Len(t, a.PriceCentsList, 5)
	require.Len(t, a.ByPlatform, 1)
	assert.Equal(t, "ebay", a.ByPlatform[0].Platform)
	assert.Equal(t, 5, a.ByPlatform[0].SaleCount)

	b := got[inventory.CompKey{GemRateID: "gem-b", CertNumber: "cert-b"}]
	require.NotNil(t, b)
	assert.Equal(t, 3, b.TotalComps)
	assert.Equal(t, 3, b.RecentComps)
	assert.Equal(t, 4500, b.MedianCents)
}

func TestCLSalesStore_GetCompSummariesByKeys_EmptyInput(t *testing.T) {
	db := setupTestDB(t)
	store := NewCLSalesStore(db.DB)
	got, err := store.GetCompSummariesByKeys(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
