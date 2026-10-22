//go:build integration

package confirmedreturns_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/httpserver"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

type listingRetryFixture struct {
	db               *postgres.DB
	store            *postgres.ConfirmedReturnStore
	listing          dhlisting.ExplicitListingService
	batch            dhlisting.Service
	patches, syncs   atomic.Int32
	fail             atomic.Bool
	bodies           []string
	api              *httptest.Server
	afterPreparation atomic.Pointer[func()]
	failExecution    atomic.Bool
	blockPatch       atomic.Pointer[chan struct{}]
	patchStarted     chan struct{}
	latePatchDone    chan struct{}
	bodiesMu         sync.Mutex
}

func newListingRetryFixture(t *testing.T, mode string) *listingRetryFixture {
	t.Helper()
	db, store, _, _, id := setupIntegrationReturn(t, "retry-cert")
	logger := mocks.NewMockLogger()
	f := &listingRetryFixture{db: db, store: store, patchStarted: make(chan struct{}, 1), latePatchDone: make(chan struct{})}
	f.fail.Store(true)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PATCH" {
			f.patches.Add(1)
			b, _ := io.ReadAll(r.Body)
			f.bodiesMu.Lock()
			f.bodies = append(f.bodies, string(b))
			f.bodiesMu.Unlock()
			if f.fail.Load() {
				switch mode {
				case "timeout":
					gate := f.blockPatch.Load()
					f.patchStarted <- struct{}{}
					<-*gate
					fmt.Fprint(w, `{"dh_inventory_id":42,"status":"listed","listing_price_cents":25000}`)
					close(f.latePatchDone)
				case "missing receipt":
					fmt.Fprint(w, "{}")
				case "disconnect":
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = conn.Close()
					}
				case "sold":
					w.WriteHeader(422)
					fmt.Fprint(w, `{"error":"Cannot update item with status 'sold'"}`)
				case "not found":
					w.WriteHeader(404)
					fmt.Fprint(w, `{"error":"missing inventory"}`)
				case "validation":
					w.WriteHeader(422)
					fmt.Fprint(w, `{"error":"invalid image"}`)
				default:
					w.WriteHeader(500)
					fmt.Fprint(w, `{"error":"provider failed"}`)
				}
				return
			}
			if gate := f.blockPatch.Load(); gate != nil {
				f.patchStarted <- struct{}{}
				<-*gate
			}
			fmt.Fprint(w, `{"dh_inventory_id":42,"status":"listed","listing_price_cents":25000}`)
		} else if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/sync") {
			f.syncs.Add(1)
			if mode == "sync failure" || mode == "sync failure after retry" {
				w.WriteHeader(500)
				fmt.Fprint(w, `{"error":"sync failed"}`)
				return
			}
			fmt.Fprint(w, `{"dh_inventory_id":42,"status":"listed","channels":[{"name":"ebay","status":"pending"},{"name":"shopify","status":"pending"}]}`)
		} else {
			t.Errorf("unexpected remote call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(provider.Close)
	adapter := dhadapter.NewInventoryAdapter(dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))).WithMutationReceipts()
	purchases := postgres.NewPurchaseStore(db.DB, logger)
	sales := postgres.NewSaleStore(db.DB, logger)
	cfg := postgres.NewDHStore(db.DB, logger)
	scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, id string, fn func(context.Context) error) error {
		if f.failExecution.Swap(false) {
			return errors.New("transient execution failure")
		}
		err := store.WithPurchaseMutation(c, id, fn)
		if err == nil {
			if hook := f.afterPreparation.Swap(nil); hook != nil {
				(*hook)()
			}
		}
		return err
	}}
	listing, err := dhlisting.NewDHListingService(purchases, logger, dhlisting.WithDHListingLister(adapter), dhlisting.WithDHListingFieldsUpdater(purchases), dhlisting.WithDHListingConfigLoader(cfg), dhlisting.WithDHListingMutationCoordinator(inventory.NewDHMutationCoordinator(scope, store), store, store))
	require.NoError(t, err)
	f.listing = listing.(dhlisting.ExplicitListingService)
	f.batch = listing
	svc := inventory.NewService(postgres.NewCampaignStore(db.DB, logger), purchases, sales, nil, postgres.NewFinanceStore(db.DB, logger), nil, cfg, inventory.WithIDGenerator(func() string { return "fixture-id" }), inventory.WithDisableBackgroundWorkers())
	returns := inventory.NewConfirmedReturnService(store, store, adapter, nil, func() string { return "return-op" })
	handler := handlers.NewCampaignsHandler(svc, nil, nil, nil, logger, context.Background(), handlers.WithConfirmedReturnService(returns), handlers.WithDHListingService(listing))
	f.api = httptest.NewServer(httpserver.NewRouter(httpserver.RouterConfig{CampaignsHandler: handler, LocalAPIToken: "local-fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger)}).Setup())
	t.Cleanup(f.api.Close)
	require.Equal(t, "return-p", id)
	return f
}

func (f *listingRetryFixture) state(t *testing.T) *inventory.ConfirmedReturnState {
	t.Helper()
	s, e := f.store.GetReturnState(context.Background(), "return-p")
	require.NoError(t, e)
	return s
}
func (f *listingRetryFixture) listHTTP(t *testing.T) int {
	t.Helper()
	status, _ := f.listHTTPResponse(t)
	return status
}

func (f *listingRetryFixture) listHTTPResponse(t *testing.T) (int, []byte) {
	t.Helper()
	r, e := http.NewRequest("POST", f.api.URL+"/api/purchases/return-p/list-on-dh", nil)
	require.NoError(t, e)
	r.Header.Set("Authorization", "Bearer local-fixture")
	response, e := http.DefaultClient.Do(r)
	require.NoError(t, e)
	defer response.Body.Close()
	body, e := io.ReadAll(response.Body)
	require.NoError(t, e)
	return response.StatusCode, body
}

func TestExplicitListingPatchRetry(t *testing.T) {
	for _, mode := range []string{"server failure", "validation", "disconnect", "missing receipt"} {
		t.Run(mode, func(t *testing.T) {
			f := newListingRetryFixture(t, mode)
			status := 502
			if mode == "validation" {
				status = 422
			}
			require.Equal(t, status, f.listHTTP(t))
			initial := f.state(t).PrecedingAttempt
			require.NotNil(t, initial)
			require.True(t, initial.Retryable)
			require.Equal(t, int32(0), f.syncs.Load())
			// Background callers must not spend an operator's retry.
			result := f.batch.ListPurchases(context.Background(), []string{"retry-cert"})
			require.Zero(t, result.Listed)
			require.Equal(t, int32(1), f.patches.Load())
			require.True(t, f.state(t).PrecedingAttempt.Retryable)
			f.fail.Store(false)
			require.Equal(t, 200, f.listHTTP(t))
			state := f.state(t)
			require.Nil(t, state.PrecedingAttempt)
			require.Equal(t, "listed", state.Purchase.DHStatus)
			require.Equal(t, int32(2), f.patches.Load())
			require.Equal(t, int32(1), f.syncs.Load())
			require.Equal(t, f.bodies[0], f.bodies[1])
			var outcome string
			require.NoError(t, f.db.QueryRow("SELECT outcome FROM dh_mutation_attempts WHERE id=$1", initial.ID).Scan(&outcome))
			require.Equal(t, "succeeded", outcome)
		})
	}
}

func TestListingRetryPreservesHolds(t *testing.T) {
	for _, mode := range []string{"not found", "sold", "sync failure", "changed price", "repeated failure", "returned"} {
		t.Run(mode, func(t *testing.T) {
			f := newListingRetryFixture(t, mode)
			if mode == "sync failure" {
				f.fail.Store(false)
			}
			if mode == "returned" {
				_, e := f.db.Exec(`INSERT INTO confirmed_dh_returns(id,idempotency_key,captured_purchase_id,dh_inventory_id,cert_number,grader,state,completed_at) VALUES('completed-return','key','return-p',42,'retry-cert','PSA','completed',now())`)
				require.NoError(t, e)
				// Use the explicit service so the first authorization is authenticated by
				// this fixture rather than the local-token-only HTTP return authorization.
				result := f.listing.ListPurchaseExplicit(context.Background(), "return-p", "retry-cert", "completed-return")
				require.Zero(t, result.Listed)
			} else {
				require.NotEqual(t, 200, f.listHTTP(t))
			}
			state := f.state(t)
			if mode == "sold" {
				require.Nil(t, state.PrecedingAttempt)
				return
			}
			require.NotNil(t, state.PrecedingAttempt)
			if mode == "sync failure" || mode == "not found" {
				require.False(t, state.PrecedingAttempt.Retryable)
				return
			}
			require.True(t, state.PrecedingAttempt.Retryable)
			if mode == "changed price" {
				_, e := f.db.Exec("UPDATE campaign_purchases SET reviewed_price_cents=30000 WHERE id='return-p'")
				require.NoError(t, e)
				code, body := f.listHTTPResponse(t)
				require.Equal(t, 409, code)
				require.Contains(t, string(body), `"code":"listing_request_changed"`)
				require.True(t, f.state(t).PrecedingAttempt.Retryable)
				_, e = f.db.Exec("UPDATE campaign_purchases SET reviewed_price_cents=25000 WHERE id='return-p'")
				require.NoError(t, e)
			}
			if mode == "repeated failure" {
				require.Equal(t, 502, f.listHTTP(t))
				require.True(t, f.state(t).PrecedingAttempt.Retryable)
			}
			f.fail.Store(false)
			require.Equal(t, 200, f.listHTTP(t))
			require.Nil(t, f.state(t).PrecedingAttempt)
		})
	}
}
