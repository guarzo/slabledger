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

// TestCountDHPipelineHealth covers the filters that make the dashboard counts
// match the queue list and expose the "unenrolled" black-hole bucket.
func TestCountDHPipelineHealth(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	received := now.Format(time.RFC3339)

	active := &inventory.Campaign{ID: "camp-active", Name: "Active", Phase: inventory.PhaseActive, CreatedAt: now, UpdatedAt: now}
	closed := &inventory.Campaign{ID: "camp-closed", Name: "Closed", Phase: inventory.PhaseClosed, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, active))
	require.NoError(t, repo.CreateCampaign(ctx, closed))

	mkPurchase := func(id, campaignID string, received *string, pushStatus inventory.DHPushStatus, invID int) *inventory.Purchase {
		return &inventory.Purchase{
			ID: id, CampaignID: campaignID,
			CardName: "Charizard", SetName: "Base Set",
			CertNumber: id, Grader: "PSA", GradeValue: 9,
			BuyCostCents: 10000, PurchaseDate: "2026-01-01",
			ReceivedAt: received, DHPushStatus: pushStatus, DHInventoryID: invID,
			CreatedAt: now, UpdatedAt: now,
		}
	}

	cases := []*inventory.Purchase{
		// Counts in pending_received: pending + received + unsold + active campaign.
		mkPurchase("pending-received-1", "camp-active", &received, inventory.DHPushStatusPending, 0),
		mkPurchase("pending-received-2", "camp-active", &received, inventory.DHPushStatusPending, 0),
		// Not received → should not count toward pending_received.
		mkPurchase("pending-not-received", "camp-active", nil, inventory.DHPushStatusPending, 0),
		// Closed campaign → excluded.
		mkPurchase("pending-closed", "camp-closed", &received, inventory.DHPushStatusPending, 0),
		// Counts as unenrolled_received: empty status + received + inv_id=0 + unsold + active.
		mkPurchase("unenrolled-1", "camp-active", &received, "", 0),
		mkPurchase("unenrolled-2", "camp-active", &received, "", 0),
		// Empty status but not received → excluded from both buckets (the scenario
		// where a PSA-sheet sync created the row but the card hasn't been scanned).
		mkPurchase("unenrolled-not-received", "camp-active", nil, "", 0),
		// Matched (has inventory ID) → not unenrolled.
		mkPurchase("matched", "camp-active", &received, inventory.DHPushStatusMatched, 42),
	}
	for _, p := range cases {
		require.NoError(t, repo.CreatePurchase(ctx, p))
	}

	// Mark the sold row to exclude it from both buckets.
	require.NoError(t, repo.CreateSale(ctx, &inventory.Sale{
		ID: "sale-unenrolled-2", PurchaseID: "unenrolled-2",
		SaleChannel: inventory.SaleChannelEbay, SalePriceCents: 12000,
		SaleDate: "2026-01-05", CreatedAt: now, UpdatedAt: now,
	}))

	health, err := repo.CountDHPipelineHealth(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, health.PendingReceived, "pending-received-1 + pending-received-2")
	assert.Equal(t, 1, health.UnenrolledReceived, "only unenrolled-1 (unenrolled-2 is sold, unenrolled-not-received has no receipt)")
}

// TestCountDHPipelineHealth_EmptyDatabaseReturnsZero guards the initial state.
func TestCountDHPipelineHealth_EmptyDatabaseReturnsZero(t *testing.T) {
	repo := setupCampaignsRepo(t)
	health, err := repo.CountDHPipelineHealth(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, health.PendingReceived)
	assert.Equal(t, 0, health.UnenrolledReceived)
}

// TestPurchaseStore_UpdatePurchaseDHPriceSync verifies the narrow update that
// the DH price re-sync flow relies on: only dh_listing_price_cents and
// dh_last_synced_at change; every other DH field on the row is preserved.
func TestPurchaseStore_UpdatePurchaseDHPriceSync(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	c := &inventory.Campaign{ID: "camp-sync", Name: "Active", Phase: inventory.PhaseActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	p := &inventory.Purchase{
		ID:                  "pur-sync-1",
		CampaignID:          "camp-sync",
		CardName:             "Charizard",
		CertNumber:          "99887766",
		Grader:              "PSA",
		GradeValue:          9,
		BuyCostCents:        5000,
		PurchaseDate:        "2026-01-01",
		ReviewedPriceCents:  12000,
		DHInventoryID:       777,
		DHListingPriceCents: 10000,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, p))

	syncedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.UpdatePurchaseDHPriceSync(ctx, p.ID, 12000, syncedAt))

	got, err := repo.GetPurchase(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 12000, got.DHListingPriceCents)
	assert.Equal(t, syncedAt.UTC().Format(time.RFC3339), got.DHLastSyncedAt)
	// Sanity: other DH fields untouched.
	assert.Equal(t, 777, got.DHInventoryID)
}
