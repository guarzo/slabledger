package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDHSaleCheckRouteFailClosed(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "auth unconfigured", true: "unauthenticated"}[configured], func(t *testing.T) {
			rt := setupTestRouter(t)
			if configured {
				rt.authMW = middleware.NewAuthMiddleware(nil, mocks.NewMockLogger()).WithLocalAPIToken("secret")
			}
			w := httptest.NewRecorder()
			rt.Setup().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/purchases/p1/dh-sale-check", nil))
			if configured {
				require.Equal(t, 401, w.Code)
			} else {
				require.Equal(t, 503, w.Code)
			}
		})
	}
}

func TestDHSaleCheckRouteUsesStoredIdentityOnly(t *testing.T) {
	calls := 0
	logger := mocks.NewMockLogger()
	svc := &mocks.ConfirmedReturnServiceMock{CheckDHSaleFn: func(_ context.Context, id string) (*inventory.DHSaleCheck, error) {
		calls++
		require.Equal(t, "p1", id)
		return &inventory.DHSaleCheck{Status: "sold", Resolvable: true, Target: inventory.ReturnTargetIdentity{DHInventoryID: 147840, CertNumber: "160944741", Grader: "PSA"}}, nil
	}}
	rt := NewRouter(RouterConfig{CampaignsService: &mocks.MockInventoryService{}, ConfirmedReturnService: svc, LocalAPIToken: "secret", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger)})
	for _, tc := range []struct {
		path, body string
		want       int
	}{
		{"/api/purchases/p1/dh-sale-check", "", 200},
		{"/api/purchases/p1/dh-sale-check?dhInventoryId=999", "", 400},
		{"/api/purchases/p1/dh-sale-check", `{"dhInventoryId":999}`, 400},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		rt.Setup().ServeHTTP(w, req)
		require.Equal(t, tc.want, w.Code, w.Body.String())
		if tc.want == 200 {
			require.JSONEq(t, `{"status":"sold","resolvable":true,"reason":"","target":{"dhInventoryId":147840,"certNumber":"160944741","grader":"PSA"}}`, w.Body.String())
		}
	}
	require.Equal(t, 1, calls, "client-supplied target must be rejected before service access")
}

func TestDHSaleCheckRouteUnavailable(t *testing.T) {
	logger := mocks.NewMockLogger()
	rt := NewRouter(RouterConfig{CampaignsService: &mocks.MockInventoryService{}, LocalAPIToken: "secret", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger)})
	req := httptest.NewRequest(http.MethodGet, "/api/purchases/p1/dh-sale-check", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	rt.Setup().ServeHTTP(w, req)
	require.Equal(t, 503, w.Code)
}
