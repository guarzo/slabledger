package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/google"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowPrepWorkerRoutesAuthorizeWithoutAcquisition(t *testing.T) {
	logger := mocks.NewMockLogger()
	ctx := context.Background()
	auth := google.NewOAuthService(mocks.NewInMemoryAuthRepository(), logger, "", "", "", nil)
	user, err := auth.GetOrCreateUser(ctx, "operator", "operator", "operator@example.test", "")
	require.NoError(t, err)
	session, err := auth.CreateSession(ctx, user.ID, "fixture", "127.0.0.1")
	require.NoError(t, err)
	for _, tc := range []struct {
		method, path    string
		operator, admin int
	}{
		{"GET", "/api/show-prep/coverage", 200, 200},
		{"GET", "/api/admin/show-prep/worker", 403, 200},
		{"POST", "/api/admin/show-prep/worker/run", 403, 202},
		{"POST", "/api/admin/show-prep/worker/retry", 403, 202},
	} {
		for _, role := range []string{"none", "operator", "admin"} {
			t.Run(tc.path+"/"+role, func(t *testing.T) {
				var acquisition, requests atomic.Int32
				retry := false
				store := &mocks.ShowPrepWorkerStoreMock{
					AcquireFn: func(context.Context, string) (sp.WorkerLease, bool, error) {
						acquisition.Add(1)
						return sp.WorkerLease{}, false, nil
					},
					ReadStateFn: func(context.Context) (sp.WorkerState, error) {
						return sp.WorkerState{AuthHold: true, WorkerRunResult: sp.WorkerRunResult{State: "auth_hold"}}, nil
					},
					RequestRunFn: func(_ context.Context, r bool) error { requests.Add(1); retry = r; return nil },
				}
				worker := sp.NewEvidenceWorker(store, func(ctx context.Context) (sp.Source, error) { acquisition.Add(1); <-ctx.Done(); return nil, ctx.Err() }, nil, nil)
				service := scheduler.NewShowPrepRefreshScheduler(worker, nil, logger, true)
				router := NewRouter(RouterConfig{ShowPrepWorkerHandler: handlers.NewShowPrepWorkerHandler(service), AuthService: auth, LocalAPIToken: "fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)}).Setup()
				r := httptest.NewRequest(tc.method, tc.path, nil)
				expected := 401
				if role == "operator" {
					r.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: session.ID})
					expected = tc.operator
				}
				if role == "admin" {
					r.Header.Set("Authorization", "Bearer fixture")
					expected = tc.admin
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				require.Equal(t, expected, w.Code, w.Body.String())
				require.Zero(t, acquisition.Load())
				if expected == 202 {
					require.Equal(t, int32(1), requests.Load())
					require.Equal(t, strings.HasSuffix(tc.path, "retry"), retry)
					require.Contains(t, w.Body.String(), "accepted")
				} else {
					require.Zero(t, requests.Load())
				}
			})
		}
	}
}
