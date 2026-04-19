package sqlite

import (
	"context"
	"fmt"
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
	now := time.Now().Truncate(time.Second)
	existingSync := "2026-03-01T00:00:00Z"

	tests := []struct {
		name              string
		initialDHSyncedAt string
		newPriceCents     int
		syncedAt          time.Time
		wantPriceCents    int
		wantSyncedAtRFC   string
	}{
		{
			name:              "first sync from empty timestamp",
			initialDHSyncedAt: "",
			newPriceCents:     12000,
			syncedAt:          time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC),
			wantPriceCents:    12000,
			wantSyncedAtRFC:   "2026-04-17T12:00:00Z",
		},
		{
			name:              "overwrites an existing timestamp",
			initialDHSyncedAt: existingSync,
			newPriceCents:     14000,
			syncedAt:          time.Date(2026, 4, 17, 15, 30, 0, 0, time.UTC),
			wantPriceCents:    14000,
			wantSyncedAtRFC:   "2026-04-17T15:30:00Z",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupCampaignsRepo(t)
			ctx := context.Background()

			c := &inventory.Campaign{ID: "camp-sync", Name: "Active", Phase: inventory.PhaseActive, CreatedAt: now, UpdatedAt: now}
			require.NoError(t, repo.CreateCampaign(ctx, c))

			p := &inventory.Purchase{
				ID:                  "pur-sync-1",
				CampaignID:          "camp-sync",
				CardName:            "Charizard",
				CertNumber:          "99887766",
				Grader:              "PSA",
				GradeValue:          9,
				BuyCostCents:        5000,
				PurchaseDate:        "2026-01-01",
				ReviewedPriceCents:  12000,
				DHInventoryID:       777,
				DHListingPriceCents: 10000,
				DHLastSyncedAt:      tc.initialDHSyncedAt,
				CreatedAt:           now,
				UpdatedAt:           now,
			}
			require.NoError(t, repo.CreatePurchase(ctx, p))

			require.NoError(t, repo.UpdatePurchaseDHPriceSync(ctx, p.ID, tc.newPriceCents, tc.syncedAt))

			got, err := repo.GetPurchase(ctx, p.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantPriceCents, got.DHListingPriceCents)
			assert.Equal(t, tc.wantSyncedAtRFC, got.DHLastSyncedAt)
			// Sanity: other DH fields untouched.
			assert.Equal(t, 777, got.DHInventoryID)
		})
	}
}

// TestResetDHFieldsForRepushDueToDelete_SetsTimestamp verifies that the
// DH-delete variant of the repush reset clears the same DH fields as the
// standard reset, preserves reviewed_price_cents, and stamps
// dh_unlisted_detected_at with the current time so the UI can badge the row.
func TestResetDHFieldsForRepushDueToDelete_SetsTimestamp(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	c := &inventory.Campaign{ID: "camp-del", Name: "Active", Phase: inventory.PhaseActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	p := &inventory.Purchase{
		ID:                  "pur-del-1",
		CampaignID:          "camp-del",
		CardName:            "Charizard",
		CertNumber:          "55443322",
		Grader:              "PSA",
		GradeValue:          10,
		BuyCostCents:        60000,
		PurchaseDate:        "2026-01-01",
		ReviewedPriceCents:  9000,
		DHInventoryID:       42,
		DHStatus:            inventory.DHStatusListed,
		DHListingPriceCents: 10000,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, p))

	before := time.Now()
	require.NoError(t, repo.ResetDHFieldsForRepushDueToDelete(ctx, p.ID))

	got, err := repo.GetPurchase(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, got.DHInventoryID)
	assert.Equal(t, inventory.DHStatus(""), got.DHStatus)
	assert.Equal(t, inventory.DHPushStatusPending, got.DHPushStatus)
	assert.Equal(t, 0, got.DHListingPriceCents)
	// Reviewed price must be preserved — repush reuses the prior review.
	assert.Equal(t, 9000, got.ReviewedPriceCents)
	// Timestamp stamped at or after the "before" capture.
	require.NotNil(t, got.DHUnlistedDetectedAt)
	assert.False(t, before.After(*got.DHUnlistedDetectedAt),
		"dh_unlisted_detected_at (%v) should be >= before (%v)", *got.DHUnlistedDetectedAt, before)
}

// TestClearDHUnlistedDetectedAt_Nils verifies the listing service's clear path:
// after a successful re-list, the timestamp is nulled out so the UI badge disappears.
func TestClearDHUnlistedDetectedAt_Nils(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	c := &inventory.Campaign{ID: "camp-clear", Name: "Active", Phase: inventory.PhaseActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	p := &inventory.Purchase{
		ID:            "pur-clear-1",
		CampaignID:    "camp-clear",
		CardName:      "Blastoise",
		CertNumber:    "66554433",
		Grader:        "PSA",
		GradeValue:    9,
		BuyCostCents:  40000,
		PurchaseDate:  "2026-01-01",
		DHInventoryID: 77,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, p))

	require.NoError(t, repo.ResetDHFieldsForRepushDueToDelete(ctx, p.ID))
	// Sanity: timestamp is set before we clear it.
	seeded, err := repo.GetPurchase(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, seeded.DHUnlistedDetectedAt)

	require.NoError(t, repo.ClearDHUnlistedDetectedAt(ctx, p.ID))

	got, err := repo.GetPurchase(ctx, p.ID)
	require.NoError(t, err)
	assert.Nil(t, got.DHUnlistedDetectedAt)
}

