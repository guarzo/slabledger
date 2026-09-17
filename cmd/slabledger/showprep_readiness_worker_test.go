package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cl "github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func readinessRuntime(t *testing.T, db *postgres.DB, sourceURL string, enabled bool) (*scheduler.BuildResult, func()) {
	t.Helper()
	encryptor, err := crypto.NewAESEncryptor(strings.Repeat("fixture-", 5))
	require.NoError(t, err)
	store := postgres.NewCardLadderStore(db.DB, encryptor)
	cfg := showRuntimeConfig()
	cfg.ShowPrepRefresh.Enabled = enabled
	logger := mocks.NewMockLogger()
	result, cancel := initializeSchedulers(t.Context(), schedulerDeps{DB: db, Config: &cfg, Logger: logger,
		CardLadderStore: store, PurchaseStore: postgres.NewPurchaseStore(db.DB, logger),
		cardLadderAuthOptions: []cl.AuthOption{cl.WithAuthBaseURL(sourceURL), cl.WithTokenBaseURL(sourceURL)}})
	stop := func() { cancel(); result.Group.StopAll(); result.Group.Wait() }
	return result, stop
}

func readinessSaveCredentials(t *testing.T, db *postgres.DB) {
	t.Helper()
	encryptor, err := crypto.NewAESEncryptor(strings.Repeat("fixture-", 5))
	require.NoError(t, err)
	require.NoError(t, postgres.NewCardLadderStore(db.DB, encryptor).SaveConfig(t.Context(),
		"fixture@example.test", "local-refresh", "unused-local", "local-key", "local-uid"))
}

// Both SDK and sales requests are local, counted even when rejected, and blocked
// during operator use. No static-token bypass of configured-client activation.
func startReadinessProvider(t *testing.T, f *readinessSourceFixture) *httptest.Server {
	t.Helper()
	source := httptest.NewServer(readinessHandler(t.Errorf, func(w http.ResponseWriter, r *http.Request) error {
		if r.URL.Path == "/search" {
			return f.serve(w, r)
		}
		f.mu.Lock()
		f.providerRequests = append(f.providerRequests, map[string]string{"method": r.Method, "path": r.URL.Path})
		blocked := f.mode == "blocked"
		f.mu.Unlock()
		if blocked {
			http.Error(w, "provider access blocked", http.StatusServiceUnavailable)
			return nil
		}
		if r.URL.Path != "/v1/token" || r.Method != "POST" {
			http.NotFound(w, r)
			t.Errorf("unexpected SDK request %s %s", r.Method, r.URL.Path)
			return nil
		}
		return json.NewEncoder(w).Encode(cl.FirebaseRefreshResponse{IDToken: "source-fixture", RefreshToken: "local-rotated", ExpiresIn: "3600"})
	}))
	t.Cleanup(source.Close)
	t.Cleanup(func() { f.setMode("complete") })
	return source
}

func archiveReadiness(t *testing.T, dir, name string, value any) {
	t.Helper()
	require.NoError(t, writeReadinessArtifact(dir, name, value))
}

func writeReadinessArtifact(dir, name string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), body, 0600)
}

func readinessProof(t *testing.T, db *postgres.DB, f *readinessSourceFixture) map[string]any {
	t.Helper()
	proof, err := readReadinessProof(t.Context(), db, f)
	require.NoError(t, err)
	return proof
}

func readReadinessProof(ctx context.Context, db *postgres.DB, f *readinessSourceFixture) (map[string]any, error) {
	ledger, err := readinessLedger(ctx, db)
	if err != nil {
		return nil, err
	}
	status, err := sp.NewEvidenceWorker(postgres.NewShowPrepWorkerStore(db.DB), nil, f.clock, nil).Status(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"ledger": ledger, "coverage": status, "now": f.clock()}
	for _, table := range []string{"showprep_evidence", "showprep_worker", "showprep_lists", "showprep_items"} {
		out[table], err = readinessRows(ctx, db, table)
		if err != nil {
			return nil, err
		}
	}
	f.mu.Lock()
	out["calls"] = append([]map[string]string{}, f.providerRequests...)
	out["sourceQueries"] = append([]url.Values{}, f.calls...)
	f.mu.Unlock()
	return out, nil
}

