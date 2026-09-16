package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestReadinessCachedSeedAtUTCMidnight(t *testing.T) {
	raw := os.Getenv("SHOW_READINESS_E2E_URL")
	if raw == "" {
		t.Skip("requires the explicitly owned disposable e2e database")
	}
	db := readinessDB(t, raw)
	defer func() { require.NoError(t, db.Close()) }()
	now := time.Date(2026, 9, 16, 0, 0, 30, 0, time.UTC)
	seedReadinessUpgrade(t, db, now)
	seedReadinessCached(t, db, now)
	store := postgres.NewShowPrepStore(db.DB)
	currentID := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	staleID := sp.Identity{ProfileID: "cached-28", Grader: "PSA", Grade: 10}
	snapshots, err := store.ReadSnapshots(t.Context(), []sp.Identity{currentID, staleID})
	require.NoError(t, err)
	require.Equal(t, "2026-08-18", snapshots[currentID].WindowStart)
	require.Equal(t, "2026-09-16", snapshots[currentID].WindowEnd)
	require.Equal(t, "2026-09-14", snapshots[staleID].WindowEnd)
	service := sp.NewService(store, nil, func() time.Time { return now })
	for _, tc := range []struct {
		index  int
		state  sp.ReadinessState
		status sp.Status
	}{
		{1, sp.ReadinessCurrent, sp.Supported},
		{28, sp.ReadinessStale, sp.NeedsReview},
	} {
		evidence, err := service.Evidence(t.Context(), readinessPurchase(tc.index))
		require.NoError(t, err)
		require.Equal(t, tc.state, evidence.Evaluation.Readiness.State)
		require.Equal(t, tc.status, evidence.Evaluation.Status)
		require.Len(t, evidence.Sales, 2)
	}
}

// Shared inventory only: worker mode must never seed verified snapshots.
func seedReadinessInventory(t *testing.T, db *postgres.DB, now time.Time) {
	t.Helper()
	ctx := t.Context()
	var database, owner string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT current_database(),current_user`).Scan(&database, &owner))
	require.Equal(t, "showprep_readiness_e2e", database)
	require.Equal(t, "showprep", owner)
	// 26 unsold existing rows + 129 = 155. The sold historical row is excluded.
	for i := 28; i <= 156; i++ {
		_, err := db.ExecContext(ctx, `INSERT INTO campaign_purchases
		(id,campaign_id,card_name,cert_number,grader,grade_value,purchase_date,received_at,gem_rate_id,
		buy_cost_cents,cl_value_cents,override_price_cents,reviewed_price_cents,dh_card_id,dh_inventory_id,dh_status,dh_push_status,dh_listing_price_cents,dh_channels_json)
		VALUES($1,'readiness-campaign',$2,$3,'PSA',10,$4,$4,$5,18000,31000,29000,40000,$6,$6,'listed','synced',30000,'["ebay"]')`,
			readinessPurchase(i), fmt.Sprintf("Readiness slab %03d", i), fmt.Sprintf("91000%03d", i), now.AddDate(0, 0, -10).Format(time.DateOnly), fmt.Sprintf("cached-%d", i), 1000+i)
		require.NoError(t, err)
	}
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET gem_rate_id='' WHERE id=$1`, readinessPurchase(30))
	require.NoError(t, err)
}

// PRECONDITION for cached use, not worker population. No provider is involved.
func seedReadinessCached(t *testing.T, db *postgres.DB, now time.Time) {
	t.Helper()
	seedReadinessInventory(t, db, now)
	ctx := t.Context()
	store := postgres.NewShowPrepStore(db.DB)
	for i := 1; i <= 156; i++ {
		if i == 25 || i == 27 || i == 30 || (i < 25 && i%2 == 0) {
			continue
		}
		profile := fmt.Sprintf("cached-%d", i)
		if i < 25 {
			profile = fmt.Sprintf("psa-%d", (i+1)/2)
		}
		if i == 26 {
			profile = "no-price"
		}
		identity := sp.Identity{ProfileID: profile, Grader: "PSA", Grade: 10}
		collected := now
		if i == 28 {
			collected = now.AddDate(0, 0, -2)
		}
		start, end := sp.Window(collected)
		sales := []sp.Sale{{ID: profile + "-a", Date: end, PriceCents: 27000, Platform: "eBay", ListingType: "BestOffer"},
			{ID: profile + "-b", Date: end, PriceCents: 29000, Platform: "eBay", ListingType: "Auction"}}
		if i == 31 {
			sales = []sp.Sale{}
		}
		if i == 32 {
			sales = sales[:1]
		}
		if i == 33 {
			sales[0].PriceCents = 20000
			sales[1].PriceCents = 22000
		}
		attempt, err := store.BeginAttempt(ctx, identity, collected)
		require.NoError(t, err)
		require.NoError(t, store.FinishAttempt(ctx, identity, attempt, sp.Snapshot{Identity: identity, Source: "cardladder", Complete: true,
			WindowStart: start, WindowEnd: end, RefreshedAt: collected, Sales: sales}))
		if i == 29 {
			attempt, err = store.BeginAttempt(ctx, identity, now)
			require.NoError(t, err)
			require.NoError(t, store.FinishAttempt(ctx, identity, attempt, sp.Snapshot{AttemptError: "fixture provider unavailable"}))
		}
	}
	service := sp.NewService(store, nil, func() time.Time { return now })
	for _, tc := range []struct {
		index  int
		state  string
		status sp.Status
		sales  int
	}{
		{1, "current", sp.Supported, 2}, {25, "not_checked", sp.NeedsReview, 0}, {26, "current", sp.NoListedPrice, 2},
		{28, "stale", sp.NeedsReview, 2}, {29, "failed", sp.NeedsReview, 2}, {30, "unavailable", sp.NeedsReview, 0},
		{31, "current", sp.NoRecentComps, 0}, {32, "current", sp.ThinEvidence, 1}, {33, "current", sp.BelowTarget, 2},
	} {
		evidence, err := service.Evidence(ctx, readinessPurchase(tc.index))
		require.NoError(t, err)
		require.EqualValues(t, tc.state, evidence.Evaluation.Readiness.State)
		require.Equal(t, tc.status, evidence.Evaluation.Status)
		require.Len(t, evidence.Sales, tc.sales)
	}
}
