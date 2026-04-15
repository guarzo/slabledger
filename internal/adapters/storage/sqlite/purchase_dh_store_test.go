package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListDHPendingItems_ReturnsOnlyPendingReceivedUnsold verifies the filter:
// only purchases with dh_push_status='pending', received_at NOT NULL, no sale,
// and a non-closed parent campaign are returned.
func TestListDHPendingItems_ReturnsOnlyPendingReceivedUnsold(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Active campaign
	c := &inventory.Campaign{ID: "camp-1", Name: "Active", Phase: inventory.PhaseActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	received := now.Format(time.RFC3339)

	// Purchase A: pending, received, no sale → should be returned
	pa := &inventory.Purchase{
		ID:           "purch-A",
		CampaignID:   "camp-1",
		CardName:     "Charizard",
		SetName:      "Base Set",
		CertNumber:   "cert-A",
		Grader:       "PSA",
		GradeValue:   10,
		BuyCostCents: 80000,
		PurchaseDate: "2026-01-01",
		ReceivedAt:   &received,
		DHPushStatus: inventory.DHPushStatusPending,
		CreatedAt:    now.Add(-48 * time.Hour),
		UpdatedAt:    now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, pa))
	// mid_price_cents is not populated by CreatePurchase; set it directly.
	_, err := repo.PurchaseStore.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET mid_price_cents = ? WHERE id = ?`, 150000, "purch-A")
	require.NoError(t, err)

	// Purchase B: pending but NOT received → should be excluded
	pb := &inventory.Purchase{
		ID:           "purch-B",
		CampaignID:   "camp-1",
		CardName:     "Blastoise",
		CertNumber:   "cert-B",
		Grader:       "PSA",
		GradeValue:   9,
		BuyCostCents: 50000,
		PurchaseDate: "2026-01-02",
		ReceivedAt:   nil,
		DHPushStatus: inventory.DHPushStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, pb))

	// Purchase C: pending, received, but has a sale → should be excluded
	pc := &inventory.Purchase{
		ID:           "purch-C",
		CampaignID:   "camp-1",
		CardName:     "Venusaur",
		CertNumber:   "cert-C",
		Grader:       "PSA",
		GradeValue:   9,
		BuyCostCents: 60000,
		PurchaseDate: "2026-01-03",
		ReceivedAt:   &received,
		DHPushStatus: inventory.DHPushStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, pc))
	require.NoError(t, repo.CreateSale(ctx, &inventory.Sale{
		ID:             "sale-C",
		PurchaseID:     "purch-C",
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 75000,
		SaleDate:       "2026-01-10",
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	items, err := repo.ListDHPendingItems(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1, "only purchase A should match the filter")
	assert.Equal(t, "purch-A", items[0].PurchaseID)
	assert.Equal(t, "Charizard", items[0].CardName)
	assert.Equal(t, "Base Set", items[0].SetName)
	assert.Equal(t, 10.0, items[0].Grade)
	assert.Equal(t, 150000, items[0].RecommendedPriceCents)
	// Never synced → confidence is "low" and daysQueued derived from created_at.
	assert.Equal(t, "low", items[0].DHConfidence)
	assert.GreaterOrEqual(t, items[0].DaysQueued, 1)
}

// TestListDHPendingItems_ConfidenceFromLastSynced verifies the three confidence buckets.
func TestListDHPendingItems_ConfidenceFromLastSynced(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	c := &inventory.Campaign{ID: "camp-1", Name: "Active", Phase: inventory.PhaseActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	received := now.Format(time.RFC3339)

	mkPurchase := func(id, cert string, syncedAt time.Time, hasSynced bool) {
		p := &inventory.Purchase{
			ID:           id,
			CampaignID:   "camp-1",
			CardName:     "Pikachu",
			CertNumber:   cert,
			Grader:       "PSA",
			GradeValue:   9,
			BuyCostCents: 10000,
			PurchaseDate: "2026-01-01",
			ReceivedAt:   &received,
			DHPushStatus: inventory.DHPushStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p))
		// The INSERT doesn't set dh_last_synced_at, so update it directly.
		if hasSynced {
			_, err := repo.PurchaseStore.db.ExecContext(ctx,
				`UPDATE campaign_purchases SET dh_last_synced_at = ? WHERE id = ?`,
				syncedAt.Format(time.RFC3339), id)
			require.NoError(t, err)
		}
	}

	mkPurchase("p-high", "c-high", now.Add(-2*time.Hour), true)   // <24h → high
	mkPurchase("p-med", "c-med", now.Add(-3*24*time.Hour), true)  // <7d → medium
	mkPurchase("p-low", "c-low", now.Add(-10*24*time.Hour), true) // >7d → low
	mkPurchase("p-never", "c-never", time.Time{}, false)          // never synced → low

	items, err := repo.ListDHPendingItems(ctx)
	require.NoError(t, err)
	require.Len(t, items, 4)

	byID := make(map[string]inventory.DHPendingItem, len(items))
	for _, item := range items {
		byID[item.PurchaseID] = item
	}
	assert.Equal(t, "high", byID["p-high"].DHConfidence)
	assert.Equal(t, "medium", byID["p-med"].DHConfidence)
	assert.Equal(t, "low", byID["p-low"].DHConfidence)
	assert.Equal(t, "low", byID["p-never"].DHConfidence)
}

// TestListDHPendingItems_ExcludesClosedCampaign verifies the c.phase != 'closed' clause.
func TestListDHPendingItems_ExcludesClosedCampaign(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	closed := &inventory.Campaign{ID: "camp-closed", Name: "Closed", Phase: inventory.PhaseClosed, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, closed))

	received := now.Format(time.RFC3339)
	p := &inventory.Purchase{
		ID:           "purch-closed",
		CampaignID:   "camp-closed",
		CardName:     "Mewtwo",
		CertNumber:   "cert-closed",
		Grader:       "PSA",
		GradeValue:   9,
		BuyCostCents: 50000,
		PurchaseDate: "2026-01-01",
		ReceivedAt:   &received,
		DHPushStatus: inventory.DHPushStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, p))

	items, err := repo.ListDHPendingItems(ctx)
	require.NoError(t, err)
	assert.Len(t, items, 0)
}

// TestListDHPendingItems_EmptyReturnsNonNilSlice ensures JSON encodes `[]` not `null`.
func TestListDHPendingItems_EmptyReturnsNonNilSlice(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	items, err := repo.ListDHPendingItems(ctx)
	require.NoError(t, err)
	require.NotNil(t, items)
	assert.Len(t, items, 0)
}
