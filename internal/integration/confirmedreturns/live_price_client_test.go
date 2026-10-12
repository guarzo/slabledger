//go:build integration

package confirmedreturns_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func livePriceFixtureService(db *postgres.DB, store *postgres.ConfirmedReturnStore, url string) dhpricing.Service {
	fields := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
	client := dh.NewClient(url, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))
	return dhpricing.NewLivePriceService(fields, adapter.NewLivePriceAdapter(client), fields, inventory.NewDHMutationCoordinator(store, store), store, store, mocks.NewMockLogger())
}

func TestLivePriceRestartReplaysABeforeReviewedOrOverrideB(t *testing.T) {
	for _, kind := range []string{"reviewed", "override", "override-only"} {
		t.Run(kind, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, "live-price-cert")
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed',dh_listing_price_cents=10000 WHERE id=$1`, id)
			require.NoError(t, err)
			var mu sync.Mutex
			keys := []string{}
			cents := []int{}
			receipts := map[string]int{}
			current := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "/api/v1/enterprise/inventory/42/live-price", r.URL.Path)
				key := r.Header.Get("Idempotency-Key")
				require.NotEmpty(t, key)
				var body map[string]int
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Len(t, body, 1)
				price := body["listing_price_cents"]
				var persistedKey, payload string
				require.NoError(t, db.QueryRowContext(ctx, `SELECT idempotency_key,payload_identity FROM dh_mutation_attempts WHERE outcome='open'`).Scan(&persistedKey, &payload))
				require.Equal(t, persistedKey, key)
				require.JSONEq(t, fmt.Sprintf(`{"ID":42,"Price":%d}`, price), payload)
				keys = append(keys, key)
				cents = append(cents, price)
				w.Header().Set("Content-Type", "application/json")
				accepted, replay := receipts[key]
				if !replay {
					accepted = price
					receipts[key] = price
					current = price
				}
				if len(keys) == 1 {
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"code":"price_update_failed","error":"commit acknowledgement lost"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"dh_inventory_id": 42, "listing_price_cents": accepted, "replayed": replay})
			}))
			defer server.Close()
			service := livePriceFixtureService(db, store, server.URL)
			first := service.SyncPurchasePrice(ctx, id)
			require.Error(t, first.Err)
			state, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.NotNil(t, state.PrecedingAttempt)
			keyA := state.PrecedingAttempt.Key
			require.NotEmpty(t, keyA)
			switch kind {
			case "reviewed":
				_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=30000 WHERE id=$1`, id)
			case "override":
				_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET override_price_cents=30000,override_set_at='2099-01-01T00:00:00Z' WHERE id=$1`, id)
			case "override-only":
				_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=0,override_price_cents=30000,override_set_at='2099-01-01T00:00:00Z' WHERE id=$1`, id)
			}
			require.NoError(t, err)
			// New service/store models restart: no in-memory request or key survives.
			store = postgres.NewConfirmedReturnStore(db.DB)
			service = livePriceFixtureService(db, store, server.URL)
			batch := service.SyncDriftedPurchases(ctx)
			require.NoError(t, batch.ListErr)
			require.Equal(t, 1, batch.ByOutcome[dhpricing.OutcomeSynced])
			state, err = store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Nil(t, state.PrecedingAttempt)
			require.Equal(t, 25000, state.Purchase.DHListingPriceCents)
			batch = service.SyncDriftedPurchases(ctx)
			require.NoError(t, batch.ListErr)
			require.Equal(t, 1, batch.ByOutcome[dhpricing.OutcomeSynced], "canonical desired B must be discovered")
			mu.Lock()
			require.Equal(t, []int{25000, 25000, 30000}, cents)
			require.Equal(t, keyA, keys[0])
			require.Equal(t, keyA, keys[1])
			require.NotEqual(t, keyA, keys[2])
			require.Equal(t, 30000, current)
			mu.Unlock()
			var n int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM dh_mutation_attempts WHERE outcome='succeeded' AND kind='price' AND phase='live_price'`).Scan(&n))
			require.Equal(t, 2, n)
		})
	}
}

