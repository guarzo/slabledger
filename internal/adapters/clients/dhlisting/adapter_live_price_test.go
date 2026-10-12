package dhlisting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func livePriceFixtureClient(url string) *dh.Client {
	return dh.NewClient(url, dh.WithEnterpriseKey("local-enterprise-key"),
		dh.WithPSAKeys("unused-local-psa-key,unused-local-psa-key-2"), dh.WithRateLimitRPS(1000))
}

func TestLivePriceAdapterWireAndHistoricalReplay(t *testing.T) {
	tests := []struct {
		name     string
		replayed bool
	}{{"first acceptance", false}, {"historical acceptance", true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "/api/v1/enterprise/inventory/147840/live-price", r.URL.Path)
				require.Equal(t, "persisted-key", r.Header.Get("Idempotency-Key"))
				require.Empty(t, r.Header.Get("X-PSA-API-Key"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, `{"listing_price_cents":53116}`, string(body))
				_, _ = fmt.Fprintf(w, `{"dh_inventory_id":147840,"listing_price_cents":53116,"replayed":%t}`, tt.replayed)
			}))
			defer server.Close()
			result, err := NewLivePriceAdapter(livePriceFixtureClient(server.URL)).UpdateLivePrice(context.Background(), 147840, "persisted-key", 53116)
			require.NoError(t, err)
			require.Equal(t, &inventory.DHLivePriceResult{DHInventoryID: 147840, ListingPriceCents: 53116, Replayed: tt.replayed}, result)
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestLivePriceAdapterClassifiesOnlyVerifiedPostLockRefusals(t *testing.T) {
	tests := []struct {
		name, code    string
		status        int
		wantRejection bool
	}{
		{"not listed", "inventory_not_listed", 409, true},
		{"stale listing", "stale_listing", 409, true},
		{"price validation", "price_validation_failed", 422, true},
		{"generic 404", "", 404, false},
		{"missing inventory or vendor", "inventory_not_found", 404, false},
		{"conflicting key", "idempotency_conflict", 409, false},
		{"unknown conflict", "unknown", 409, false},
		{"invalid request", "invalid_request", 422, false},
		{"invalid cents", "invalid_price", 422, false},
		{"invalid key", "invalid_idempotency_key", 422, false},
		{"unauthorized", "inventory_not_listed", 401, false},
		{"forbidden", "stale_listing", 403, false},
		{"wrong status not listed", "inventory_not_listed", 422, false},
		{"wrong status stale", "stale_listing", 422, false},
		{"wrong status validation", "price_validation_failed", 409, false},
		{"server error", "stale_listing", 500, false},
		{"bad gateway", "inventory_not_listed", 502, false},
		{"unavailable", "price_update_failed", 503, false},
		{"gateway timeout", "lock_contention", 504, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "/api/v1/enterprise/inventory/147840/live-price", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprintf(w, `{"code":%q,"error":"provider refusal"}`, tt.code)
			}))
			defer server.Close()
			result, err := NewLivePriceAdapter(livePriceFixtureClient(server.URL)).UpdateLivePrice(context.Background(), 147840, "persisted-key", 53116)
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, int32(1), calls.Load(), "no retry, fallback PATCH, PSA rotation or lifecycle operation")
			var upstream *httpx.UpstreamError
			require.ErrorAs(t, err, &upstream)
			require.Equal(t, tt.status, upstream.StatusCode)
			require.Equal(t, tt.code, upstream.Code)
			var rejection *inventory.DHLivePriceRejection
			require.Equal(t, tt.wantRejection, errors.As(err, &rejection))
			if tt.wantRejection {
				require.Equal(t, tt.code, rejection.Code)
				require.ErrorIs(t, err, rejection.Cause)
			}
			var generic *inventory.DHNonMutationError
			require.False(t, errors.As(err, &generic), "live-price refusal cannot relax legacy nonmutation handling")
		})
	}
}

func TestLivePriceAdapterRefusalRequiresJSONMediaType(t *testing.T) {
	for _, media := range []string{"text/html", "text/plain", "application/jsonp", "application/json-invalid"} {
		t.Run(media, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", media)
				w.WriteHeader(409)
				_, _ = io.WriteString(w, `{"code":"stale_listing","error":"untrusted display"}`)
			}))
			defer server.Close()
			_, err := NewLivePriceAdapter(livePriceFixtureClient(server.URL)).UpdateLivePrice(context.Background(), 147840, "persisted-key", 53116)
			require.Error(t, err)
			var rejection *inventory.DHLivePriceRejection
			require.False(t, errors.As(err, &rejection), "display body must not settle an uncertain operation")
		})
	}
}

