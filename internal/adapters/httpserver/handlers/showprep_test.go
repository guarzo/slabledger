package handlers

import (
	"context"
	"encoding/json"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const showTestID = "11111111-1111-4111-8111-111111111111"

func TestShowPrepHTTPContract(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"valid", `{"purchaseIds":["` + showTestID + `"]}`, 200},
		{"missing", `{}`, 400}, {"bad uuid", `{"purchaseIds":["bad"]}`, 400},
		{"unknown field", `{"purchaseIds":["` + showTestID + `"],"price":5}`, 400},
		{"trailing body", `{"purchaseIds":["` + showTestID + `"]} {}`, 400},
		{"empty", `{"purchaseIds":[]}`, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := NewShowPrepHandler(sp.NewService(&mocks.ShowPrepStoreMock{}, nil, time.Now), 90*time.Second, mocks.NewMockLogger())
			r := httptest.NewRequest(http.MethodPost, "/api/show-prep/evaluate", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.HandleEvaluate(w, r)
			require.Equal(t, tt.status, w.Code)
			if tt.status == 200 {
				var body struct {
					Evaluations []map[string]any `json:"evaluations"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Len(t, body.Evaluations, 1)
				for _, key := range []string{"purchaseId", "cardName", "certNumber", "grader", "grade", "status", "reason", "availability", "canAdd", "canPack", "listedPriceCents", "localPriceCents", "priceMismatch", "priceAssociationUnclear", "listingSyncedAt", "medianCents", "compCount", "latestSaleDate", "windowStart", "windowEnd", "refreshedAt", "evidenceVersion", "version"} {
					require.Contains(t, body.Evaluations[0], key)
				}
			}
		})
	}
}
func TestShowPrepHTTPErrorMapping(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{{"missing", sp.ErrNotFound, 404}, {"stale", sp.ErrConflict, 409}, {"invalid", sp.ErrInvalid, 400}, {"storage", context.DeadlineExceeded, 500}} {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.ShowPrepStoreMock{CreateListFn: func(context.Context, string, string) (sp.List, error) { return sp.List{}, tt.err }}
			h := NewShowPrepHandler(sp.NewService(store, nil, time.Now), time.Second, mocks.NewMockLogger())
			r := httptest.NewRequest("POST", "/api/show-prep/lists", strings.NewReader(`{"id":"`+showTestID+`","name":"Show"}`))
			w := httptest.NewRecorder()
			h.HandleCreateList(w, r)
			require.Equal(t, tt.status, w.Code)
		})
	}
}
func TestShowPrepRefreshBudgetIncludesEarlierRouteWork(t *testing.T) {
	calls := 0
	finished := false
	p := sp.Purchase{ID: showTestID, ProfileID: "psa-1", Grader: "PSA", Grade: 10, Known: true, ListedPriceCents: 30000}
	store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
		return map[string]sp.Purchase{showTestID: p}, nil
	}, FinishAttemptFn: func(ctx context.Context, _ sp.Identity, _ int64, s sp.Snapshot) error {
		require.NoError(t, ctx.Err())
		require.False(t, s.Complete)
		require.Contains(t, s.AttemptError, "timeout")
		finished = true
		return nil
	}}
	source := &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) { calls++; return sp.Snapshot{}, nil }}
	h := NewShowPrepHandler(sp.NewService(store, source, time.Now), time.Second, mocks.NewMockLogger())
	wrapped := CaptureShowPrepStart(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(550 * time.Millisecond)
		h.HandleRefresh(w, r)
	}))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, httptest.NewRequest("POST", "/api/show-prep/refresh", strings.NewReader(`{"purchaseIds":["`+showTestID+`"]}`)))
	require.Equal(t, 200, w.Code)
	require.Zero(t, calls, "expired source phase cannot start after authentication/decoding work")
	require.True(t, finished)
}

func TestRefreshDeadlines(t *testing.T) {
	start := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name                                              string
		write, parent, elapsed, source, persist, response time.Duration
	}{
		{"default", 90 * time.Second, 0, 0, 60 * time.Second, 75 * time.Second, 80 * time.Second},
		{"short", time.Second, 0, 0, 500 * time.Millisecond, 750 * time.Millisecond, time.Second},
		{"unlimited", 0, 0, 0, 60 * time.Second, 75 * time.Second, 80 * time.Second},
		{"parent shorter", 90 * time.Second, 2 * time.Second, 0, time.Second, 1500 * time.Millisecond, 2 * time.Second},
		{"expired parent", 90 * time.Second, -time.Second, 0, 0, 0, 0},
		{"decode elapsed", time.Second, 0, 600 * time.Millisecond, 500 * time.Millisecond, 750 * time.Millisecond, time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.parent != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, start.Add(tt.parent))
				defer cancel()
			}
			source, persist, response := refreshDeadlines(start, tt.write, ctx)
			require.Equal(t, start.Add(tt.source), source)
			require.Equal(t, start.Add(tt.persist), persist)
			require.Equal(t, start.Add(tt.response), response)
			if tt.elapsed > 0 {
				require.True(t, source.Before(start.Add(tt.elapsed)), "body time does not grant a fresh source budget")
			}
		})
	}
}
