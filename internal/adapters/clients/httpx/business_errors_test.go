package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/stretchr/testify/require"
)

func TestClient_BusinessRejectionsPreserveCircuit(t *testing.T) {
	tests := []struct {
		status, attempts, requests int
		failures                   uint32
		state                      string
	}{
		{400, 1, 6, 0, "closed"},
		{409, 1, 6, 0, "closed"},
		{422, 1, 6, 0, "closed"},
		{503, 2, 2, 2, "open"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprint(w, `{"code":"rejected","error":"temporary rejection"}`)
			}))
			defer server.Close()
			config := DefaultConfig("business-errors")
			config.RetryPolicy.MaxRetries = 1
			config.RetryPolicy.InitialBackoff = time.Millisecond
			config.CircuitBreakerConfig.MinRequests = 4
			config.CircuitBreakerConfig.FailureRatio = 0.5
			client := NewClient(config)

			// The path must not make a business rejection look like a 503 outage.
			for i := 0; i < tt.requests; i++ {
				_, err := client.Get(context.Background(), server.URL+"/inventory/503", nil, 0)
				var failure *RequestError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, tt.attempts, failure.Attempts)
				var upstream *UpstreamError
				require.ErrorAs(t, err, &upstream)
				require.Equal(t, tt.status, upstream.StatusCode)
				require.Equal(t, "rejected", upstream.Code)
				require.Equal(t, "temporary rejection", upstream.Message)
				if tt.status < 500 {
					require.True(t, apperrors.HasErrorCode(err, apperrors.ErrCodeProviderInvalidReq))
				}
				if i == 0 {
					require.Equal(t, tt.failures, client.GetCircuitBreakerStats().Failures)
				}
			}
			require.EqualValues(t, tt.requests*tt.attempts, calls.Load())
			stats := client.GetCircuitBreakerStats()
			require.Equal(t, tt.state, stats.State)
			if tt.state == "closed" {
				require.EqualValues(t, tt.requests, stats.Successes)
				require.Zero(t, stats.Failures)
			} else {
				_, err := client.Get(context.Background(), server.URL, nil, 0)
				require.True(t, apperrors.HasErrorCode(err, apperrors.ErrCodeProviderCircuitOpen))
				require.EqualValues(t, tt.requests*tt.attempts, calls.Load())
			}
		})
	}
}
