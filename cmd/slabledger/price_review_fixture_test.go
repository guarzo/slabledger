package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Cached preconditions only. Dates move with the local fixture clock; identities
// are invented. Declining amounts reproduce TestRecentPriceCounterexample.
func seedPriceReview(t *testing.T, db *postgres.DB, now time.Time) {
	t.Helper()
	seedReadinessUpgrade(t, db, now)
	_, err := db.ExecContext(t.Context(), `DELETE FROM campaign_purchases WHERE id > $1 AND id <> $2`, readinessPurchase(7), readinessPurchase(27))
	require.NoError(t, err)
	names := []string{"Declining fixture", "Supported fixture", "Mixed fixture", "Single sale fixture", "Old sale fixture", "Unpriced fixture", "Failed fixture"}
	asks := []int{320000, 240000, 254000, 320000, 240000, 0, 240000}
	store := postgres.NewShowPrepStore(db.DB)
	for n, name := range names {
		i := n + 1
		profile := fmt.Sprintf("price-review-%d", i)
		_, err = db.ExecContext(t.Context(), `UPDATE campaign_purchases SET card_name=$2,gem_rate_id=$3,reviewed_price_cents=$4,override_price_cents=0,dh_listing_price_cents=350000 WHERE id=$1`, readinessPurchase(i), name, profile, asks[n])
		require.NoError(t, err)
		identity := sp.Identity{ProfileID: profile, Grader: "PSA", Grade: 10}
		start, end := sp.Window(now)
		prices := []int{231500, 202500, 309937, 242500, 220000, 233012, 222500, 232500}
		sales := make([]sp.Sale, 0, len(prices)+8)
		for j, cents := range prices {
			date := end
			if j >= 2 {
				date = now.AddDate(0, 0, -1).Format(time.DateOnly)
			}
			sales = append(sales, sp.Sale{ID: fmt.Sprintf("%s-%02d", profile, j), Date: date, PriceCents: cents, Platform: "eBay", ListingType: "Auction"})
		}
		// Higher older context must never overpower the falling recent market.
		for j := 0; j < 8; j++ {
			sales = append(sales, sp.Sale{ID: fmt.Sprintf("%s-old-%d", profile, j), Date: now.AddDate(0, 0, -12).Format(time.DateOnly), PriceCents: 400000, Platform: "eBay", ListingType: "BestOffer"})
		}
		if i == 4 {
			sales = sales[:1]
		}
		if i == 5 {
			sales = sales[8:]
		}
		attempt, err := store.BeginAttempt(t.Context(), identity, now)
		require.NoError(t, err)
		require.NoError(t, store.FinishAttempt(t.Context(), identity, attempt, sp.Snapshot{Identity: identity, Source: "cardladder", Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now, Sales: sales}))
		if i == 7 {
			attempt, err = store.BeginAttempt(t.Context(), identity, now)
			require.NoError(t, err)
			require.NoError(t, store.FinishAttempt(t.Context(), identity, attempt, sp.Snapshot{AttemptError: "controlled cached source failure"}))
		}
	}
	// Case2 already has the target remote preset but is not listed; case3 cannot
	// list because it is neither received nor shipped and has no inventory ID.
	_, err = db.ExecContext(t.Context(), `UPDATE campaign_purchases SET dh_status='in_stock',dh_listing_price_cents=230000,dh_channels_json='' WHERE id=$1`, readinessPurchase(2))
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE campaign_purchases SET received_at=NULL,dh_inventory_id=0,dh_status='',dh_push_status='',dh_listing_price_cents=0 WHERE id=$1`, readinessPurchase(3))
	require.NoError(t, err)
	service := sp.NewService(store, nil, func() time.Time { return now })
	for i, want := range []sp.Status{sp.BelowTarget, sp.Supported, sp.MixedEvidence, sp.ThinEvidence, sp.NoRecentComps, sp.NoListedPrice, sp.NeedsReview} {
		evidence, err := service.Evidence(t.Context(), readinessPurchase(i+1))
		require.NoError(t, err)
		require.Equal(t, want, evidence.Evaluation.Status)
	}
}

type priceReviewDH struct {
	mu                   sync.Mutex
	wire                 []map[string]any
	syncs                map[string]dhpricing.SyncResult
	lists                map[string]dhlisting.DHListingResult
	syncCalls, listCalls int
	done                 map[string]chan struct{}
}

func priceReviewCampaignHandler(t *testing.T, db *postgres.DB) (*handlers.CampaignsHandler, *priceReviewDH, func()) {
	t.Helper()
	log := mocks.NewMockLogger()
	proof := &priceReviewDH{wire: []map[string]any{}, syncs: map[string]dhpricing.SyncResult{}, lists: map[string]dhlisting.DHListingResult{}, done: map[string]chan struct{}{}}
	for i := 1; i <= 3; i++ {
		proof.done[readinessPurchase(i)] = make(chan struct{})
	}
	// Only this external transport is controlled. It cannot write local rows.
	remote := httptest.NewServer(readinessHandler(t.Errorf, func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("Authorization") != "Bearer price-review-local-dh" {
			return fmt.Errorf("unexpected DH credentials")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return err
		}
		proof.mu.Lock()
		proof.wire = append(proof.wire, map[string]any{"method": r.Method, "path": r.URL.Path, "body": body})
		proof.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "PATCH" && r.URL.Path == "/api/v1/enterprise/inventory/1001":
			if body["listing_price_cents"] != float64(240000) || body["status"] != "listed" {
				return fmt.Errorf("unexpected already-listed update: %v", body)
			}
			return json.NewEncoder(w).Encode(dh.InventoryResult{DHInventoryID: 1001, Status: "listed", ListingPriceCents: 240000})
		case r.Method == "PATCH" && r.URL.Path == "/api/v1/enterprise/inventory/1002":
			if body["listing_price_cents"] != float64(230000) || body["status"] != "listed" {
				return fmt.Errorf("unexpected eligible listing: %v", body)
			}
			return json.NewEncoder(w).Encode(dh.InventoryResult{DHInventoryID: 1002, Status: "listed", ListingPriceCents: 230000})
		case r.Method == "POST" && r.URL.Path == "/api/v1/enterprise/inventory/1002/sync":
			if !reflect.DeepEqual(body["channels"], []any{"ebay", "shopify"}) {
				return fmt.Errorf("unexpected channels: %v", body)
			}
			return json.NewEncoder(w).Encode(dh.ChannelSyncResponse{DHInventoryID: 1002, Status: "synced"})
		default:
			return fmt.Errorf("forbidden DH endpoint: %s %s", r.Method, r.URL.Path)
		}
	}))
	client := dh.NewClient(remote.URL, dh.WithEnterpriseKey("price-review-local-dh"), dh.WithLogger(log))
	adapter := dhadapter.NewInventoryAdapter(client)
	purchases := postgres.NewPurchaseStore(db.DB, log)
	campaigns := readinessCampaigns(db)
	syncService := dhpricing.NewService(purchases, adapter, purchases, purchases, log)
	listService, err := dhlisting.NewDHListingService(campaigns, log, dhlisting.WithDHListingLister(adapter), dhlisting.WithDHListingFieldsUpdater(purchases), dhlisting.WithDHListingConfigLoader(postgres.NewDHStore(db.DB, log)))
	require.NoError(t, err)
	syncer := &mocks.MockDHPriceSyncer{SyncPurchasePriceFn: func(ctx context.Context, id string) {
		result := syncService.SyncPurchasePrice(ctx, id)
		proof.mu.Lock()
		proof.syncCalls++
		_, duplicate := proof.syncs[id]
		proof.syncs[id] = result
		gate := proof.done[id]
		proof.mu.Unlock()
		if gate == nil || duplicate {
			t.Errorf("unexpected or duplicate sync purchase %s", id)
			return
		}
		close(gate)
	}}
	listing := &mocks.MockDHListingService{ListPurchasesFn: func(ctx context.Context, certs []string) dhlisting.DHListingResult {
		// Deterministic test schedule only, NOT a production atomicity promise.
		// Join the actual sync before testing the already-in-sync listing branch.
		if len(certs) != 1 {
			t.Errorf("unexpected list certs: %v", certs)
			return dhlisting.DHListingResult{}
		}
		id := ""
		for i := 1; i <= 3; i++ {
			if certs[0] == fmt.Sprintf("910000%02d", i) {
				id = readinessPurchase(i)
			}
		}
		if id == "" {
			t.Errorf("unexpected listing cert: %v", certs)
			return dhlisting.DHListingResult{}
		}
		select {
		case <-proof.done[id]:
		case <-ctx.Done():
			return dhlisting.DHListingResult{Error: ctx.Err()}
		}
		result := listService.ListPurchases(ctx, certs)
		proof.mu.Lock()
		proof.listCalls++
		proof.lists[id] = result
		proof.mu.Unlock()
		return result
	}}
	handler := handlers.NewCampaignsHandler(campaigns, nil, nil, nil, log, t.Context(), handlers.WithDHPriceSyncer(syncer), handlers.WithDHListingService(listing))
	return handler, proof, remote.Close
}

func (p *priceReviewDH) snapshot() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Published entries are immutable; copy containers before concurrent encoding.
	return map[string]any{"wire": slices.Clone(p.wire), "syncs": maps.Clone(p.syncs), "lists": maps.Clone(p.lists),
		"syncCalls": p.syncCalls, "listCalls": p.listCalls}
}

func priceReviewRows(ctx context.Context, db *postgres.DB) (map[string]string, error) {
	result, err := readinessLedger(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, table := range []string{"showprep_lists", "showprep_items", "showprep_price_holds", "showprep_evidence"} {
		var rows string
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM `+table+` t`).Scan(&rows); err != nil {
			return nil, err
		}
		result[table] = rows
	}
	return result, nil
}

