package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestPriceReviewRealBrowser(t *testing.T) {
	raw := os.Getenv("SHOW_READINESS_E2E_URL")
	if raw == "" {
		t.Skip("requires explicit owned SHOW_READINESS_E2E_URL")
	}
	require.Equal(t, readinessDBURL, raw)
	// This test never inherits worker mode, including when invoked in a package run.
	t.Setenv("SHOW_READINESS_MODE", "cached")
	t.Chdir("../..")
	_, err := os.Stat("web/dist/index.html")
	require.NoError(t, err, "build frontend first")
	artifacts := os.Getenv("SHOW_READINESS_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	artifacts, err = filepath.Abs(artifacts)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(artifacts, 0750))
	db := readinessDB(t, raw)
	defer func() { require.NoError(t, db.Close()) }()
	// Leave room for the cached workflow before a real UTC window rollover.
	untilMidnight := time.Until(time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour))
	if untilMidnight < 2*time.Minute {
		select {
		case <-time.After(untilMidnight + time.Second):
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}
	now := time.Now().UTC()
	f := &readinessSourceFixture{now: now, started: now}
	seedPriceReview(t, db, now)
	source := startReadinessProvider(t, f)
	t.Setenv("CL_SEARCH_URL", source.URL+"/search")
	f.setMode("blocked")
	runtime, stop := readinessRuntime(t, db, source.URL, false)
	defer stop()
	workerStatus, err := runtime.ShowPrepRefresh.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, "disabled", workerStatus.State)
	handler, dhProof, closeDH := priceReviewCampaignHandler(t, db)
	defer closeDH()
	defer handler.WaitBackground()
	router := readinessRouter(db, f, runtime, handler)
	if os.Getenv("PRICE_REVIEW_DH_COLD_PROBE") == "1" {
		// Negative control: reproduce the old nil-collaborator composition.
		// Positive save assertions MUST fail, not quietly accept zero calls.
		router = readinessRouter(db, f, runtime)
	}
	initial, err := priceReviewRows(t.Context(), db)
	require.NoError(t, err)
	archiveReadiness(t, artifacts, "price-review-initial", initial)
	var failedReads atomic.Bool
	var unmarkedMobileReads atomic.Int32
	if os.Getenv("PRICE_REVIEW_UNMARKED_MOBILE_PROBE") == "1" {
		unmarkedMobileReads.Store(1)
	}
	var requestMu sync.Mutex
	requests := []map[string]string{}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requests = append(requests, map[string]string{"method": r.Method, "path": r.URL.Path})
		requestMu.Unlock()
		// Precisely attributed server-side read fault; all other requests reach the
		// real router/auth/service/PG. No browser interception of application APIs.
		if r.Method == "GET" && r.URL.Path == "/api/inventory" && r.Header.Get("X-Price-Review-Browser-Context") == "mobile" && unmarkedMobileReads.CompareAndSwap(1, 0) {
			// Negative control: recover on retry, but never mark this as allowed.
			t.Log("injected one UNMARKED mobile inventory503; retry reaches real router")
			http.Error(w, `{"error":"unmarked mobile failure probe"}`, http.StatusServiceUnavailable)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/api/inventory" && failedReads.Load() {
			w.Header().Set("X-Price-Review-Controlled-Fault", "inventory-read")
			http.Error(w, `{"error":"controlled inventory read failure"}`, http.StatusServiceUnavailable)
			return
		}
		router.ServeHTTP(w, r)
	}))
	defer app.Close()
	var phaseMu sync.Mutex
	phase := "read-only"
	baseline := initial
	var savedSnapshot map[string]string
	readComplete, saveComplete := false, false
	control := httptest.NewServer(readinessHandler(t.Errorf, func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("Authorization") != "Bearer "+readinessToken {
			http.Error(w, "fixture token required", http.StatusUnauthorized)
			return nil
		}
		phaseMu.Lock()
		defer phaseMu.Unlock()
		if r.Method == "POST" {
			switch r.URL.Path {
			case "/read-only-complete":
				rows, err := priceReviewRows(r.Context(), db)
				if err != nil {
					return err
				}
				if phase != "read-only" || !reflect.DeepEqual(initial, rows) {
					return fmt.Errorf("read-only phase changed whole persisted rows")
				}
				dhProof.mu.Lock()
				empty := len(dhProof.wire) == 0 && len(dhProof.syncs) == 0 && len(dhProof.lists) == 0
				dhProof.mu.Unlock()
				if !empty {
					return fmt.Errorf("read-only phase invoked DH save services")
				}
				if err := writeReadinessArtifact(artifacts, "price-review-read-only-complete", rows); err != nil {
					return err
				}
				readComplete = true
				phase = "background"
			case "/background":
				if phase != "background" {
					return fmt.Errorf("background mutation out of phase")
				}
				// A separate actor changes only committed asking; archive separately from
				// the immutable operator phase. This is not an operator navigation write.
				_, err := db.ExecContext(r.Context(), `UPDATE campaign_purchases SET reviewed_price_cents=310000 WHERE id=$1`, readinessPurchase(1))
				if err != nil {
					return err
				}
				baseline, err = priceReviewRows(r.Context(), db)
				if err != nil {
					return err
				}
				if err := validatePriceReviewBackground(initial, baseline); err != nil {
					return err
				}
				if err := writeReadinessArtifact(artifacts, "price-review-controlled-background", baseline); err != nil {
					return err
				}
				phase = "save"
			case "/fail-inventory-read":
				failedReads.Store(true)
			case "/recover-inventory-read":
				failedReads.Store(false)
			case "/save-complete":
				if phase != "save" {
					return fmt.Errorf("save proof out of phase")
				}
				handler.WaitBackground()
				after, err := priceReviewRows(r.Context(), db)
				if err != nil {
					return err
				}
				// Assertions run in the owning test goroutine after the browser returns.
				if err := writeReadinessArtifact(artifacts, "price-review-save-complete", map[string]any{"before": baseline, "after": after, "dh": dhProof.snapshot()}); err != nil {
					return err
				}
				savedSnapshot = after
				saveComplete = true
				phase = "show-actions"
			default:
				return fmt.Errorf("unexpected control command %s", r.URL.Path)
			}
		} else if r.Method != "GET" || r.URL.Path != "/state" {
			http.NotFound(w, r)
			return nil
		}
		f.mu.Lock()
		sourceCalls, providerCalls := len(f.calls), len(f.providerRequests)
		f.mu.Unlock()
		if sourceCalls != 0 || providerCalls != 0 {
			return fmt.Errorf("cached price review acquired provider data: %d/%d", sourceCalls, providerCalls)
		}
		rows, err := priceReviewRows(r.Context(), db)
		if err != nil {
			return err
		}
		requestMu.Lock()
		log := append([]map[string]string{}, requests...)
		requestMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(map[string]any{"mode": "cached", "workerEnabled": false, "phase": phase, "rows": rows, "sourceCalls": sourceCalls, "providerRequests": providerCalls, "dh": dhProof.snapshot(), "requests": log})
	}))
	defer control.Close()
	t.Logf("owned cached app=%s control=%s source=%s artifacts=%s", app.URL, control.URL, source.URL, artifacts)
	command := exec.CommandContext(t.Context(), "node", "web/tests/price-review-real.cjs")
	command.Env = append(os.Environ(), "SHOW_READINESS_APP="+app.URL, "SHOW_READINESS_CONTROL="+control.URL, "SHOW_READINESS_TOKEN="+readinessToken, "SHOW_READINESS_ARTIFACTS="+artifacts)
	output, runErr := command.CombinedOutput()
	t.Logf("browser output:\n%s", output)
	handler.WaitBackground()
	require.Zero(t, unmarkedMobileReads.Load(), "negative probe must reach the mobile application request")
	require.NoError(t, runErr)
	phaseMu.Lock()
	defer phaseMu.Unlock()
	require.True(t, readComplete)
	require.True(t, saveComplete)
	final, err := priceReviewRows(t.Context(), db)
	require.NoError(t, err)
	assertPriceReviewSaved(t, baseline, savedSnapshot, dhProof)
	// The later explicit show commands cannot add other writes.
	for table, rows := range savedSnapshot {
		if table != "showprep_lists" && table != "showprep_items" {
			require.Equal(t, rows, final[table], table)
		}
	}
	service := sp.NewService(postgres.NewShowPrepStore(db.DB), nil, f.clock)
	for i, tt := range []struct {
		name         string
		price        int
		availability sp.Availability
		canPack      bool
	}{
		{name: "declining review save", price: 240000, availability: sp.Ready, canPack: true},
		{name: "supported review save", price: 230000, availability: sp.Ready, canPack: true},
		// The normal Inventory save cannot receive or make this card packable.
		{name: "unreceived inventory save", price: 240000, availability: sp.NotReceived, canPack: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			evidence, err := service.Evidence(t.Context(), readinessPurchase(i+1))
			require.NoError(t, err)
			require.Equal(t, tt.price, evidence.Evaluation.LocalPriceCents)
			require.Equal(t, sp.Supported, evidence.Evaluation.Status)
			require.Equal(t, sp.PriceAssessmentPolicy, evidence.Evaluation.PolicyVersion)
			require.Equal(t, tt.availability, evidence.Evaluation.Availability)
			require.Equal(t, tt.canPack, evidence.Evaluation.CanPack)
		})
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Empty(t, f.calls)
	require.Empty(t, f.providerRequests)
	archiveReadiness(t, artifacts, "price-review-final", map[string]any{"rows": final, "dh": dhProof.snapshot()})
}
