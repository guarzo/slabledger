package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Supplements the full 155-card production-startup proof with an actual source
// adapter at UTC renewal. Only the existing domain clock seam is injected in the
// second Group; production composition itself has no clock/reset hook.
func TestShowPrepRuntimePositiveMidnightAndFinancialWrite(t *testing.T) {
	db := showRuntimeDB(t)
	started := time.Now()
	f := &readinessSourceFixture{now: started.UTC(), started: started, workerDataset: true}
	provider := startReadinessProvider(t, f)
	t.Setenv("CL_SEARCH_URL", provider.URL+"/search")
	showRuntimeSeed(t, db, readinessPurchase(1), "psa-1")
	showRuntimeSeed(t, db, readinessPurchase(31), "cached-31")
	_, err := db.Exec(`UPDATE campaign_purchases SET reviewed_price_cents=30000,dh_listing_price_cents=40000,received_at='2026-09-01'`)
	require.NoError(t, err)
	readinessSaveCredentials(t, db)
	before, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	result, stop := readinessRuntime(t, db, provider.URL, true)
	defer stop()
	waitCurrent := func(worker *scheduler.ShowPrepRefreshScheduler, previousSweep string) {
		require.Eventually(t, func() bool {
			s, e := worker.Status(t.Context())
			return e == nil && s.CurrentIdentities == 2 && s.CurrentCards == 2 && s.State == "idle" && s.LastSweepAt != previousSweep
		}, 5*time.Second, 10*time.Millisecond)
	}
	waitCurrent(result.ShowPrepRefresh, "")
	initialStatus, err := result.ShowPrepRefresh.Status(t.Context())
	require.NoError(t, err)
	stop()
	service := sp.NewService(postgres.NewShowPrepStore(db.DB), nil, f.clock)
	positive, err := service.Evidence(t.Context(), readinessPurchase(1))
	require.NoError(t, err)
	require.Equal(t, sp.Supported, positive.Evaluation.Status)
	require.Equal(t, 30000, positive.Evaluation.LocalPriceCents)
	require.Equal(t, 40000, positive.Evaluation.ListedPriceCents)
	require.Equal(t, sp.PriceAssessmentPolicy, positive.Evaluation.PolicyVersion)
	require.Equal(t, 28000, positive.Evaluation.MedianCents)
	zero, err := service.Evidence(t.Context(), readinessPurchase(31))
	require.NoError(t, err)
	require.Equal(t, sp.NoRecentComps, zero.Evaluation.Status)
	require.Equal(t, sp.ReadinessCurrent, zero.Evaluation.Readiness.State)
	afterPopulation, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	require.Equal(t, before, afterPopulation)

	// A fresh actual production initialization must retain both current identities,
	// including complete zero; no extra Firebase or source request is needed.
	restarted, stopRestart := readinessRuntime(t, db, provider.URL, true)
	defer stopRestart()
	waitCurrent(restarted.ShowPrepRefresh, initialStatus.LastSweepAt)
	stopRestart()
	f.mu.Lock()
	calls, requests := len(f.calls), len(f.providerRequests)
	f.mu.Unlock()
	require.Equal(t, 2, calls)
	require.Equal(t, 3, requests)

	// NEXT midnight, not now+24h: the real adapter stamps actual wall time.
	f.mu.Lock()
	next := f.now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	f.now, f.started = next, time.Now()
	f.mu.Unlock()
	require.Less(t, next.Sub(time.Now().UTC()), 24*time.Hour)
	stale, err := service.Evidence(t.Context(), readinessPurchase(1))
	require.NoError(t, err)
	require.Equal(t, sp.ReadinessStale, stale.Evaluation.Readiness.State)
	require.Len(t, stale.Sales, 2)
	f.setMode("hold")
	worker := sp.NewEvidenceWorker(postgres.NewShowPrepWorkerStore(db.DB), result.CardLadderCredentials.Source, f.clock, nil)
	cfg := showRuntimeConfig()
	group := scheduler.BuildGroup(&cfg, scheduler.BuildDeps{Logger: mocks.NewMockLogger(), ShowPrepWorker: worker, ShowPrepCredentials: result.CardLadderCredentials})
	ctx, cancel := context.WithCancel(t.Context())
	defer func() { cancel(); group.Group.StopAll(); group.Group.Wait() }()
	group.Group.StartAll(ctx)
	require.Eventually(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.calls) == 3 }, 3*time.Second, 10*time.Millisecond)
	status, err := worker.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, "running", status.State)

	// Deliberate financial action in its OWN phase: actual router/auth/service/PG,
	// while the actual source HTTP is held. It must not wait for collection.
	router := readinessRouter(db, f, &group)
	writeCtx, cancelWrite := context.WithTimeout(t.Context(), time.Second)
	defer cancelWrite()
	req := httptest.NewRequest(http.MethodPatch, "/api/purchases/"+readinessPurchase(1)+"/price-override", strings.NewReader(`{"priceCents":32500,"source":"manual"}`)).WithContext(writeCtx)
	req.Header.Set("Authorization", "Bearer "+readinessToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	var override int
	require.NoError(t, db.QueryRow(`SELECT override_price_cents FROM campaign_purchases WHERE id=$1`, readinessPurchase(1)).Scan(&override))
	require.Equal(t, 32500, override)
	afterFinancialWrite, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	require.NotEqual(t, before["campaign_purchases"], afterFinancialWrite["campaign_purchases"])
	for _, table := range []string{"campaigns", "campaign_sales", "cl_sales_comps"} {
		require.Equal(t, before[table], afterFinancialWrite[table])
	}
	f.setMode("complete")
	waitCurrent(group.ShowPrepRefresh, initialStatus.LastSweepAt)
	cancel()
	group.Group.StopAll()
	group.Group.Wait()
	final, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	require.Equal(t, afterFinancialWrite, final, "renewal must not add any financial mutation")
	fresh, err := service.Evidence(t.Context(), readinessPurchase(1))
	require.NoError(t, err)
	require.Equal(t, sp.ReadinessCurrent, fresh.Evaluation.Readiness.State)
	require.Equal(t, sp.BelowTarget, fresh.Evaluation.Status)
	require.Equal(t, 32500, fresh.Evaluation.LocalPriceCents)
	require.Equal(t, 40000, fresh.Evaluation.ListedPriceCents)
	require.Equal(t, sp.PriceAssessmentPolicy, fresh.Evaluation.PolicyVersion)
	require.Equal(t, 28000, fresh.Evaluation.MedianCents)
	require.NotEqual(t, positive.Evaluation.Version, fresh.Evaluation.Version)
	f.mu.Lock()
	calls, requests = len(f.calls), len(f.providerRequests)
	f.mu.Unlock()
	require.Equal(t, 4, calls)
	require.Equal(t, 5, requests, "renewal reuses shared client/token pacing")
	if dir := os.Getenv("SHOW_READINESS_RUNTIME_ARTIFACTS"); dir != "" {
		require.NoError(t, os.MkdirAll(dir, 0750))
		archiveReadiness(t, dir, "runtime-renewal", map[string]any{"before": before, "afterPopulation": afterPopulation,
			"afterIntentionalFinancialWrite": afterFinancialWrite, "afterRenewal": final, "positive": positive, "zero": zero,
			"stale": stale, "fresh": fresh, "sourceCalls": calls, "providerRequests": requests, "nextMidnight": next})
	}
	// Useful output even without opt-in artifact generation during full PG gates.
	summary, err := json.Marshal(map[string]any{"sourceCalls": calls, "providerRequests": requests, "nextMidnight": next, "financialWrite": "isolated override=32500"})
	require.NoError(t, err)
	t.Log(string(summary))
}
