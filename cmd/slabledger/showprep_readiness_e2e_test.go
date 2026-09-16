package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	mode := os.Getenv("SHOW_READINESS_MODE")
	require.Contains(t, []string{"cached", "worker"}, mode, "select the explicit cached-precondition or real worker mode")
	t.Chdir("../..") // Match the executable's normal repository-root static asset lookup.
	_, err := os.Stat("web/dist/index.html")
	require.NoError(t, err, "run cd web && npm run build first")
	// Avoid an uncontrolled real midnight during the initial cold/reload assertions.
	// Worker population uses the real one-request/second pace, so leave room
	// for the full fleet before the next real UTC window.
	untilMidnight := time.Until(time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour))
	if os.Getenv("SHOW_READINESS_SERVE") != "1" && untilMidnight < 4*time.Minute {
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
	artifacts := os.Getenv("SHOW_READINESS_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	artifacts, err = filepath.Abs(artifacts)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(artifacts, 0750))
	legacy := seedReadinessUpgrade(t, db, f.clock())
	archiveReadiness(t, artifacts, "upgrade-legacy-45", map[string]any{"version": 45, "ledger": legacy})
	archiveReadiness(t, artifacts, "upgrade-empty-47", readinessProof(t, db, f))
	f.workerDataset = mode == "worker"
	source := startReadinessProvider(t, f)
	t.Setenv("CL_SEARCH_URL", source.URL+"/search")
	if mode == "worker" {
		populateReadinessWorker(t, db, f, source.URL, artifacts)
	} else {
		seedReadinessCached(t, db, f.clock()) // PRECONDITION, not worker population.
	}
	f.setMode("blocked")
	f.mu.Lock()
	sourceBaseline, providerBaseline := len(f.calls), len(f.providerRequests)
	f.mu.Unlock()
	// Disabled production composition also supplies the real coverage/admin API.
	var mu sync.RWMutex
	mu.Lock()
	result, stopRuntime := readinessRuntime(t, db, source.URL, false)
	mu.Unlock()
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		stopRuntime()
	}()
	baseline, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	router := readinessRouter(db, f, result)
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
	cachedCompleted := false
	control := httptest.NewServer(readinessHandler(t.Errorf, func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("Authorization") != "Bearer "+readinessToken {
			http.Error(w, "fixture token required", http.StatusUnauthorized)
			return nil
		}
		if r.Method == "POST" {
			switch r.URL.Path {
			case "/cached-complete":
				f.mu.Lock()
				unchanged := len(f.calls) == sourceBaseline && len(f.providerRequests) == providerBaseline
				f.mu.Unlock()
				if !unchanged {
					return fmt.Errorf("cached operator phase acquired source data")
				}
				proof, err := readReadinessProof(r.Context(), db, f)
				if err != nil {
					return err
				}
				if err := writeReadinessArtifact(artifacts, "cached-operator-complete", proof); err != nil {
					return err
				}
				mu.Lock()
				cachedCompleted = true
				mu.Unlock()
			case "/publication-start":
				mu.RLock()
				ready := mode == "worker" && cachedCompleted
				mu.RUnlock()
				if !ready {
					return fmt.Errorf("publication requires completed worker-mode cached proof")
				}
				// Separate concurrency phase, NEVER attributed to zero-source cached use.
				// Return from the prior stale-window UI test to the real business clock.
				mu.Lock()
				stopRuntime()
				f.mu.Lock()
				f.now = time.Now().UTC()
				f.started = f.now
				f.workerDataset = false
				f.mu.Unlock()
				f.setMode("hold")
				result, stopRuntime = readinessRuntime(t, db, source.URL, true)
				router = readinessRouter(db, f, result)
				mu.Unlock()
			case "/publication-stop":
				mu.Lock()
				stopRuntime()
				f.setMode("blocked")
				mu.Unlock()
			case "/source":
				mode := r.URL.Query().Get("mode")
				if mode != "hold" && mode != "complete" && mode != "failed" && mode != "partial" {
					http.Error(w, "invalid mode", http.StatusBadRequest)
					return nil
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
				closeCurrent := readinessCurrentCloser(&db)
				err := restartReadiness(&mu, func() error {
					stopRuntime() // Join under the lifecycle lock before closing the current pool.
					return closeCurrent()
				}, func() error {
					next, err := openReadinessDB(ctx, raw)
					if err != nil {
						return err
					}
					db = next
					result, stopRuntime = readinessRuntime(t, db, source.URL, false)
					router = readinessRouter(db, f, result)
					return nil
				})
				if err != nil {
					return err
				}
			default:
				http.NotFound(w, r)
				return nil
			}
		} else if r.Method != "GET" || r.URL.Path != "/state" {
			http.NotFound(w, r)
			return nil
		}
		mu.RLock()
		defer mu.RUnlock()
		f.mu.Lock()
		calls := append([]url.Values{}, f.calls...)
		providerRequests := append([]map[string]string{}, f.providerRequests...)
		now := time.Now().UTC().Add(f.now.Sub(f.started))
		f.mu.Unlock()
		requestsMu.Lock()
		requestLog := append([]map[string]string{}, requests...)
		requestsMu.Unlock()
		return serveReadinessState(w, r, db, baseline, map[string]any{"mode": mode, "now": now, "wallNow": time.Now().UTC(), "calls": calls, "providerRequests": providerRequests, "requests": requestLog})
	}))
	defer control.Close()
	t.Logf("real app=%s source=%s control=%s DB=127.0.0.1:44620/showprep_readiness_e2e", app.URL, source.URL, control.URL)
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
	mu.RLock()
	defer mu.RUnlock()
	f.mu.Lock()
	calls, providerRequests := len(f.calls), len(f.providerRequests)
	f.mu.Unlock()
	require.True(t, cachedCompleted, "zero-source operator phase must be archived before concurrent publication")
	if mode == "worker" {
		require.Equal(t, sourceBaseline+1, calls, "separate concurrency phase repairs only the controlled failed identity")
		require.Equal(t, providerBaseline+2, providerRequests, "concurrency phase uses one new runtime token and one source request")
	} else {
		require.Equal(t, sourceBaseline, calls, "seeded-cache mode must remain absolute zero")
		require.Equal(t, providerBaseline, providerRequests)
	}
	finalLedger, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	require.Equal(t, baseline, finalLedger)
	archiveReadiness(t, artifacts, "operator-final", readinessProof(t, db, f))
}
