package httpserver

import (
	"context"
	"encoding/json"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestShowPrepRefreshSocketDeadline(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	var mu sync.Mutex
	var snapshot *sp.Snapshot
	calls := 0
	persistedLive := false
	purchase := sp.Purchase{ID: id, ProfileID: "psa-1", Grader: "PSA", Grade: 10, ListedPriceCents: 30000, Known: true, Exists: true, CampaignExists: true, Phase: "active", Received: true}
	store := &mocks.ShowPrepStoreMock{
		ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
			return map[string]sp.Purchase{id: purchase}, nil
		},
		BeginAttemptFn: func(_ context.Context, i sp.Identity, _ time.Time) (int64, error) {
			mu.Lock()
			defer mu.Unlock()
			snapshot = &sp.Snapshot{Identity: i, Attempt: 1, AttemptState: "running"}
			return 1, nil
		},
		FinishAttemptFn: func(ctx context.Context, _ sp.Identity, _ int64, s sp.Snapshot) error {
			mu.Lock()
			defer mu.Unlock()
			persistedLive = ctx.Err() == nil
			s.AttemptState = "failed"
			snapshot = &s
			return nil
		},
		ReadSnapshotsFn: func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
			mu.Lock()
			defer mu.Unlock()
			return map[sp.Identity]*sp.Snapshot{purchase.Identity(): snapshot}, nil
		},
	}
	source := &mocks.ShowPrepSourceMock{FetchFn: func(ctx context.Context, _ sp.Identity, _ time.Time) (sp.Snapshot, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		<-ctx.Done()
		return sp.Snapshot{}, ctx.Err()
	}}
	logger := mocks.NewMockLogger()
	h := handlers.NewShowPrepHandler(sp.NewService(store, source, time.Now), time.Second, logger)
	router := NewRouter(RouterConfig{ShowPrepHandler: h, LocalAPIToken: "fixture", Logger: logger, SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)})
	server := httptest.NewUnstartedServer(router.Setup())
	server.Config.WriteTimeout = time.Second
	server.Start()
	defer server.Close()
	request, err := http.NewRequest("POST", server.URL+"/api/show-prep/refresh", strings.NewReader(`{"purchaseIds":["`+id+`"]}`))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer fixture")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, 200, response.StatusCode)
	var result struct {
		Evaluations []sp.Evaluation `json:"evaluations"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	require.Len(t, result.Evaluations, 1)
	require.Equal(t, sp.NeedsReview, result.Evaluations[0].Status)
	require.Contains(t, result.Evaluations[0].Reason, "timeout")
	mu.Lock()
	defer mu.Unlock()
	require.True(t, persistedLive)
	require.Equal(t, 1, calls)
	require.Equal(t, "failed", snapshot.AttemptState)
	require.False(t, snapshot.Complete)
}
func TestShowPrepAuthentication(t *testing.T) {
	for _, tt := range []struct {
		name, configured, supplied string
		status                     int
	}{{"disabled OAuth no token", "", "", 401}, {"token required", "fixture", "", 401}, {"wrong token", "fixture", "wrong", 401}, {"local token without OAuth", "fixture", "fixture", 200}} {
		t.Run(tt.name, func(t *testing.T) {
			logger := mocks.NewMockLogger()
			h := handlers.NewShowPrepHandler(sp.NewService(&mocks.ShowPrepStoreMock{}, nil, time.Now), time.Second, logger)
			router := NewRouter(RouterConfig{ShowPrepHandler: h, LocalAPIToken: tt.configured, Logger: logger, SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)})
			r := httptest.NewRequest("GET", "/api/show-prep/lists", nil)
			if tt.supplied != "" {
				r.Header.Set("Authorization", "Bearer "+tt.supplied)
			}
			w := httptest.NewRecorder()
			router.Setup().ServeHTTP(w, r)
			require.Equal(t, tt.status, w.Code)
		})
	}
}
