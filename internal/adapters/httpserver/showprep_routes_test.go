package httpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// A stale authenticated client must never acquire or mutate evidence through refresh.
func TestShowPrepRefreshRetired(t *testing.T) {
	for _, tc := range []struct {
		name, token, body string
		status            int
	}{
		{"authenticated", "fixture", `{"purchaseIds":["11111111-1111-4111-8111-111111111111"]}`, 410},
		{"malformed stale client", "fixture", `{`, 410},
		{"unauthenticated", "", `{}`, 401},
		{"wrong token", "wrong", `{}`, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, storeAccess := 0, 0
			purchase := sp.Purchase{ID: "11111111-1111-4111-8111-111111111111", ProfileID: "psa-1", Grader: "PSA", Grade: 10,
				Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "active", ListedPriceCents: 30000}
			store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
				storeAccess++
				return map[string]sp.Purchase{purchase.ID: purchase}, nil
			}}
			source := &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) { calls++; return sp.Snapshot{}, nil }}
			logger := mocks.NewMockLogger()
			h := handlers.NewShowPrepHandler(sp.NewService(store, source, time.Now), logger)
			router := NewRouter(RouterConfig{ShowPrepHandler: h, LocalAPIToken: "fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)}).Setup()
			r := httptest.NewRequest("POST", "/api/show-prep/refresh", strings.NewReader(tc.body))
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			require.Zero(t, calls)
			require.Zero(t, storeAccess, "retired route must not touch purchases, evidence or lists")
			if tc.status == 410 {
				require.Contains(t, w.Header().Get("Content-Type"), "application/json")
				require.JSONEq(t, `{"error":"Comp collection is server-managed; reload the application"}`, w.Body.String())
			}
		})
	}
}

func TestShowPrepPreviewAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, configuredToken, suppliedToken string
		composed                             bool
		status                               int
	}{
		{"authenticated preview", "fixture", "fixture", true, 200},
		{"unauthenticated", "fixture", "", true, 401},
		{"wrong token", "fixture", "wrong", true, 401},
		{"no auth configured", "", "", true, 401},
		{"missing composition", "fixture", "fixture", false, 503},
		{"missing composition requires auth", "fixture", "", false, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := mocks.NewMockLogger()
			reads := 0
			const id = "11111111-1111-4111-8111-111111111111"
			store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
				reads++
				return map[string]sp.Purchase{id: {ID: id, LocalPriceCents: 280000}}, nil
			}}
			var h *handlers.ShowPrepHandler
			if tc.composed {
				h = handlers.NewShowPrepHandler(sp.NewService(store, nil, time.Now), logger)
			}
			router := NewRouter(RouterConfig{ShowPrepHandler: h, LocalAPIToken: tc.configuredToken, Logger: logger,
				SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)}).Setup()
			r := httptest.NewRequest("POST", "/api/show-prep/preview", strings.NewReader(`{"purchaseId":"`+id+`","priceCents":240000}`))
			if tc.suppliedToken != "" {
				r.Header.Set("Authorization", "Bearer "+tc.suppliedToken)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == 200 {
				require.Equal(t, 1, reads)
				var result sp.PricePreview
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				require.Equal(t, id, result.PurchaseID)
				require.Equal(t, 240000, result.TrialPriceCents)
			} else {
				require.Zero(t, reads)
			}
		})
	}
}

