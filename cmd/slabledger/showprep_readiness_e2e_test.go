package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Opt-in only. Run from the repository root after npm run build; see the
// companion browser script and docs/USER_GUIDE.md for the pinned disposable DB.
func TestShowReadinessRealBrowser(t *testing.T) {
	raw := os.Getenv("SHOW_READINESS_E2E_URL")
	if raw == "" {
		t.Skip("set SHOW_READINESS_E2E_URL to the owned disposable e2e database explicitly")
	}
	require.Equal(t, readinessDBURL, raw, "refusing unspecified/developer/production database")
	t.Chdir("../..") // Match the executable's normal repository-root static asset lookup.
	_, err := os.Stat("web/dist/index.html")
	require.NoError(t, err, "run cd web && npm run build first")
	// Avoid an uncontrolled real midnight during the initial cold/reload assertions.
	// This rare wait is under two minutes, within the four-minute fixture timeout.
	untilMidnight := time.Until(time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour))
	if os.Getenv("SHOW_READINESS_SERVE") != "1" && untilMidnight < 2*time.Minute {
		t.Logf("waiting %s for UTC day before count-sensitive flow", untilMidnight)
		select {
		case <-time.After(untilMidnight + time.Second):
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}
	db := readinessDB(t, raw)
	defer func() { require.NoError(t, db.Close()) }()
	// Advance with real elapsed time so an interactive preview remains usable.
	// Only explicit rollover shifts the service/source-fixture clock.
	started := time.Now()
	f := &readinessSourceFixture{now: started.UTC(), started: started}
	seedReadinessUpgrade(t, db, f.clock())
	baseline := readinessLedger(t, db)
	source := startReadinessSource(t, f)
	var mu sync.RWMutex
	router := readinessRouter(db, f, source.URL)
	var requestsMu sync.Mutex
	requests := []map[string]string{}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		requestsMu.Lock()
		requests = append(requests, map[string]string{"method": r.Method, "path": r.URL.Path})
		requestsMu.Unlock()
		router.ServeHTTP(w, r)
	}))
	defer app.Close()
	defer f.setMode("complete") // Release a held local source before draining the API.
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+readinessToken {
			http.Error(w, "fixture token required", http.StatusUnauthorized)
			return
		}
		if r.Method == "POST" {
			switch r.URL.Path {
			case "/source":
				mode := r.URL.Query().Get("mode")
				if mode != "hold" && mode != "complete" && mode != "failed" && mode != "partial" {
					http.Error(w, "invalid mode", http.StatusBadRequest)
					return
				}
				f.setMode(mode)
			case "/rollover":
				f.mu.Lock()
				y, m, d := time.Now().UTC().Add(f.now.Sub(f.started)).Date()
				f.now = time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC)
				f.started = time.Now()
				f.mu.Unlock()
			case "/restart":
				// Drain requests, close the old SQL pool, and build a completely new
				// router, auth/inventory/show service, source client, and store.
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				err := restartReadiness(&mu, db.Close, func() error {
					next, err := openReadinessDB(ctx, raw)
					if err != nil {
						return err
					}
					db = next
					router = readinessRouter(db, f, source.URL)
					return nil
				})
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			default:
				http.NotFound(w, r)
				return
			}
		} else if r.Method != "GET" || r.URL.Path != "/state" {
			http.NotFound(w, r)
			return
		}
		mu.RLock()
		defer mu.RUnlock()
		require.Equal(t, baseline, readinessLedger(t, db), "financial/legacy ledger changed")
		f.mu.Lock()
		calls := append([]url.Values{}, f.calls...)
		now := time.Now().UTC().Add(f.now.Sub(f.started))
		f.mu.Unlock()
		requestsMu.Lock()
		requestLog := append([]map[string]string{}, requests...)
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"now": now, "wallNow": time.Now().UTC(), "calls": calls, "requests": requestLog,
			"evidence": readinessRows(t, db, "showprep_evidence"), "lists": readinessRows(t, db, "showprep_lists"),
			"items": readinessRows(t, db, "showprep_items"), "holds": readinessRows(t, db, "showprep_price_holds"), "ledgerUnchanged": true}))
	}))
	defer control.Close()
	t.Logf("real app=%s source=%s control=%s DB=127.0.0.1:44620/showprep_readiness_e2e", app.URL, source.URL, control.URL)
	artifacts := os.Getenv("SHOW_READINESS_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	artifacts, err = filepath.Abs(artifacts)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(artifacts, 0750))
	metadata, err := json.MarshalIndent(map[string]string{"app": app.URL, "source": source.URL, "control": control.URL, "token": readinessToken, "now": f.clock().Format(time.RFC3339Nano)}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(artifacts, "fixture.json"), metadata, 0600))
	if os.Getenv("SHOW_READINESS_SERVE") == "1" {
		// Separate interactive session, never concurrent with count-sensitive tests.
		// Both browser tabs must send Authorization: Bearer show-readiness-local-fixture.
		t.Logf("interactive fixture ready; token=%s; SIGINT/SIGTERM stops servers; resets e2e DB on launch", readinessToken)
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stop)
		<-stop
		return
	}
	cmd := exec.CommandContext(t.Context(), "node", "web/tests/show-readiness-real.cjs")
	cmd.Env = append(os.Environ(), "SHOW_READINESS_APP="+app.URL, "SHOW_READINESS_CONTROL="+control.URL,
		"SHOW_READINESS_TOKEN="+readinessToken, "SHOW_READINESS_ARTIFACTS="+artifacts)
	output, err := cmd.CombinedOutput()
	t.Logf("browser output:\n%s\nartifacts=%s", output, artifacts)
	require.NoError(t, err)
	require.Equal(t, baseline, readinessLedger(t, db))
}
