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