// Catches approval bypasses, unauthenticated access and disabled-service SPA fallthrough.
func TestShowPrepRetiredAndScanAuthentication(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	for _, route := range []struct {
		name, method, suffix, body string
		status                     int
	}{
		{"scan", "POST", id + "/scan-confirm", `{"commandId":"22222222-2222-4222-8222-222222222222","certNumber":"123","withdrawalGeneration":0}`, 200},
		{"delete list", "DELETE", id, `{"commandId":"22222222-2222-4222-8222-222222222222","expectedState":"stale"}`, 409},
		{"legacy add", "POST", id + "/items", `{"items":[{"purchaseId":"` + id + `","evaluationVersion":"observed"}]}`, 410},
		{"legacy pack", "PUT", id + "/items/" + id, `{"version":1,"packed":true,"evaluationVersion":"observed"}`, 410},
		{"legacy acknowledge", "PUT", id + "/items/" + id, `{"version":1,"acknowledge":true,"evaluationVersion":"observed"}`, 410},
		{"legacy malformed", "PUT", "bad/items/bad", `{`, 410},
		{"legacy malformed add", "POST", "bad/items", `{`, 410},
		{"member removal retained", "DELETE", id + "/items/" + id, "", 200},
		{"transition not public", "POST", id + "/confirm-existing", `{}`, 404},
	} {
		for _, auth := range []struct {
			name, configured, supplied string
			composed                   bool
			status                     int
		}{
			{"authenticated", "fixture", "fixture", true, 0},
			{"unauthenticated", "fixture", "", true, 401},
			{"wrong token", "fixture", "wrong", true, 401},
			{"unconfigured", "", "", true, 401},
			{"disabled authenticated", "fixture", "fixture", false, 503},
			{"disabled unauthenticated", "fixture", "", false, 401},
			{"disabled unconfigured", "", "", false, 401},
		} {
			t.Run(route.name+"/"+auth.name, func(t *testing.T) {
				logger := mocks.NewMockLogger()
				reads, writes := 0, 0
				store := &mocks.ShowPrepStoreMock{
					ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) { reads++; return nil, nil },
					WithinFn: func(context.Context, func(sp.Session) error) error {
						if route.status == 410 || auth.status != 0 {
							t.Fatal("retired or unauthenticated route touched the service")
						}
						return nil
					},
					RemoveItemFn: func(context.Context, string, string) error { writes++; return nil },
					InsertItemFn: func(context.Context, string, sp.Item) error { t.Fatal("legacy Add bypass"); return nil },
					SaveItemFn:   func(context.Context, string, sp.Item) error { t.Fatal("legacy Pack/Acknowledge bypass"); return nil },
				}
				// Use real domain execution for live routes; retired routes prohibit even pre-transaction observations.
				if route.status != 410 && auth.status == 0 {
					store.WithinFn = nil
				} else {
					store.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) {
						t.Fatal("retired or unauthenticated route observed associations")
						return nil, nil
					}
					store.GetItemsFn = func(context.Context, string) ([]sp.Item, error) {
						t.Fatal("retired or unauthenticated route read members")
						return nil, nil
					}
				}
				source := &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) {
					t.Fatal("route acquired comps")
					return sp.Snapshot{}, nil
				}}
				var h *handlers.ShowPrepHandler
				if auth.composed {
					h = handlers.NewShowPrepHandler(sp.NewService(store, source, time.Now), logger)
				}
				router := NewRouter(RouterConfig{ShowPrepHandler: h, LocalAPIToken: auth.configured, Logger: logger,
					SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)}).Setup()
				r := httptest.NewRequest(route.method, "/api/show-prep/lists/"+route.suffix, strings.NewReader(route.body))
				if auth.supplied != "" {
					r.Header.Set("Authorization", "Bearer "+auth.supplied)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				status := auth.status
				if status == 0 {
					status = route.status
				}
				require.Equal(t, status, w.Code, w.Body.String())
				require.Zero(t, reads)
				wantWrites := 0
				if route.name == "member removal retained" && auth.status == 0 {
					wantWrites = 1
				}
				require.Equal(t, wantWrites, writes)
				if status != 401 || auth.configured == "" {
					require.Contains(t, w.Header().Get("Content-Type"), "application/json")
					require.NotContains(t, w.Body.String(), "<!doctype")
				}
				if status == 410 {
					require.Contains(t, w.Body.String(), "scan-confirm")
				}
			})
		}
	}
}

func TestShowPrepAPIPrefixIsolation(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, configuredToken, suppliedToken string
		handlerConfigured                                  bool
		status                                             int
		jsonError                                          bool
	}{
		{"unknown requires auth", "GET", "/api/show-prep/unknown", "fixture", "", true, 401, false},
		{"unknown rejects wrong token", "GET", "/api/show-prep/unknown", "fixture", "wrong", true, 401, false},
		{"authenticated unknown", "GET", "/api/show-prep/unknown", "fixture", "fixture", true, 404, true},
		{"authenticated unknown write", "POST", "/api/show-prep/unknown", "fixture", "fixture", true, 404, true},
		{"authenticated prefix root", "GET", "/api/show-prep/", "fixture", "fixture", true, 404, true},
		{"authenticated nested unknown", "GET", "/api/show-prep/lists/missing/unknown", "fixture", "fixture", true, 404, true},
		{"known route still works", "GET", "/api/show-prep/lists", "fixture", "fixture", true, 200, false},
		{"known route still requires auth", "GET", "/api/show-prep/lists", "fixture", "", true, 401, false},
		{"no auth configuration", "GET", "/api/show-prep/unknown", "", "", true, 401, true},
		{"missing service still unavailable", "GET", "/api/show-prep/unknown", "fixture", "fixture", false, 503, true},
		{"missing service still requires auth", "GET", "/api/show-prep/unknown", "fixture", "", false, 401, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := mocks.NewMockLogger()
			var handler *handlers.ShowPrepHandler
			if tt.handlerConfigured {
				handler = handlers.NewShowPrepHandler(sp.NewService(&mocks.ShowPrepStoreMock{}, nil, time.Now), logger)
			}
			router := NewRouter(RouterConfig{
				ShowPrepHandler: handler,
				LocalAPIToken:   tt.configuredToken,
				Logger:          logger,
				SPAHandler:      handlers.NewSPAHandler(logger),
				HealthHandler:   handlers.NewHealthHandler(nil, nil, logger),
			})
			request := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.suppliedToken != "" {
				request.Header.Set("Authorization", "Bearer "+tt.suppliedToken)
			}
			response := httptest.NewRecorder()
			router.Setup().ServeHTTP(response, request)
			require.Equal(t, tt.status, response.Code)
			if tt.jsonError {
				require.Contains(t, response.Header().Get("Content-Type"), "application/json")
				var body struct {
					Error string `json:"error"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				require.NotEmpty(t, body.Error, "API errors must not fall through to an HTML page")
			}
		})
	}
}