func TestLivePriceAdapterRefusalWithPriorUncertainty(t *testing.T) {
	tests := []struct {
		code      string
		status    int
		rejection bool
	}{
		{"inventory_not_listed", 409, true}, {"stale_listing", 409, true}, {"price_validation_failed", 422, true},
		{"inventory_not_found", 404, false}, {"idempotency_conflict", 409, false}, {"invalid_price", 422, false},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			source := &httpx.RequestError{Uncertain: true, Attempts: 2, Phase: httpx.PhaseResponse,
				Err: &httpx.UpstreamError{Code: tt.code, StatusCode: tt.status, ContentType: "application/json"}}
			err := classifyDHLivePriceError(source)
			var rejection *inventory.DHLivePriceRejection
			require.Equal(t, tt.rejection, errors.As(err, &rejection))
			require.ErrorIs(t, err, source)
			if !tt.rejection {
				require.Same(t, source, err)
			}
		})
	}
}

func TestLivePriceRetryDeadlineDoesNotShortenLongValidDelays(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 123456789, time.UTC)
	for _, seconds := range []int64{172800, 999999999, 10000000000} {
		t.Run(fmt.Sprint(seconds), func(t *testing.T) {
			want := time.Unix(now.Unix()+seconds, int64(now.Nanosecond()))
			require.True(t, livePriceRetryNotBefore(fmt.Sprint(seconds), now).Equal(want), "valid delta-seconds must not fall back or be capped")
		})
	}
	deadline := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	require.True(t, livePriceRetryNotBefore(deadline.Format(http.TimeFormat), now).Equal(deadline))
}

func TestLivePriceAdapterRateLimitDeadline(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name, header string
		delay        time.Duration
		absolute     bool
	}{
		{"seconds", "37", 37 * time.Second, false},
		{"zero seconds", "0", 0, false},
		{"HTTP date", now.Add(time.Hour).Format(http.TimeFormat), time.Hour, true},
		{"past date", now.Add(-time.Hour).Format(http.TimeFormat), 0, false},
		{"missing", "", 15 * time.Minute, false},
		{"malformed", "later", 15 * time.Minute, false},
		{"negative", "-1", 15 * time.Minute, false},
		{"fractional", "1.5", 15 * time.Minute, false},
		{"signed", "+1", 15 * time.Minute, false},
		{"overflow", "999999999999999999999999", 15 * time.Minute, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if tt.header != "" {
					w.Header().Set("Retry-After", tt.header)
				}
				w.WriteHeader(429)
				_, _ = io.WriteString(w, `{"error":"slow down"}`)
			}))
			defer server.Close()
			before := time.Now()
			result, err := NewLivePriceAdapter(livePriceFixtureClient(server.URL)).UpdateLivePrice(context.Background(), 147840, "persisted-key", 53116)
			after := time.Now()
			require.Nil(t, result)
			var retry *inventory.DHMutationRetryError
			require.ErrorAs(t, err, &retry)
			var upstream *httpx.UpstreamError
			require.ErrorAs(t, retry.Cause, &upstream)
			require.Equal(t, tt.header, upstream.RetryAfter)
			require.ErrorIs(t, err, retry.Cause)
			if tt.absolute {
				require.Equal(t, now.Add(time.Hour), retry.NotBefore)
			} else {
				require.False(t, retry.NotBefore.Before(before.Add(tt.delay)))
				require.False(t, retry.NotBefore.After(after.Add(tt.delay)))
			}
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestLivePriceAdapterInvalidReceiptsAndLostResponseRemainUnknown(t *testing.T) {
	tests := []struct {
		name, body string
		disconnect bool
	}{
		{"missing Boolean", `{"dh_inventory_id":147840,"listing_price_cents":53116}`, false},
		{"wrong target", `{"dh_inventory_id":1,"listing_price_cents":53116,"replayed":true}`, false},
		{"wrong price", `{"dh_inventory_id":147840,"listing_price_cents":53117,"replayed":false}`, false},
		{"nested rejection", `{"error":{"code":"stale_listing"}}`, false},
		{"lost response", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if tt.disconnect {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				}
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			result, err := NewLivePriceAdapter(livePriceFixtureClient(server.URL)).UpdateLivePrice(context.Background(), 147840, "persisted-key", 53116)
			require.Error(t, err)
			require.Nil(t, result)
			var failure *httpx.RequestError
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.Uncertain)
			var rejection *inventory.DHLivePriceRejection
			require.False(t, errors.As(err, &rejection))
			var retry *inventory.DHMutationRetryError
			require.False(t, errors.As(err, &retry))
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestLivePriceAdapterCanceledPreservesCause(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { calls.Add(1) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := NewLivePriceAdapter(livePriceFixtureClient(server.URL)).UpdateLivePrice(ctx, 147840, "persisted-key", 53116)
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls.Load())
}
