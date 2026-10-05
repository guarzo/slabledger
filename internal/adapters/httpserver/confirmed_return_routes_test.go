package httpserver

import (
	"context"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfirmedReturnRouterFallbackForwarding(t *testing.T) {
	calls := 0
	logger := mocks.NewMockLogger()
	svc := &mocks.ConfirmedReturnServiceMock{GetReturnStateFn: func(_ context.Context, id string) (*inventory.ConfirmedReturnState, error) {
		calls++
		if id != "p1" {
			t.Fatalf("wrong purchase %s", id)
		}
		return &inventory.ConfirmedReturnState{Outcome: "completed"}, nil
	}}
	rt := NewRouter(RouterConfig{CampaignsService: &mocks.MockInventoryService{}, ConfirmedReturnService: svc, LocalAPIToken: "secret", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger)})
	req := httptest.NewRequest("GET", "/api/purchases/p1/confirmed-return", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	rt.Setup().ServeHTTP(w, req)
	if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), `"completed"`) {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
}
func TestConfirmedReturnRoutesFailClosed(t *testing.T) {
	for _, configured := range []bool{false, true} {
		for _, method := range []string{"GET", "POST"} {
			t.Run(method+map[bool]string{false: " absent", true: " configured"}[configured], func(t *testing.T) {
				rt := setupTestRouter(t)
				if configured {
					rt.authMW = middleware.NewAuthMiddleware(nil, mocks.NewMockLogger()).WithLocalAPIToken("secret")
				}
				path := "/api/purchases/p1/confirmed-return"
				if method == "POST" {
					path = "/api/purchases/p1/confirm-return"
				}
				w := httptest.NewRecorder()
				rt.Setup().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(`{"returnConfirmed":true,"expectedSaleId":null}`)))
				want := 503
				if configured {
					want = 401
				}
				if w.Code != want {
					t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
				}
			})
		}
	}
}
