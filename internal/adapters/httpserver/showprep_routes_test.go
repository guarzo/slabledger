package httpserver

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

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
				handler = handlers.NewShowPrepHandler(sp.NewService(&mocks.ShowPrepStoreMock{}, nil, time.Now), time.Second, logger)
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