func TestLivePriceCancellationRetainsReceivedCooldown(t *testing.T) {
	db, store, _, _, id := setupIntegrationReturn(t, "cancel-price-cert")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed' WHERE id=$1`, id)
	require.NoError(t, err)
	var deadline = time.Now().Add(time.Hour)
	remote := &mocks.DHLivePriceUpdaterMock{UpdateLivePriceFn: func(context.Context, int, string, int) (*inventory.DHLivePriceResult, error) {
		cancel()
		return nil, &inventory.DHMutationRetryError{NotBefore: deadline, Cause: errors.New("received 429 before cancellation")}
	}}
	fields := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
	svc := dhpricing.NewLivePriceService(fields, remote, fields, inventory.NewDHMutationCoordinator(store, store), store, store, mocks.NewMockLogger())
	require.Error(t, svc.SyncPurchasePrice(ctx, id).Err)
	state, err := store.GetReturnState(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	require.NotNil(t, state.PrecedingAttempt.RetryNotBefore)
	require.WithinDuration(t, deadline, *state.PrecedingAttempt.RetryNotBefore, time.Microsecond)
}

func TestLivePriceCooldownCommitsBeforeExecutionOwnershipRelease(t *testing.T) {
	db, store, _, _, id := setupIntegrationReturn(t, "atomic-cooldown-cert")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed' WHERE id=$1`, id)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("Retry-After", "600"); w.WriteHeader(429) }))
	defer server.Close()
	var scopes atomic.Int32
	observingScope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, id string, fn func(context.Context) error) error {
		err := store.WithPurchaseMutation(c, id, fn)
		if scopes.Add(1) == 2 {
			// This is the first instant another caller can acquire ownership.
			// No detached third transaction may be needed to persist the 429.
			state, readErr := store.GetReturnState(ctx, id)
			require.NoError(t, readErr)
			require.NotNil(t, state.PrecedingAttempt)
			require.NotNil(t, state.PrecedingAttempt.RetryNotBefore, "cooldown must commit before ownership releases")
			require.NoError(t, err, "429 deferral commits rather than rolls back an empty execution")
		}
		return err
	}}
	fields := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
	client := dh.NewClient(server.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))
	svc := dhpricing.NewLivePriceService(fields, adapter.NewLivePriceAdapter(client), fields, inventory.NewDHMutationCoordinator(observingScope, store), store, store, mocks.NewMockLogger())
	var retry *inventory.DHMutationRetryError
	require.ErrorAs(t, svc.SyncPurchasePrice(ctx, id).Err, &retry)
	require.EqualValues(t, 2, scopes.Load(), "must not release ownership for separate cooldown persistence")
}

func TestLivePriceConcurrentPreparationCannotBypassCooldown(t *testing.T) {
	db, store, _, _, id := setupIntegrationReturn(t, "interleaved-price-cert")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed' WHERE id=$1`, id)
	require.NoError(t, err)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(429)
	}))
	defer server.Close()
	prepared := make(chan struct{})
	resume := make(chan struct{})
	var scopes atomic.Int32
	pausedScope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, id string, fn func(context.Context) error) error {
		err := store.WithPurchaseMutation(c, id, fn)
		if scopes.Add(1) == 1 && err == nil {
			close(prepared)
			<-resume
		}
		return err
	}}
	fields := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
	client := dh.NewClient(server.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))
	paused := dhpricing.NewLivePriceService(fields, adapter.NewLivePriceAdapter(client), fields, inventory.NewDHMutationCoordinator(pausedScope, store), store, store, mocks.NewMockLogger())
	completed := make(chan dhpricing.SyncResult, 1)
	go func() { completed <- paused.SyncPurchasePrice(ctx, id) }()
	<-prepared // B has committed A's preparation, but has not acquired execution.
	first := livePriceFixtureService(db, store, server.URL).SyncPurchasePrice(ctx, id)
	close(resume)
	second := <-completed
	var retry *inventory.DHMutationRetryError
	require.ErrorAs(t, first.Err, &retry)
	require.ErrorAs(t, second.Err, &retry)
	require.EqualValues(t, 1, calls.Load(), "B must observe the cooldown persisted after B prepared")
	state, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	require.NotNil(t, state.PrecedingAttempt.RetryNotBefore)
}

func TestLivePriceCooldownSurvivesRestart(t *testing.T) {
	db, store, _, _, id := setupIntegrationReturn(t, "cooldown-cert")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed' WHERE id=$1`, id)
	require.NoError(t, err)
	var mu sync.Mutex
	keys := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		w.Header().Set("Content-Type", "application/json")
		if len(keys) == 1 {
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(`{"dh_inventory_id":42,"listing_price_cents":25000,"replayed":false}`))
	}))
	defer server.Close()
	require.Error(t, livePriceFixtureService(db, store, server.URL).SyncPurchasePrice(ctx, id).Err)
	state, err := store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, state.PrecedingAttempt)
	require.NotNil(t, state.PrecedingAttempt.RetryNotBefore)
	service := livePriceFixtureService(db, postgres.NewConfirmedReturnStore(db.DB), server.URL)
	require.Error(t, service.SyncPurchasePrice(ctx, id).Err)
	mu.Lock()
	require.Len(t, keys, 1)
	mu.Unlock()
	_, err = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET retry_not_before=clock_timestamp()-interval '1 second' WHERE id=$1`, state.PrecedingAttempt.ID)
	require.NoError(t, err)
	require.NoError(t, service.SyncPurchasePrice(ctx, id).Err)
	mu.Lock()
	require.Len(t, keys, 2)
	require.Equal(t, keys[0], keys[1])
	mu.Unlock()
	state, err = store.GetReturnState(ctx, id)
	require.NoError(t, err)
	require.Nil(t, state.PrecedingAttempt)
}