func validatePriceReviewBackground(before, after map[string]string) error {
	for table, rows := range before {
		if table != "campaign_purchases" && rows != after[table] {
			return fmt.Errorf("background changed unexpected table %s", table)
		}
	}
	var oldRows, newRows []map[string]any
	if err := json.Unmarshal([]byte(before["campaign_purchases"]), &oldRows); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(after["campaign_purchases"]), &newRows); err != nil {
		return err
	}
	for _, row := range oldRows {
		if row["id"] == readinessPurchase(1) {
			row["reviewed_price_cents"] = float64(310000)
		}
	}
	if !reflect.DeepEqual(oldRows, newRows) {
		return fmt.Errorf("background changed more than the declared committed asking")
	}
	return nil
}

// Compare complete rows, allowing only individually named fields on explicitly
// saved purchases. No collaborator fabricates persisted listing effects.
func assertPriceReviewSaved(t *testing.T, before, after map[string]string, proof *priceReviewDH) {
	t.Helper()
	for table, rows := range before {
		if table != "campaign_purchases" {
			require.Equal(t, rows, after[table], table)
		}
	}
	var oldRows, newRows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(before["campaign_purchases"]), &oldRows))
	require.NoError(t, json.Unmarshal([]byte(after["campaign_purchases"]), &newRows))
	require.Len(t, newRows, len(oldRows))
	for i, old := range oldRows {
		next := newRows[i]
		require.Equal(t, old["id"], next["id"])
		id := old["id"].(string)
		index := 0
		for j := 1; j <= 3; j++ {
			if id == readinessPurchase(j) {
				index = j
			}
		}
		if index == 0 {
			require.Equal(t, old, next)
			continue
		}
		require.Equal(t, float64([]int{240000, 230000, 240000}[index-1]), next["reviewed_price_cents"])
		require.Equal(t, "manual", next["review_source"])
		require.NotEmpty(t, next["reviewed_at"])
		allowed := "reviewed_price_cents reviewed_at review_source updated_at"
		if index == 1 {
			require.Equal(t, float64(240000), next["dh_listing_price_cents"])
			require.NotEmpty(t, next["dh_last_synced_at"])
			allowed += " dh_listing_price_cents dh_last_synced_at"
		}
		if index == 2 {
			require.Equal(t, "listed", next["dh_status"])
			require.Equal(t, "matched", next["dh_cert_status"])
			require.Equal(t, float64(230000), next["dh_listing_price_cents"])
			require.JSONEq(t, `["ebay","shopify"]`, next["dh_channels_json"].(string))
			// persistListed supplies no LastSyncedAt; do not claim it stamps sync time.
			require.Equal(t, "", next["dh_last_synced_at"])
			allowed += " dh_status dh_cert_status dh_channels_json dh_last_synced_at"
		}
		for _, field := range strings.Fields(allowed) {
			delete(old, field)
			delete(next, field)
		}
		require.Equal(t, old, next, "unexpected field mutation on %s", id)
	}
	proof.mu.Lock()
	defer proof.mu.Unlock()
	require.Equal(t, 3, proof.syncCalls)
	require.Equal(t, 3, proof.listCalls)
	require.Len(t, proof.syncs, 3)
	require.Len(t, proof.lists, 3)
	require.Len(t, proof.wire, 3)
	for i, want := range []dhpricing.Outcome{dhpricing.OutcomeSynced, dhpricing.OutcomeSkippedNoDrift, dhpricing.OutcomeSkippedNoInventory} {
		result := proof.syncs[readinessPurchase(i+1)]
		require.NoError(t, result.Err)
		require.Equal(t, want, result.Outcome)
	}
	require.Equal(t, dhlisting.DHListingResult{Synced: 1, Total: 1}, proof.lists[readinessPurchase(1)])
	require.Equal(t, dhlisting.DHListingResult{Listed: 1, Synced: 1, Total: 1}, proof.lists[readinessPurchase(2)])
	ineligible := proof.lists[readinessPurchase(3)]
	require.Equal(t, 1, ineligible.Skipped)
	require.Zero(t, ineligible.Listed)
	require.Zero(t, ineligible.Synced)
	require.EqualError(t, ineligible.FailedCerts["91000003"], "not received or shipped by PSA; cannot list on DH")
}
