package cardladder

import (
	"context"
	"encoding/json"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestShowPrepSourceCoalescesIdentity(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		_ = json.NewEncoder(w).Encode(map[string]any{"hits": []any{}, "totalHits": 0})
	}))
	defer server.Close()
	client := NewClient(WithBaseURL(server.URL), WithStaticToken("fixture"))
	client.rateLimiter = rate.NewLimiter(rate.Inf, 1)
	source := NewShowPrepSource(client, nil)
	identity := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	now := time.Now()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Go(func() {
		snapshot, err := source.Fetch(context.Background(), identity, now)
		if err == nil && !snapshot.Complete {
			err = context.DeadlineExceeded
		}
		results <- err
	})
	<-entered
	wg.Go(func() {
		snapshot, err := source.Fetch(context.Background(), identity, now)
		if err == nil && !snapshot.Complete {
			err = context.DeadlineExceeded
		}
		results <- err
	})
	// Keep the real first socket open while the second caller joins its flight.
	time.Sleep(30 * time.Millisecond)
	close(release)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), calls.Load())
}
func TestShowPrepSourceRejectsMalformedEnvelopes(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"missing total", `{"hits":[]}`},
		{"nonfinite JSON", `{"hits":[{"price":NaN}],"totalHits":1}`},
		{"null total", `{"hits":[],"totalHits":null}`},
		{"negative total", `{"hits":[],"totalHits":-1}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tt.body)) }))
			defer server.Close()
			client := NewClient(WithBaseURL(server.URL), WithStaticToken("fixture"))
			client.rateLimiter = rate.NewLimiter(rate.Inf, 1)
			result, _ := NewShowPrepSource(client, nil).Fetch(context.Background(), sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}, time.Now())
			require.False(t, result.Complete)
			require.NotEmpty(t, result.AttemptError)
		})
	}
}
func TestShowPrepFractionalGradesUseExistingConditionConvention(t *testing.T) {
	for _, tt := range []struct {
		grader    string
		grade     float64
		condition string
	}{{"PSA", 8.5, "g8_5"}, {"BGS", 9.5, "g9_5"}} {
		t.Run(tt.grader, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Contains(t, r.URL.Query().Get("filters"), "condition:"+tt.condition)
				_ = json.NewEncoder(w).Encode(map[string]any{"totalHits": 1, "hits": []map[string]any{{"itemId": "sale", "profileId": "psa-1", "gradingCompany": tt.grader, "condition": tt.condition, "price": 270, "date": "2026-09-14"}}})
			}))
			defer server.Close()
			client := NewClient(WithBaseURL(server.URL), WithStaticToken("fixture"))
			result, err := NewShowPrepSource(client, nil).Fetch(context.Background(), sp.Identity{ProfileID: "psa-1", Grader: tt.grader, Grade: tt.grade}, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.True(t, result.Complete)
			require.Len(t, result.Sales, 1)
		})
	}
}

func TestShowPrepUnavailableCredentialsMakeNoSourceCall(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	client := NewClient(WithBaseURL(server.URL))
	source := NewShowPrepSource(client, nil)
	result, err := source.Fetch(context.Background(), sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}, time.Now())
	require.Error(t, err)
	require.False(t, result.Complete)
	require.Zero(t, calls)
}
