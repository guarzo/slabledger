package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// TestSaleColumnsMatchScanDests pins the one invariant the compiler cannot:
// the canonical column list and the scan-destination slice must have the same
// length, or every Scan silently reads the wrong field into the wrong column.
func TestSaleColumnsMatchScanDests(t *testing.T) {
	cols := strings.Split(saleColumns, ",")
	dests := saleScanDests(&saleNulls{})
	require.Len(t, dests, len(cols), "saleScanDests must have one destination per saleColumns entry")
}

// TestSaleColumnsAliasedIsDerived guards the drift this ticket fixed: the JOIN
// column list must stay a mechanical alias of the canonical one. Before SLA-85
// the two were maintained by hand and had diverged by six columns, so
// JOIN-loaded sales came back with those fields zeroed.
func TestSaleColumnsAliasedIsDerived(t *testing.T) {
	canonical := strings.Split(saleColumns, ",")
	aliased := strings.Split(saleColumnsAliased, ",")
	require.Len(t, aliased, len(canonical), "aliased list must cover every canonical column")
	for i := range canonical {
		want := "s." + strings.TrimSpace(canonical[i])
		require.Equal(t, want, strings.TrimSpace(aliased[i]))
	}
}

// TestSaleLoadsIdenticallyThroughBothPaths reads one sale through the direct
// SELECT (SaleStore) and through the LEFT JOIN (AnalyticsStore) and requires the
// two inventory.Sale values to be equal. A column added to one list but not the
// other fails here.
func TestSaleLoadsIdenticallyThroughBothPaths(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ss := NewSaleStore(db.DB, logger)
	as := NewAnalyticsStore(db.DB, logger)
	ctx := context.Background()

	const campaignID = "camp-sale-bothpaths"
	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ($1, 'Both Paths Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`, campaignID)
	require.NoError(t, err, "seed campaign")

	p := makeTestPurchase()
	p.CampaignID = campaignID
	require.NoError(t, ps.CreatePurchase(ctx, p), "create purchase")

	// Populate every sale column, so a dropped column shows up as a zeroed
	// field rather than coinciding with the Go zero value.
	feePct := 0.129
	sale := makeTestSale(p.ID)
	sale.SaleChannel = inventory.SaleChannelEbay
	sale.SaleFeeCents = 780
	sale.LastSoldCents = 5500
	sale.LowestListCents = 5200
	sale.ConservativeCents = 4900
	sale.MedianCents = 5300
	sale.ActiveListings = 7
	sale.SalesLast30d = 3
	sale.Trend30d = 1.25
	sale.SnapshotDate = "2026-06-30"
	sale.SnapshotJSON = `{"source":"test"}`
	sale.OriginalListPriceCents = 7500
	sale.PriceReductions = 2
	sale.DaysListed = 41
	sale.SoldAtAskingPrice = true
	sale.WasCracked = true
	sale.OrderID = "order-both-paths"
	sale.ForcedLiquidation = true
	sale.SaleReason = "invoice_pressure"
	sale.CLValueAtSaleCents = 6100
	sale.ChannelFeePctAtSale = &feePct
	sale.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	sale.UpdatedAt = sale.CreatedAt
	require.NoError(t, ss.CreateSale(ctx, sale), "create sale")

	direct, err := ss.GetSaleByPurchaseID(ctx, p.ID)
	require.NoError(t, err, "get sale by purchase id")

	withSales, err := as.GetPurchasesWithSales(ctx, campaignID)
	require.NoError(t, err, "get purchases with sales")

	var joined *inventory.Sale
	for _, pws := range withSales {
		if pws.Purchase.ID == p.ID {
			joined = pws.Sale
			break
		}
	}
	require.NotNil(t, joined, "JOIN path returned no sale for the seeded purchase")
	require.Equal(t, *direct, *joined, "the direct and JOIN read paths must agree")
}