// TestPurchaseStore_ListDHPriceDrift verifies the query returns exactly the
// unsold purchases whose reviewed price diverges from dh_listing_price_cents.
// Each case seeds one or more purchases and asserts which IDs are returned.
func TestPurchaseStore_ListDHPriceDrift(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	// purchaseSpec describes a seed purchase with the DH-relevant fields that
	// affect the drift query. The test fills in boilerplate fields from a
	// shared template to keep cases focused on what's being tested.
	type purchaseSpec struct {
		id                  string
		reviewedPriceCents  int
		dhInventoryID       int
		dhListingPriceCents int
		dhPushStatus        string
		sold                bool
	}

	// sellCert increments per test row to avoid UNIQUE constraint collisions.
	nextCert := 11111111

	tests := []struct {
		name    string
		seeds   []purchaseSpec
		wantIDs []string
	}{
		{
			name: "drifted purchase is returned",
			seeds: []purchaseSpec{
				{id: "drift", reviewedPriceCents: 15000, dhInventoryID: 100, dhListingPriceCents: 10000},
			},
			wantIDs: []string{"drift"},
		},
		{
			name: "in-sync reviewed == DH price is excluded",
			seeds: []purchaseSpec{
				{id: "sync", reviewedPriceCents: 10000, dhInventoryID: 101, dhListingPriceCents: 10000},
			},
			wantIDs: nil,
		},
		{
			name: "purchase with no DH inventory ID is excluded",
			seeds: []purchaseSpec{
				{id: "noinv", reviewedPriceCents: 10000, dhInventoryID: 0},
			},
			wantIDs: nil,
		},
		{
			name: "zero reviewed price is excluded",
			seeds: []purchaseSpec{
				{id: "noreview", reviewedPriceCents: 0, dhInventoryID: 102, dhListingPriceCents: 8000},
			},
			wantIDs: nil,
		},
		{
			name: "dismissed push status is excluded",
			seeds: []purchaseSpec{
				{id: "dismissed", reviewedPriceCents: 15000, dhInventoryID: 103, dhListingPriceCents: 10000, dhPushStatus: inventory.DHPushStatusDismissed},
			},
			wantIDs: nil,
		},
		{
			name: "held push status is excluded",
			seeds: []purchaseSpec{
				{id: "held", reviewedPriceCents: 15000, dhInventoryID: 104, dhListingPriceCents: 10000, dhPushStatus: inventory.DHPushStatusHeld},
			},
			wantIDs: nil,
		},
		{
			name: "sold drifted purchase is excluded",
			seeds: []purchaseSpec{
				{id: "sold", reviewedPriceCents: 15000, dhInventoryID: 105, dhListingPriceCents: 10000, sold: true},
			},
			wantIDs: nil,
		},
		{
			name: "mixed pool returns only the drifted row",
			seeds: []purchaseSpec{
				{id: "mix-drift", reviewedPriceCents: 15000, dhInventoryID: 200, dhListingPriceCents: 10000},
				{id: "mix-sync", reviewedPriceCents: 10000, dhInventoryID: 201, dhListingPriceCents: 10000},
				{id: "mix-dismissed", reviewedPriceCents: 15000, dhInventoryID: 202, dhListingPriceCents: 10000, dhPushStatus: inventory.DHPushStatusDismissed},
			},
			wantIDs: []string{"mix-drift"},
		},
		{
			name:    "empty store returns empty slice",
			seeds:   nil,
			wantIDs: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupCampaignsRepo(t)
			ctx := context.Background()

			c := &inventory.Campaign{ID: "camp", Name: "Active", Phase: inventory.PhaseActive, CreatedAt: now, UpdatedAt: now}
			require.NoError(t, repo.CreateCampaign(ctx, c))

			for _, s := range tc.seeds {
				cert := nextCert
				nextCert++
				p := &inventory.Purchase{
					ID:                  s.id,
					CampaignID:          "camp",
					CardName:            "Card-" + s.id,
					CertNumber:          fmt.Sprintf("%08d", cert),
					Grader:              "PSA",
					GradeValue:          9,
					BuyCostCents:        5000,
					PurchaseDate:        "2026-01-01",
					ReviewedPriceCents:  s.reviewedPriceCents,
					DHInventoryID:       s.dhInventoryID,
					DHListingPriceCents: s.dhListingPriceCents,
					DHPushStatus:        s.dhPushStatus,
					CreatedAt:           now,
					UpdatedAt:           now,
				}
				require.NoError(t, repo.CreatePurchase(ctx, p), "seed %s", s.id)
				if s.sold {
					require.NoError(t, repo.CreateSale(ctx, &inventory.Sale{
						ID:             "sale-" + s.id,
						PurchaseID:     s.id,
						SaleChannel:    inventory.SaleChannelEbay,
						SalePriceCents: 16000,
						SaleDate:       "2026-01-10",
						CreatedAt:      now,
						UpdatedAt:      now,
					}))
				}
			}

			got, err := repo.ListDHPriceDrift(ctx)
			require.NoError(t, err)

			gotIDs := make([]string, len(got))
			for i, p := range got {
				gotIDs[i] = p.ID
			}
			assert.ElementsMatch(t, tc.wantIDs, gotIDs)
		})
	}
}
