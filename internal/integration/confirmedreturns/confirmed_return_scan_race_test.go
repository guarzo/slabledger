//go:build integration

package confirmedreturns_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
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

// A provider-PATCH barrier would hold purchase ownership and prevent the check
// from reading the open journal. Pause only after prepare has COMMITTED.
func TestScanDHSaleCheckOpenListingAttempt(t *testing.T) {
	for _, tc := range []struct {
		name, patchError, outcome, receipt string
		patchStatus                        int
		resolvable                         bool
	}{
		{"verified sold guard", "Cannot update item with status 'sold'", "rejected", "dh_patch_sold_guard", 422, true},
		{"unknown provider response", "provider unavailable", "open", "", 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const cert = "scan-sale-check-cert"
			const target = 147840
			db, store, _, _, id := setupIntegrationReturn(t, cert)
			logger := mocks.NewMockLogger()
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=$1,dh_status='in_stock',dh_push_status='matched',reviewed_price_cents=25000 WHERE id=$2`, target, id)
			require.NoError(t, err)

			var patches, returns atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer fixture" {
					t.Errorf("unexpected DH authorization")
					w.WriteHeader(401)
					return
				}
				base := fmt.Sprintf("/api/v1/enterprise/inventory/%d", target)
				switch {
				case r.Method == "GET" && r.URL.Path == "/api/v1/enterprise/inventory":
					if r.URL.Query().Get("cert_number") != cert {
						t.Errorf("DH check used wrong cert: %s", r.URL.RawQuery)
					}
					_, _ = fmt.Fprintf(w, `{"results":[{"dh_inventory_id":%d,"cert_number":%q,"status":"sold"}],"meta":{"total_count":1}}`, target, cert)
				case r.Method == "PATCH" && r.URL.Path == base:
					patches.Add(1)
					w.WriteHeader(tc.patchStatus)
					_, _ = fmt.Fprintf(w, `{"error":%q}`, tc.patchError)
				case r.Method == "POST" && r.URL.Path == base+"/return-to-stock":
					returns.Add(1)
					_, _ = fmt.Fprintf(w, `{"dh_inventory_id":%d,"external_sale_id":443,"item_status":"in_stock","restored":true}`, target)
				default:
					t.Errorf("unexpected provider dispatch %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			t.Cleanup(provider.Close)
			adapter := dhadapter.NewInventoryAdapter(dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))).WithMutationReceipts()
			prepared := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan struct{})
			var releaseOnce sync.Once
			var scopeCalls atomic.Int32
			scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, purchaseID string, fn func(context.Context) error) error {
				err := store.WithPurchaseMutation(c, purchaseID, fn)
				call := scopeCalls.Add(1) // only scan-triggered listing uses this scope
				if call == 1 && err == nil {
					close(prepared)
					<-release
				}
				if call == 2 {
					close(finished)
				}
				return err
			}}
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				select {
				case <-prepared:
					select {
					case <-finished:
					case <-time.After(10 * time.Second):
						t.Error("listing execution did not finish after barrier release")
					}
				default:
				}
			})
			coord := inventory.NewDHMutationCoordinator(scope, store)
			repo := postgres.NewPurchaseStore(db.DB, logger)
			sales := postgres.NewSaleStore(db.DB, logger)
			cfg := postgres.NewDHStore(db.DB, logger)
			inv := inventory.NewService(postgres.NewCampaignStore(db.DB, logger), repo, sales, nil, postgres.NewFinanceStore(db.DB, logger), nil, cfg, inventory.WithIDGenerator(uuid.NewString), inventory.WithDisableBackgroundWorkers())
			returnsSvc := inventory.NewConfirmedReturnService(store, store, adapter, nil, uuid.NewString)
			listing, err := dhlisting.NewDHListingService(repo, logger, dhlisting.WithDHListingLister(adapter), dhlisting.WithDHListingFieldsUpdater(repo), dhlisting.WithDHListingConfigLoader(cfg), dhlisting.WithDHListingMutationCoordinator(coord, store, store))
			require.NoError(t, err)
			listingCtx, cancelListing := context.WithCancel(ctx)
			handler := handlers.NewCampaignsHandler(inv, nil, nil, nil, logger, listingCtx, handlers.WithConfirmedReturnService(returnsSvc), handlers.WithDHListingService(listing))
			api := httptest.NewServer(httpserver.NewRouter(httpserver.RouterConfig{CampaignsHandler: handler, LocalAPIToken: "local-fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger)}).Setup())
			// Release before closing the HTTP server: a timed-out request may still
			// be unwinding on the other side of the preparation barrier.
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				cancelListing()
				api.Close()
			})
			// Unlike returnHTTP, this fixture bounds each request separately so a
			// blocked GET/POST fails and releases the barrier during test cleanup.
			returnHTTP := func(t *testing.T, server *httptest.Server, method, path, body string, authenticated bool) (int, []byte) {
				t.Helper()
				requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(requestCtx, method, server.URL+path, strings.NewReader(body))
				require.NoError(t, err)
				if authenticated {
					req.Header.Set("Authorization", "Bearer local-fixture")
				}
				response, err := server.Client().Do(req)
				require.NoError(t, err)
				defer response.Body.Close()
				raw, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				return response.StatusCode, raw
			}
			path := "/api/purchases/" + id
			body := fmt.Sprintf(`{"returnConfirmed":true,"expectedSaleId":null,"expectedTarget":{"dhInventoryId":%d,"certNumber":%q,"grader":"PSA"}}`, target, cert)

			code, raw := returnHTTP(t, api, "POST", "/api/purchases/scan-cert", fmt.Sprintf(`{"certNumber":%q}`, cert), true)
			require.Equal(t, 200, code, string(raw))
			var scan inventory.ScanCertResult
			require.NoError(t, json.Unmarshal(raw, &scan))
			require.Equal(t, "existing", scan.Status)
			select {
			case <-prepared:
			case <-time.After(10 * time.Second):
				t.Fatal("scan listing did not commit its preparation")
			}
			code, raw = returnHTTP(t, api, "GET", path+"/confirmed-return", "", true)
			require.Equal(t, 200, code, string(raw))
			var state inventory.ConfirmedReturnState
			require.NoError(t, json.Unmarshal(raw, &state))
			require.NotNil(t, state.PrecedingAttempt)
			require.Equal(t, "open", state.PrecedingAttempt.Outcome)
			require.Equal(t, "list", state.PrecedingAttempt.Kind)
			attemptID := state.PrecedingAttempt.ID
			code, raw = returnHTTP(t, api, "GET", path+"/dh-sale-check", "", true)
			require.Equal(t, 200, code, string(raw))
			var check inventory.DHSaleCheck
			require.NoError(t, json.Unmarshal(raw, &check))
			require.False(t, check.Resolvable)
			require.Equal(t, "mutation_pending", check.Reason)
			code, raw = returnHTTP(t, api, "POST", path+"/confirm-return", body, true)
			require.Equal(t, 409, code, string(raw))
			require.Contains(t, string(raw), "preceding_dh_mutation_uncertain")
			require.Zero(t, patches.Load(), "execution has not begun")
			require.Zero(t, returns.Load(), "open listing fences Resolve")
			releaseOnce.Do(func() { close(release) })
			select {
			case <-finished:
			case <-time.After(10 * time.Second):
				t.Fatal("listing execution did not finish")
			}
			require.Eventually(t, func() bool {
				var outcome string
				return db.QueryRowContext(ctx, `SELECT outcome FROM dh_mutation_attempts WHERE id=$1`, attemptID).Scan(&outcome) == nil && outcome == tc.outcome
			}, 5*time.Second, 20*time.Millisecond)
			var outcome, receipt string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT outcome,coalesce(receipt,'') FROM dh_mutation_attempts WHERE id=$1`, attemptID).Scan(&outcome, &receipt))
			require.Equal(t, tc.outcome, outcome)
			require.Equal(t, tc.receipt, receipt)
			code, raw = returnHTTP(t, api, "GET", path+"/dh-sale-check", "", true)
			require.Equal(t, 200, code, string(raw))
			require.NoError(t, json.Unmarshal(raw, &check))
			require.Equal(t, tc.resolvable, check.Resolvable)
			if tc.resolvable {
				require.Equal(t, "sold", check.Status)
				code, raw = returnHTTP(t, api, "POST", path+"/confirm-return", `{"returnConfirmed":true,"expectedSaleId":null}`, true)
				require.Equal(t, 409, code, string(raw))
				require.Contains(t, string(raw), "client_update_required")
				require.Zero(t, returns.Load(), "an old client must not dispatch a return")
				code, raw = returnHTTP(t, api, "POST", path+"/confirm-return", body, true)
				require.Equal(t, 200, code, string(raw))
				require.Contains(t, string(raw), `"outcome":"completed"`)
			} else {
				require.Equal(t, "mutation_pending", check.Reason)
				code, raw = returnHTTP(t, api, "POST", path+"/confirm-return", body, true)
				require.Equal(t, 409, code, string(raw))
				require.Contains(t, string(raw), "preceding_dh_mutation_uncertain")
				// Another real scan must attempt auto-list, but the open journal
				// must prevent a second preparation and provider PATCH.
				code, raw = returnHTTP(t, api, "POST", "/api/purchases/scan-cert", fmt.Sprintf(`{"certNumber":%q}`, cert), true)
				require.Equal(t, 200, code, string(raw))
				require.NoError(t, json.Unmarshal(raw, &scan))
				require.Equal(t, "existing", scan.Status)
				require.Eventually(t, func() bool { return scopeCalls.Load() >= 3 }, 5*time.Second, 20*time.Millisecond, "second scan must reach listing preparation")
				listingDone := make(chan struct{})
				go func() { handler.WaitBackground(); close(listingDone) }()
				select {
				case <-listingDone:
				case <-time.After(5 * time.Second):
					t.Fatal("second scan listing did not finish")
				}
				require.Equal(t, int32(3), scopeCalls.Load(), "second scan must not reach execution")
				var attempts int
				require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM dh_mutation_attempts`).Scan(&attempts))
				require.Equal(t, 1, attempts, "second scan must not prepare a new journal entry")
				saved, err := store.GetReturnState(ctx, id)
				require.NoError(t, err)
				require.NotNil(t, saved.PrecedingAttempt)
				require.Equal(t, attemptID, saved.PrecedingAttempt.ID)
				require.Equal(t, "open", saved.PrecedingAttempt.Outcome)
				require.Zero(t, returns.Load())
			}
			require.Equal(t, int32(1), patches.Load(), "scan must never replay an unkeyed PATCH")
			if tc.resolvable {
				require.Equal(t, int32(1), returns.Load())
			} else {
				require.Zero(t, returns.Load())
			}
		})
	}
}
