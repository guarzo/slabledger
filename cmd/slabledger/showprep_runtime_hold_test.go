package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cl "github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/google"
	"github.com/guarzo/slabledger/internal/adapters/httpserver"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowPrepRuntimeAuthHoldExplicitResumeWithoutFailedCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, resume     string
		refreshRejection bool
	}{
		{"source rejection/retry", "retry", false},
		{"source rejection/credentials on another instance", "credentials", false},
		{"refresh rejection/retry", "retry", true},
		{"refresh rejection/credentials on another instance", "credentials", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := showRuntimeDB(t)
			ctx := context.Background()
			logger := mocks.NewMockLogger()
			showRuntimeSeed(t, db, "11111111-1111-4111-8111-111111111111", "a-first")
			showRuntimeSeed(t, db, "22222222-2222-4222-8222-222222222222", "z-next")
			var calls, tokenRejections atomic.Int32
			var reject atomic.Bool
			reject.Store(true)
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/accounts:signInWithPassword":
					_ = json.NewEncoder(w).Encode(cl.FirebaseAuthResponse{IDToken: "new-id", RefreshToken: "new-refresh", LocalID: "uid"})
				case "/v1/token":
					if tc.refreshRejection && reject.Load() {
						tokenRejections.Add(1)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{"error":{"code":400,"message":"TOKEN_EXPIRED","errors":[{"domain":"global","reason":"invalid","message":"TOKEN_EXPIRED"}]}}`))
						return
					}
					_ = json.NewEncoder(w).Encode(cl.FirebaseRefreshResponse{IDToken: "current-id", RefreshToken: "rotated", ExpiresIn: "3600"})
				case "/search":
					calls.Add(1)
					if !tc.refreshRejection && reject.Load() {
						http.Error(w, "secret-provider-error", http.StatusUnauthorized)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"hits": []any{}, "totalHits": 0})
				default:
					t.Errorf("unexpected provider route %s", r.URL.Path)
				}
			}))
			defer fixture.Close()
			t.Setenv("CL_SEARCH_URL", fixture.URL+"/search")
			encryptor, err := crypto.NewAESEncryptor(strings.Repeat("fixture-", 5))
			require.NoError(t, err)
			store := postgres.NewCardLadderStore(db.DB, encryptor)
			require.NoError(t, store.SaveConfig(ctx, "fixture", "old-refresh", "collection", "key", "uid"))
			cfg := showRuntimeConfig()
			deps := schedulerDeps{DB: db, Config: &cfg, Logger: logger, CardLadderStore: store, cardLadderAuthOptions: []cl.AuthOption{cl.WithAuthBaseURL(fixture.URL), cl.WithTokenBaseURL(fixture.URL)}}
			result, cancel := initializeSchedulers(ctx, deps)
			t.Cleanup(func() { cancel(); result.Group.StopAll(); result.Group.Wait() })
			require.Eventually(t, func() bool { s, e := result.ShowPrepRefresh.Status(ctx); return e == nil && s.LastSweepAt != "" }, 4*time.Second, 10*time.Millisecond)
			status, err := result.ShowPrepRefresh.Status(ctx)
			require.NoError(t, err)
			require.Equal(t, "auth_hold", status.State, "credential rejection must stop the due fleet")
			var held bool
			require.NoError(t, db.QueryRow(`SELECT auth_hold FROM showprep_worker`).Scan(&held))
			require.True(t, held, "hold must be persisted, not just process-local")
			var initialCalls int32 = 1
			if tc.refreshRejection {
				initialCalls = 0
				require.Equal(t, int32(1), tokenRejections.Load(), "one token rejection, not one per due identity")
			}
			require.Equal(t, initialCalls, calls.Load())
			require.Equal(t, 1, status.FailedIdentities)
			require.Equal(t, 1, status.MissingIdentities)
			_, err = db.Exec(`DELETE FROM campaign_purchases WHERE gem_rate_id IN (SELECT profile_id FROM showprep_evidence WHERE attempt_state='failed')`)
			require.NoError(t, err)
			status, err = result.ShowPrepRefresh.Status(ctx)
			require.NoError(t, err)
			require.Equal(t, 0, status.FailedIdentities)
			require.Equal(t, "auth_hold", status.State)
			// Both an in-client rotation and a stored refresh-token rotation are not
			// an operator repair. Status and normal Run now preserve the durable hold.
			require.NoError(t, store.UpdateRefreshToken(ctx, "rotated-in-storage"))
			handler := buildShowPrepWorkerHandler(handlerInputs{SchedulerResult: result})
			auth := google.NewOAuthService(postgres.NewAuthRepository(db.DB, nil), logger, "", "", "", nil)
			router := httpserver.NewRouter(httpserver.RouterConfig{ShowPrepWorkerHandler: handler, AuthService: auth, LocalAPIToken: "fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)}).Setup()
			request := func(path string) int {
				req := httptest.NewRequest("POST", path, nil)
				req.Header.Set("Authorization", "Bearer fixture")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				return w.Code
			}
			require.Equal(t, 202, request("/api/admin/show-prep/worker/run"))
			require.Eventually(t, func() bool { saved, _ := store.GetConfig(ctx); return saved.RefreshToken == "rotated-in-storage" }, time.Second, time.Millisecond)
			require.NoError(t, result.CardLadderCredentials.Refresh(ctx))
			status, err = result.ShowPrepRefresh.Status(ctx)
			require.NoError(t, err)
			require.Equal(t, "auth_hold", status.State)
			require.Equal(t, initialCalls, calls.Load())
			require.NotContains(t, status.Error, "secret")
			// Even after the fixture is healthy, ordinary run and token rotation
			// cannot resume the held worker. Only explicit operational repair can.
			reject.Store(false)
			require.Equal(t, 202, request("/api/admin/show-prep/worker/run"))
			require.Never(t, func() bool {
				s, e := result.ShowPrepRefresh.Status(ctx)
				return e != nil || s.State != "auth_hold" || calls.Load() != initialCalls
			}, 200*time.Millisecond, 10*time.Millisecond)
			require.NoError(t, db.QueryRow(`SELECT auth_hold FROM showprep_worker`).Scan(&held))
			require.True(t, held)
			if tc.resume == "retry" {
				require.Equal(t, 202, request("/api/admin/show-prep/worker/retry"))
			} else {
				// Save through another instance's REAL handler. Its local loop need not
				// be running for the shared durable epoch/hold reset to take effect.
				build := scheduler.BuildDeps{Logger: logger}
				wireShowPrepScheduler(ctx, deps, &build)
				other := scheduler.BuildGroup(&cfg, build)
				h := handlers.NewCardLadderHandler(store, nil, logger)
				wireShowPrepCredentials(h, &other)
				req := httptest.NewRequest("POST", "/api/admin/cardladder/config", strings.NewReader(`{"email":"fixture@example.test","password":"fixture","collectionId":"collection","firebaseApiKey":"new-key"}`))
				w := httptest.NewRecorder()
				h.HandleSaveConfig(w, req)
				require.Equal(t, 200, w.Code, w.Body.String())
				result.ShowPrepRefresh.Wake() // stand in for the next scheduled minute, not request-owned execution
			}
			require.Eventually(t, func() bool {
				s, e := result.ShowPrepRefresh.Status(ctx)
				return e == nil && s.CurrentIdentities == 1 && s.State == "idle"
			}, 4*time.Second, 10*time.Millisecond)
			require.Equal(t, initialCalls+1, calls.Load())
			if tc.refreshRejection {
				require.Equal(t, int32(1), tokenRejections.Load())
			}
		})
	}
}

func TestShowPrepRuntimeDisabledStillHasCoverage(t *testing.T) {
	db := showRuntimeDB(t)
	logger := mocks.NewMockLogger()
	cfg := showRuntimeConfig()
	cfg.ShowPrepRefresh.Enabled = false
	showRuntimeSeed(t, db, "11111111-1111-4111-8111-111111111111", "")
	result, cancel := initializeSchedulers(context.Background(), schedulerDeps{DB: db, Config: &cfg, Logger: logger})
	defer cancel()
	defer result.Group.Wait()
	defer result.Group.StopAll()
	s, err := result.ShowPrepRefresh.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, "disabled", s.State)
	require.False(t, s.Configured)
	require.Equal(t, 1, s.EligibleCards)
	require.Equal(t, 1, s.UnresolvedCards)
	require.Empty(t, s.LastSweepAt)
	require.NoError(t, result.ShowPrepRefresh.RequestRun(context.Background(), false))
	var requested bool
	require.NoError(t, db.QueryRow(`SELECT requested FROM showprep_worker`).Scan(&requested))
	require.True(t, requested)
}
