package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func showRuntimeDB(t *testing.T) *postgres.DB {
	t.Helper()
	raw := os.Getenv("SHOW_PREP_RUNTIME_TEST_URL")
	if raw == "" {
		t.Skip("requires explicit owned SHOW_PREP_RUNTIME_TEST_URL")
	}
	require.True(t, safeShowRuntimeTestURL(raw), "refusing non-loopback or non-dedicated runtime database")
	db, err := postgres.Open(context.Background(), raw, mocks.NewMockLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var name, user, owner string
	require.NoError(t, db.QueryRow(`SELECT current_database(),current_user,pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()`).Scan(&name, &user, &owner))
	require.Equal(t, "showprep_runtime_test", name)
	require.Contains(t, []string{"showprep", "slabledger"}, user)
	require.Equal(t, user, owner)
	require.NoError(t, postgres.RunMigrations(db, ""))
	_, err = db.Exec(`TRUNCATE campaigns,showprep_evidence,showprep_worker,cardladder_config,cl_card_mappings CASCADE; INSERT INTO showprep_worker(singleton)VALUES(true); INSERT INTO campaigns(id,name,phase)VALUES('runtime','Runtime','active')`)
	require.NoError(t, err)
	return db
}

func showRuntimeSeed(t *testing.T, db *postgres.DB, id, profile string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,set_name,grader,grade_value,purchase_date,gem_rate_id)VALUES($1,'runtime','Runtime slab',$1,'Base Set','PSA',10,'2026-09-01',$2)`, id, profile)
	require.NoError(t, err)
}

func showRuntimeConfig() config.Config {
	cfg := config.Default()
	cfg.PriceRefresh.Enabled = false
	cfg.SessionCleanup.Enabled = false
	cfg.Maintenance.AccessLogCleanupEnabled = false
	cfg.Maintenance.DHEventCleanupEnabled = false
	cfg.InventoryRefresh.Enabled = false
	cfg.SnapshotEnrich.Enabled = false
	cfg.CardLadder.Enabled = false
	cfg.DHSoldReconciler.Enabled = false
	return cfg
}

func TestShowPrepRuntimeRealStartupFirstSaveAndCrossInstance(t *testing.T) {
	db := showRuntimeDB(t)
	ctx := context.Background()
	logger := mocks.NewMockLogger()
	showRuntimeSeed(t, db, "11111111-1111-4111-8111-111111111111", "verified-profile")
	var searchCalls, tokenCalls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/accounts:signInWithPassword":
			_ = json.NewEncoder(w).Encode(cl.FirebaseAuthResponse{IDToken: "login-id", RefreshToken: "saved-token", LocalID: "uid"})
		case "/v1/token":
			tokenCalls.Add(1)
			_ = json.NewEncoder(w).Encode(cl.FirebaseRefreshResponse{IDToken: "current-id", RefreshToken: "rotated-token", ExpiresIn: "3600"})
		case "/search":
			searchCalls.Add(1)
			require.Equal(t, "Bearer current-id", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{"hits": []any{}, "totalHits": 0})
		default:
			t.Errorf("unexpected provider endpoint %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer source.Close()
	t.Setenv("CL_SEARCH_URL", source.URL+"/search")
	encryptor, err := crypto.NewAESEncryptor(strings.Repeat("fixture-", 5))
	require.NoError(t, err)
	store := postgres.NewCardLadderStore(db.DB, encryptor)
	cfg := showRuntimeConfig()
	deps := schedulerDeps{DB: db, Config: &cfg, Logger: logger, CardLadderStore: store, PurchaseStore: postgres.NewPurchaseStore(db.DB, logger), cardLadderAuthOptions: []cl.AuthOption{cl.WithAuthBaseURL(source.URL), cl.WithTokenBaseURL(source.URL)}}
	result, cancel := initializeSchedulers(ctx, deps)
	t.Cleanup(func() { cancel(); result.Group.StopAll(); result.Group.Wait() })
	require.NotNil(t, result.ShowPrepRefresh)
	require.NotNil(t, result.CardLadderRefresh)
	require.Eventually(t, func() bool {
		s, e := result.ShowPrepRefresh.Status(ctx)
		return e == nil && s.State == "unconfigured" && s.LastSweepAt != ""
	}, 3*time.Second, 10*time.Millisecond)
	require.Zero(t, searchCalls.Load())
	require.Zero(t, tokenCalls.Load())
	// A second instance is constructed while no configuration exists. It must
	// subsequently read the saved row, not retain its initial nil client.
	secondDeps := scheduler.BuildDeps{Logger: logger}
	wireShowPrepScheduler(ctx, deps, &secondDeps)
	second := scheduler.BuildGroup(&cfg, secondDeps)
	h := handlers.NewCardLadderHandler(store, nil, logger)
	wireShowPrepCredentials(h, result)
	h.SetRefresher(result.CardLadderRefresh)
	auth := google.NewOAuthService(postgres.NewAuthRepository(db.DB, nil), logger, "", "", "", nil)
	router := httpserver.NewRouter(httpserver.RouterConfig{CardLadderHandler: h, ShowPrepWorkerHandler: buildShowPrepWorkerHandler(handlerInputs{SchedulerResult: result}), ShowPrepHandler: buildShowPrepHandler(handlerInputs{DB: db, Logger: logger}), AuthService: auth, LocalAPIToken: "fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)}).Setup()
	requestCtx, stopRequest := context.WithCancel(ctx)
	req := httptest.NewRequest("POST", "/api/admin/cardladder/config", strings.NewReader(`{"email":"fixture@example.test","password":"fixture","collectionId":"collection","firebaseApiKey":"key"}`)).WithContext(requestCtx)
	req.Header.Set("Authorization", "Bearer fixture")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	stopRequest()
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Eventually(t, func() bool {
		s, e := result.ShowPrepRefresh.Status(ctx)
		return e == nil && s.CurrentIdentities == 1 && s.State == "idle"
	}, 4*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), searchCalls.Load())
	require.Equal(t, int32(1), tokenCalls.Load())
	client := result.CardLadderCredentials.Current()
	require.NotNil(t, client)
	before, err := result.ShowPrepRefresh.Status(ctx)
	require.NoError(t, err)
	result.ShowPrepRefresh.Wake()
	require.Eventually(t, func() bool {
		s, e := result.ShowPrepRefresh.Status(ctx)
		return e == nil && s.LastSweepAt != before.LastSweepAt
	}, time.Second, time.Millisecond)
	require.Equal(t, int32(1), tokenCalls.Load(), "ordinary sweep must retain rotated token")
	require.Same(t, client, result.CardLadderCredentials.Current())
	// Stop the first loop, add a new identity and start the already-built second
	// instance. Source/worker/store are the REAL production composition.
	result.Group.StopAll()
	result.Group.Wait()
	showRuntimeSeed(t, db, "22222222-2222-4222-8222-222222222222", "another-profile")
	second.Group.StartAll(ctx)
	t.Cleanup(func() { second.Group.StopAll(); second.Group.Wait() })
	require.Eventually(t, func() bool { s, e := second.ShowPrepRefresh.Status(ctx); return e == nil && s.CurrentIdentities == 2 }, 4*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(2), searchCalls.Load())
	require.NotNil(t, second.CardLadderCredentials.Current())
}
