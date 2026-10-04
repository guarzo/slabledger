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
	"testing"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/httpserver"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnHTTPBothCardsAcceptance(t *testing.T) {
	for _, tc := range []struct {
		cert             string
		target, external int
		repair           bool
	}{{"160944741", 147840, 443, true}, {"162787413", 364577, 848, false}} {
		t.Run(tc.cert, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, tc.cert)
			ctx := context.Background()
			logger := mocks.NewMockLogger()
			_, e := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=$1,dh_push_status='pending',dh_listing_price_cents=999,dh_channels_json='["ebay"]' WHERE id=$2`, tc.target, id)
			require.NoError(t, e)
			mu := sync.Mutex{}
			repaired := !tc.repair
			remoteStatus := "sold"
			keys := []string{}
			patches, syncs := 0, 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				require.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
				base := fmt.Sprintf("/api/v1/enterprise/inventory/%d", tc.target)
				switch {
				case r.Method == "GET" && r.URL.Path == "/api/v1/enterprise/inventory":
					require.Equal(t, tc.cert, r.URL.Query().Get("cert_number"))
					_, _ = fmt.Fprintf(w, `{"results":[{"dh_inventory_id":%d,"cert_number":%q,"status":%q}],"meta":{"total_count":1}}`, tc.target, tc.cert, remoteStatus)
				case r.Method == "POST" && r.URL.Path == base+"/return-to-stock":
					raw, e := io.ReadAll(r.Body)
					require.NoError(t, e)
					require.JSONEq(t, `{"return_confirmed":true}`, string(raw))
					key := r.Header.Get("Idempotency-Key")
					require.NotEmpty(t, key)
					keys = append(keys, key)
					if !repaired {
						w.WriteHeader(409)
						_, _ = io.WriteString(w, `{"code":"sale_attribution_missing","error":"Original external sale attribution is missing"}`)
						return
					}
					remoteStatus = "in_stock"
					_, _ = fmt.Fprintf(w, `{"dh_inventory_id":%d,"external_sale_id":%d,"item_status":"in_stock","restored":true}`, tc.target, tc.external)
				case r.Method == "PATCH" && r.URL.Path == base:
					patches++
					var body dh.InventoryUpdate
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "listed", body.Status)
					require.NotNil(t, body.ListingPriceCents)
					require.Equal(t, 25000, *body.ListingPriceCents)
					remoteStatus = "listed"
					_, _ = fmt.Fprintf(w, `{"dh_inventory_id":%d,"status":"listed","listing_price_cents":25000}`, tc.target)
				case r.Method == "POST" && r.URL.Path == base+"/sync":
					syncs++
					var body struct {
						Channels []string `json:"channels"`
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, []string{"ebay", "shopify"}, body.Channels)
					_, _ = fmt.Fprintf(w, `{"dh_inventory_id":%d,"status":"listed","channels":[{"name":"ebay","status":"pending"},{"name":"shopify","status":"pending"}]}`, tc.target)
				default:
					t.Errorf("unexpected remote mutation %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer provider.Close()
			client := dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithRateLimitRPS(1000))
			adapter := dhadapter.NewInventoryAdapter(client).WithMutationReceipts()
			coord := inventory.NewDHMutationCoordinator(store, store)
			repo := postgres.NewPurchaseStore(db.DB, logger)
			sales := postgres.NewSaleStore(db.DB, logger)
			cfg := postgres.NewDHStore(db.DB, logger)
			inv := inventory.NewService(postgres.NewCampaignStore(db.DB, logger), repo, sales, nil, postgres.NewFinanceStore(db.DB, logger), nil, cfg, inventory.WithIDGenerator(uuid.NewString), inventory.WithDisableBackgroundWorkers(), inventory.WithDHMutationCoordinator(coord, store, store, store))
			returns := inventory.NewConfirmedReturnService(store, store, adapter, inv.(inventory.ConfirmedUnsellCAS).DeleteSaleByPurchaseIDCAS, uuid.NewString)
			listing, e := dhlisting.NewDHListingService(repo, logger, dhlisting.WithDHListingLister(adapter), dhlisting.WithDHListingFieldsUpdater(repo), dhlisting.WithDHListingConfigLoader(cfg), dhlisting.WithDHListingMutationCoordinator(coord, store, store))
			require.NoError(t, e)
			handler := handlers.NewCampaignsHandler(inv, nil, nil, nil, logger, nil, handlers.WithConfirmedReturnService(returns), handlers.WithDHListingService(listing))
			api := httptest.NewServer(httpserver.NewRouter(httpserver.RouterConfig{CampaignsHandler: handler, LocalAPIToken: "local-fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger)}).Setup())
			defer api.Close()
			path := "/api/purchases/" + id
			for _, endpoint := range []struct{ method, suffix string }{{"POST", "/confirm-return"}, {"GET", "/confirmed-return"}} {
				code, _ := returnHTTP(t, api, endpoint.method, path+endpoint.suffix, `{"returnConfirmed":true,"expectedSaleId":null}`, false)
				require.Equal(t, 401, code)
			}
			var state inventory.ConfirmedReturnState
			body := `{"returnConfirmed":true,"expectedSaleId":null}`
			code, raw := returnHTTP(t, api, "POST", path+"/confirm-return", body, true)
			if tc.repair {
				require.Equal(t, 409, code)
				require.Contains(t, string(raw), "sale_attribution_missing")
				code, raw = returnHTTP(t, api, "GET", path+"/confirmed-return", "", true)
				require.Equal(t, 200, code)
				require.NoError(t, json.Unmarshal(raw, &state))
				require.NotNil(t, state.Operation)
				require.Equal(t, "sale_attribution_missing", state.Operation.LastError.Code)
				mu.Lock()
				repaired = true
				mu.Unlock() // dedicated local fake-DH attribution repair only
				body = fmt.Sprintf(`{"returnConfirmed":true,"expectedSaleId":null,"operationId":%q}`, state.Operation.ID)
				code, raw = returnHTTP(t, api, "POST", path+"/confirm-return", body, true)
			}
			require.Equal(t, 200, code, string(raw))
			require.NoError(t, json.Unmarshal(raw, &state))
			require.Equal(t, "completed", state.Operation.State)
			require.Equal(t, fmt.Sprintf("ext-%d", tc.external), state.Operation.ReturnedOrderID)
			require.True(t, state.AwaitingListing)
			require.Nil(t, state.Sale)
			require.NotContains(t, string(raw), "idempotency")
			require.NotContains(t, string(raw), "PayloadIdentity")
			require.Zero(t, listing.ListPurchases(ctx, []string{tc.cert}).Listed)
			price := dhpricing.NewService(repo, adapter, repo, repo, logger, dhpricing.WithMutationCoordinator(coord, store, store))
			require.Equal(t, dhpricing.OutcomeError, price.SyncPurchasePrice(ctx, id).Outcome)
			code, _ = returnHTTP(t, api, "POST", path+"/list-on-dh", "", false)
			require.Equal(t, 401, code)
			require.NoError(t, cfg.SaveDHPushConfig(ctx, &inventory.DHPushConfig{ListingsPaused: true}))
			code, _ = returnHTTP(t, api, "POST", path+"/list-on-dh", "", true)
			require.Equal(t, 409, code)
			saved, e := returns.GetReturnState(ctx, id)
			require.NoError(t, e)
			require.True(t, saved.AwaitingListing)
			require.NoError(t, cfg.SaveDHPushConfig(ctx, &inventory.DHPushConfig{}))
			_, e = db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=0 WHERE id=$1`, id)
			require.NoError(t, e)
			code, _ = returnHTTP(t, api, "POST", path+"/list-on-dh", "", true)
			require.Equal(t, 409, code)
			saved, e = returns.GetReturnState(ctx, id)
			require.NoError(t, e)
			require.True(t, saved.AwaitingListing)
			_, e = db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=25000 WHERE id=$1`, id)
			require.NoError(t, e)
			at, e := store.ObservationTime(ctx)
			require.NoError(t, e)
			code, raw = returnHTTP(t, api, "POST", path+"/list-on-dh", "", true)
			require.Equal(t, 200, code, string(raw))
			applied, e := store.ApplyDHObservation(ctx, id, tc.target, at, func(c context.Context) error { return repo.UpdatePurchaseDHStatus(c, id, "in_stock") })
			require.NoError(t, e)
			require.False(t, applied)
			code, raw = returnHTTP(t, api, "GET", path+"/confirmed-return", "", true)
			require.Equal(t, 200, code)
			require.NoError(t, json.Unmarshal(raw, &state))
			require.False(t, state.AwaitingListing)
			require.Equal(t, "listed", state.Purchase.DHStatus)
			mu.Lock()
			returnsBeforeReplay := len(keys)
			mu.Unlock()
			code, raw = returnHTTP(t, api, "POST", path+"/confirm-return", body, true)
			require.Equal(t, 200, code)
			require.Contains(t, string(raw), "completed_replay")
			mu.Lock()
			require.Equal(t, 1, patches)
			require.Equal(t, 1, syncs)
			require.Len(t, keys, returnsBeforeReplay, "completed replay must not dispatch")
			if tc.repair {
				// Proven keyed operations retain transport retries. Assert stable
				// authority, not a count affected by the legacy retry classifier.
				require.GreaterOrEqual(t, len(keys), 2)
				for _, key := range keys {
					require.Equal(t, keys[0], key)
				}
			} else {
				require.Len(t, keys, 1)
			}
			mu.Unlock()
			var count int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM campaign_sales`).Scan(&count))
			require.Zero(t, count)
		})
	}
}
func returnHTTP(t *testing.T, server *httptest.Server, method, path, body string, authenticated bool) (int, []byte) {
	t.Helper()
	r, e := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	require.NoError(t, e)
	if authenticated {
		r.Header.Set("Authorization", "Bearer local-fixture")
	}
	response, e := server.Client().Do(r)
	require.NoError(t, e)
	defer response.Body.Close()
	raw, e := io.ReadAll(response.Body)
	require.NoError(t, e)
	return response.StatusCode, raw
}