// Decisive A: this finishes before any browser process is launched. Every
// successful payload comes through actual production runtime/source composition.
func populateReadinessWorker(t *testing.T, db *postgres.DB, f *readinessSourceFixture, sourceURL, artifacts string) {
	t.Helper()
	seedReadinessInventory(t, db, f.clock())
	baseline, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	archiveReadiness(t, artifacts, "worker-before", readinessProof(t, db, f))
	readinessSaveCredentials(t, db)
	coldProbe := os.Getenv("SHOW_READINESS_COLD_PROBE") == "1"
	result, stop := readinessRuntime(t, db, sourceURL, !coldProbe)
	defer stop()
	wait := 4 * time.Minute // 142 identities at the REAL shared one-request/second pace.
	if coldProbe {
		wait = 2 * time.Second
	}
	require.Eventually(t, func() bool {
		s, e := result.ShowPrepRefresh.Status(t.Context())
		return e == nil && s.State == "failed" && s.EligibleIdentities == 142 && s.CurrentIdentities == 141 &&
			s.FailedIdentities == 1 && s.MissingIdentities == 0 && s.StaleIdentities == 0 &&
			s.EligibleCards == 155 && s.CurrentCards == 153 && s.UnresolvedCards == 1
	}, wait, 50*time.Millisecond, "whole resolved cohort must populate without a browser; partial cached-29 and unresolved card30 are deliberate")
	stop() // cancel + stop + join BEFORE blocking the source and before browser creation.
	service := sp.NewService(postgres.NewShowPrepStore(db.DB), nil, f.clock)
	for _, tc := range []struct {
		index  int
		status sp.Status
		sales  int
		asking int
	}{
		{1, sp.Supported, 2, 30000}, {25, sp.Supported, 2, 30000}, {26, sp.NoListedPrice, 2, 0}, {28, sp.Supported, 2, 30000},
		{29, sp.NeedsReview, 2, 30000}, {30, sp.NeedsReview, 0, 30000}, {31, sp.NoRecentComps, 0, 30000},
		{32, sp.ThinEvidence, 1, 30000}, {33, sp.BelowTarget, 2, 30000},
	} {
		e, err := service.Evidence(t.Context(), readinessPurchase(tc.index))
		require.NoError(t, err)
		require.Equal(t, tc.status, e.Evaluation.Status)
		require.Equal(t, tc.asking, e.Evaluation.LocalPriceCents)
		require.Equal(t, 40000, e.Evaluation.ListedPriceCents)
		require.Equal(t, sp.PriceAssessmentPolicy, e.Evaluation.PolicyVersion)
		require.Len(t, e.Sales, tc.sales)
		if tc.index == 1 {
			require.Equal(t, 28000, e.Evaluation.MedianCents)
			require.Equal(t, 28000, e.Evaluation.Recent.MedianCents)
			require.Equal(t, 27000, e.Sales[0].PriceCents)
			require.Equal(t, 29000, e.Sales[1].PriceCents)
		}
	}
	final, err := readinessLedger(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, baseline, final)
	f.mu.Lock()
	calls, requests := len(f.calls), len(f.providerRequests)
	f.mu.Unlock()
	require.Equal(t, 142, calls)
	require.Equal(t, 143, requests, "one real Firebase token exchange plus 142 sales requests")
	expected := map[string]int{"outside": 1, "no-price": 1}
	for i := 1; i <= 12; i++ {
		expected[fmt.Sprintf("psa-%d", i)] = 1
	}
	for i := 28; i <= 156; i++ {
		if i != 30 {
			expected[fmt.Sprintf("cached-%d", i)] = 1
		}
	}
	actual := map[string]int{}
	f.mu.Lock()
	for _, query := range f.calls {
		profile := strings.TrimPrefix(strings.Split(query.Get("filters"), "|")[1], "profileId:")
		actual[profile]++
	}
	f.mu.Unlock()
	require.Equal(t, expected, actual, "each exact resolved identity once, no UI-selected cohort or legacy shortcut")
	archiveReadiness(t, artifacts, "worker-populated", readinessProof(t, db, f))
}
