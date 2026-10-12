//go:build integration

package confirmedreturns_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestLivePricePendingReceiptSurvivesEligibilityChanges(t *testing.T) {
	for _, tc := range []struct{ name, change string }{
		{"delisted", `UPDATE campaign_purchases SET dh_status='in_stock' WHERE id='return-p'`},
		{"sale", `UPDATE campaign_purchases SET dh_status='sold' WHERE id='return-p'; INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,created_at,updated_at) VALUES('sale','return-p','local','2026-01-01',now(),now())`},
		{"no drift", `UPDATE campaign_purchases SET dh_listing_price_cents=25000 WHERE id='return-p'`},
		{"zero reviewed", `UPDATE campaign_purchases SET reviewed_price_cents=0 WHERE id='return-p'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, "pending-lifecycle-cert")
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed',dh_listing_price_cents=10000 WHERE id=$1`, id)
			require.NoError(t, err)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if calls.Add(1) == 1 {
					w.WriteHeader(502)
					return
				}
				_, _ = w.Write([]byte(`{"dh_inventory_id":42,"listing_price_cents":25000,"replayed":true}`))
			}))
			defer server.Close()
			require.Error(t, livePriceFixtureService(db, store, server.URL).SyncPurchasePrice(ctx, id).Err)
			_, err = db.ExecContext(ctx, tc.change)
			require.NoError(t, err)
			batch := livePriceFixtureService(db, postgres.NewConfirmedReturnStore(db.DB), server.URL).SyncDriftedPurchases(ctx)
			require.NoError(t, batch.ListErr)
			require.Equal(t, 1, batch.ByOutcome[dhpricing.OutcomeSynced])
			require.EqualValues(t, 2, calls.Load())
			state, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Nil(t, state.PrecedingAttempt)
			if tc.name == "sale" || tc.name == "delisted" {
				require.Equal(t, 10000, state.Purchase.DHListingPriceCents, "historical receipt cannot assert current inventory price")
			}
			if tc.name == "sale" {
				require.NotNil(t, state.Sale)
				require.Equal(t, "sold", state.Purchase.DHStatus)
			}
		})
	}
}

func TestLivePriceLocalCompletionFailureReplaysSameKey(t *testing.T) {
	db, store, _, _, id := setupIntegrationReturn(t, "persist-live-price-cert")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed' WHERE id=$1`, id)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE FUNCTION fail_live_price_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.dh_listing_price_cents=25000 THEN RAISE EXCEPTION 'cache persist unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_live_price_write BEFORE UPDATE ON campaign_purchases FOR EACH ROW EXECUTE FUNCTION fail_live_price_write()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_live_price_write ON campaign_purchases; DROP FUNCTION IF EXISTS fail_live_price_write()`)
	})
	var calls atomic.Int32
	var key atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			key.Store(r.Header.Get("Idempotency-Key"))
		} else {
			require.Equal(t, key.Load(), r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dh_inventory_id":42,"listing_price_cents":25000,"replayed":true}`))
	}))
	defer server.Close()
	require.Error(t, livePriceFixtureService(db, store, server.URL).SyncPurchasePrice(ctx, id).Err)
	state, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	_, err = db.ExecContext(ctx, `DROP TRIGGER fail_live_price_write ON campaign_purchases; DROP FUNCTION fail_live_price_write()`)
	require.NoError(t, err)
	require.NoError(t, livePriceFixtureService(db, postgres.NewConfirmedReturnStore(db.DB), server.URL).SyncPurchasePrice(ctx, id).Err)
	state, err = store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.Nil(t, state.PrecedingAttempt)
	require.EqualValues(t, 2, calls.Load())
}

func TestLivePriceRefusalAndUnknownOutcomeSettlement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		settled bool
	}{
		{"serialized not listed", 409, `{"code":"inventory_not_listed","error":"not listed"}`, true},
		{"serialized stale listing", 409, `{"code":"stale_listing","error":"stale"}`, true},
		{"serialized validation", 422, `{"code":"price_validation_failed","error":"invalid price"}`, true},
		{"key conflict", 409, `{"code":"idempotency_conflict","error":"different payload"}`, false},
		{"ambiguous missing target or vendor", 404, `{"code":"inventory_not_found","error":"missing"}`, false},
		{"route404", 404, `{"error":"Not Found"}`, false},
		{"bad receipt", 200, `{"dh_inventory_id":99,"listing_price_cents":25000,"replayed":true}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, "rejection-cert")
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed' WHERE id=$1`, id)
			require.NoError(t, err)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if calls.Add(1) == 1 {
					w.WriteHeader(503)
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			svc := livePriceFixtureService(db, store, server.URL)
			require.Error(t, svc.SyncPurchasePrice(ctx, id).Err)
			require.Error(t, svc.SyncPurchasePrice(ctx, id).Err)
			state, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			if tc.settled {
				require.Nil(t, state.PrecedingAttempt)
			} else {
				require.NotNil(t, state.PrecedingAttempt)
			}
			require.EqualValues(t, 2, calls.Load())
		})
	}
}

func TestLivePriceOversizedDesiredPriceNeverPrepares(t *testing.T) {
	for _, column := range []string{"reviewed_price_cents", "override_price_cents"} {
		t.Run(column, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, "oversized-price-cert")
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed',`+column+`=10000000000,override_set_at='2099-01-01T00:00:00Z' WHERE id=$1`, id)
			require.NoError(t, err)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"dh_inventory_id":42,"listing_price_cents":25000,"replayed":false}`))
			}))
			defer server.Close()
			svc := livePriceFixtureService(db, store, server.URL)
			require.ErrorIs(t, svc.SyncPurchasePrice(ctx, id).Err, inventory.ErrReturnConflict)
			state, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Nil(t, state.PrecedingAttempt)
			require.Zero(t, calls.Load())
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET `+column+`=25000 WHERE id=$1`, id)
			require.NoError(t, err)
			require.NoError(t, svc.SyncPurchasePrice(ctx, id).Err)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestLivePriceHistoricalUnkeyedAttemptStaysFenced(t *testing.T) {
	db, store, _, _, id := setupIntegrationReturn(t, "legacy-price-cert")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `INSERT INTO dh_mutation_attempts(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,kind,phase,payload_identity) VALUES('old','return-p','return-p',42,'legacy-price-cert','PSA','price','patch','historical')`)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("legacy attempt must not be dispatched") }))
	defer server.Close()
	require.ErrorIs(t, livePriceFixtureService(db, store, server.URL).SyncPurchasePrice(ctx, id).Err, inventory.ErrReturnConflict)
	state, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	require.Empty(t, state.PrecedingAttempt.Key)
	require.Equal(t, "old", state.PrecedingAttempt.ID)
}
