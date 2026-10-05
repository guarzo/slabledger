package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests catch a request override leaking into the shared retry policy,
// and loss of evidence that an earlier dispatched attempt was uncertain.
func TestRequestLocalNoRetry(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		disable bool
		want    int32
	}{
		{"default", context.Background(), false, 3},
		{"request override", context.Background(), true, 1},
		{"context override", WithNoRetry(context.Background()), false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			config := DefaultConfig("local-retry-test")
			config.RetryPolicy.MaxRetries = 2
			config.RetryPolicy.InitialBackoff = time.Millisecond
			client := NewClient(config)
			_, err := client.Do(tt.ctx, Request{Method: "PATCH", URL: server.URL, DisableRetry: tt.disable})
			require.Error(t, err)
			require.Equal(t, tt.want, calls.Load())
			var failure *RequestError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, int(tt.want), failure.Attempts)
			require.Equal(t, PhaseResponse, failure.Phase)
			require.True(t, failure.Uncertain)

			// The same client must still retry a later ordinary read.
			calls.Store(0)
			_, err = client.Get(context.Background(), server.URL, nil, 0)
			require.Error(t, err)
			require.Equal(t, int32(3), calls.Load())
		})
	}
}

func TestRequestFailurePhase(t *testing.T) {
	tests := []struct {
		name    string
		phase   string
		timeout time.Duration
		handler http.HandlerFunc
	}{
		{"timeout", PhaseDispatch, 15 * time.Millisecond, func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}},
		{"response read", PhaseResponseRead, time.Second, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("short"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				tt.handler(w, r)
			}))
			defer server.Close()
			_, err := NewClient(DefaultConfig("phase-test")).Do(context.Background(), Request{
				Method: "PATCH", URL: server.URL, Timeout: tt.timeout, DisableRetry: true,
			})
			var failure *RequestError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, tt.phase, failure.Phase)
			require.True(t, failure.Uncertain)
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestRequestRetainsPriorUncertainty(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprint(w, `{"code":"not_sold","error":"This item is not sold."}`)
	}))
	defer server.Close()
	config := DefaultConfig("uncertainty-test")
	config.RetryPolicy.InitialBackoff = time.Millisecond
	_, err := NewClient(config).Post(context.Background(), server.URL, map[string]string{"Idempotency-Key": "persisted-key"}, []byte(`{"return_confirmed":true}`), 0)
	var failure *RequestError
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Uncertain, "a later rejection cannot erase an earlier unknown 5xx")
	require.Equal(t, 2, failure.Attempts)
	var upstream *UpstreamError
	require.True(t, errors.As(err, &upstream))
	require.Equal(t, http.StatusConflict, upstream.StatusCode)
}
