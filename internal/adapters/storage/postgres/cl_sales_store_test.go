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

	// Insert sales comps. Use recent dates so they fall inside the 90-day window.
	now := "2026-04-20"
	for _, row := range [][]any{
		{"gem-a", "item-a-1", now, 10000, "ebay", "PSA 10"},
		{"gem-a", "item-a-2", now, 12000, "ebay", "PSA 10"},
		{"gem-a", "item-a-3", now, 11000, "ebay", "PSA 10"},
		{"gem-a", "item-a-4", now, 13000, "ebay", "PSA 10"},
		{"gem-a", "item-a-5", now, 9000, "ebay", "PSA 10"},
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
	assert.Len(t, a.PriceCentsList, 5)

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
