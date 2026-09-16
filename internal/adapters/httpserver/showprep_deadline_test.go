package httpserver

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Refresh socket-budget coverage was retired with browser-owned acquisition.
// Authentication remains a requirement for cached reads and explicit list writes.
func TestShowPrepAuthentication(t *testing.T) {
	for _, tt := range []struct {
		name, configured, supplied string
		status                     int
	}{{"disabled OAuth no token", "", "", 401}, {"token required", "fixture", "", 401}, {"wrong token", "fixture", "wrong", 401}, {"local token without OAuth", "fixture", "fixture", 200}} {
		t.Run(tt.name, func(t *testing.T) {
			logger := mocks.NewMockLogger()
			h := handlers.NewShowPrepHandler(sp.NewService(&mocks.ShowPrepStoreMock{}, nil, time.Now), logger)
			router := NewRouter(RouterConfig{ShowPrepHandler: h, LocalAPIToken: tt.configured, Logger: logger, SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)})
			r := httptest.NewRequest("GET", "/api/show-prep/lists", nil)
			if tt.supplied != "" {
				r.Header.Set("Authorization", "Bearer "+tt.supplied)
			}
			w := httptest.NewRecorder()
			router.Setup().ServeHTTP(w, r)
			require.Equal(t, tt.status, w.Code)
		})
	}
}
