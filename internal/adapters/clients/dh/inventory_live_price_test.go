package dh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/stretchr/testify/require"
)

func TestUpdateLivePriceWireAndReplay(t *testing.T) {
	tests := []struct {
		name, key string
		cents     int
	}{
		{"ordinary", "persisted-live-price-key", 53116},
		{"maximum UTF8 key and cents", strings.Repeat("é", 255), 9999999999},
		{"minimum key and cents", "x", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "/api/v1/enterprise/inventory/147840/live-price", r.URL.Path)
				require.Empty(t, r.URL.RawQuery)
				require.Equal(t, tt.key, r.Header.Get("Idempotency-Key"))
				require.Equal(t, "Bearer local-enterprise-key", r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("X-PSA-API-Key"))
				require.Equal(t, "application/json", r.Header.Get("Content-Type"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf(`{"listing_price_cents":%d}`, tt.cents), string(body))
				_, _ = fmt.Fprintf(w, `{"dh_inventory_id":147840,"listing_price_cents":%d,"replayed":%t}`, tt.cents, n > 1)
			}))
			defer server.Close()
			client := newTestClient(server.URL)
			client.enterpriseKey = "local-enterprise-key"
			client.psaKeys = []string{"unused-local-psa-key"}
			for _, replayed := range []bool{false, true} {
				result, err := client.UpdateLivePrice(context.Background(), 147840, tt.key, tt.cents)
				require.NoError(t, err)
				require.Equal(t, &LivePriceResult{DHInventoryID: 147840, ListingPriceCents: tt.cents, Replayed: replayed}, result)
			}
			require.Equal(t, int32(2), calls.Load())
		})
	}
}

func TestUpdateLivePriceRejectsInputBeforeDispatch(t *testing.T) {
	tests := []struct {
		name, key string
		id, cents int
	}{
		{"zero target", "key", 0, 100},
		{"negative target", "key", -1, 100},
		{"zero cents", "key", 1, 0},
		{"negative cents", "key", 1, -1},
		{"excessive cents", "key", 1, 10000000000},
		{"empty key", "", 1, 100},
		{"blank key", " \t", 1, 100},
		{"leading whitespace", " key", 1, 100},
		{"trailing whitespace", "key\n", 1, 100},
		{"too many runes", strings.Repeat("é", 256), 1, 100},
		{"invalid UTF8", "key\xff", 1, 100},
		{"NUL", "key\x00end", 1, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{}`)
			}))
			defer server.Close()
			result, err := newTestClient(server.URL).UpdateLivePrice(context.Background(), tt.id, tt.key, tt.cents)
			require.Error(t, err)
			require.Nil(t, result)
			require.True(t, apperrors.HasErrorCode(err, apperrors.ErrCodeProviderInvalidReq))
			require.Equal(t, int32(0), calls.Load())
		})
	}
}

func TestUpdateLivePriceOneAttemptErrorsPreserveEvidence(t *testing.T) {
	tests := []struct {
		name             string
		status           int
		body, retryAfter string
		readFailure      bool
	}{
		{"not found", 404, `{"code":"inventory_not_found","error":"missing"}`, "", false},
		{"conflict", 409, `{"code":"idempotency_conflict","error":"different"}`, "", false},
		{"invalid price", 422, `{"code":"invalid_price","error":"invalid"}`, "", false},
		{"unauthorized", 401, `{"error":"auth"}`, "", false},
		{"forbidden", 403, `{"error":"auth"}`, "", false},
		{"rate limit", 429, `{"error":"slow down"}`, "37", false},
		{"server error", 500, `{}`, "", false},
		{"bad gateway", 502, `{}`, "", false},
		{"unavailable", 503, `{}`, "", false},
		{"gateway timeout", 504, `{}`, "", false},
		{"response read failure", 200, "short", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", tt.retryAfter)
				if tt.readFailure {
					w.Header().Set("Content-Length", "100")
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			result, err := retryTestClient(server.URL, time.Second).UpdateLivePrice(context.Background(), 147840, "persisted-key", 53116)
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, int32(1), calls.Load())
			var failure *httpx.RequestError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, 1, failure.Attempts)
			require.Equal(t, tt.status >= 500 || tt.readFailure, failure.Uncertain)
			if !tt.readFailure {
				var upstream *httpx.UpstreamError
				require.ErrorAs(t, err, &upstream)
				require.Equal(t, tt.status, upstream.StatusCode)
				require.Equal(t, tt.retryAfter, upstream.RetryAfter)
			}
		})
	}
}

func TestUpdateLivePriceRejectsInvalidReceipt(t *testing.T) {
	tests := []struct{ name, receipt string }{
		{"missing target", `{"listing_price_cents":53116,"replayed":false}`},
		{"missing cents", `{"dh_inventory_id":147840,"replayed":false}`},
		{"wrong field", `{"dh_inventory_id":147840,"price_cents":53116,"replayed":false}`},
		{"missing replayed", `{"dh_inventory_id":147840,"listing_price_cents":53116}`},
		{"null replayed", `{"dh_inventory_id":147840,"listing_price_cents":53116,"replayed":null}`},
		{"null target", `{"dh_inventory_id":null,"listing_price_cents":53116,"replayed":false}`},
		{"null cents", `{"dh_inventory_id":147840,"listing_price_cents":null,"replayed":false}`},
		{"wrong target", `{"dh_inventory_id":147841,"listing_price_cents":53116,"replayed":true}`},
		{"wrong cents", `{"dh_inventory_id":147840,"listing_price_cents":53117,"replayed":true}`},
		{"negative target", `{"dh_inventory_id":-1,"listing_price_cents":53116,"replayed":false}`},
		{"negative cents", `{"dh_inventory_id":147840,"listing_price_cents":-1,"replayed":false}`},
		{"zero target", `{"dh_inventory_id":0,"listing_price_cents":53116,"replayed":false}`},
		{"zero cents", `{"dh_inventory_id":147840,"listing_price_cents":0,"replayed":false}`},
		{"out of range cents", `{"dh_inventory_id":147840,"listing_price_cents":10000000000,"replayed":false}`},
		{"string target", `{"dh_inventory_id":"147840","listing_price_cents":53116,"replayed":false}`},
		{"string cents", `{"dh_inventory_id":147840,"listing_price_cents":"53116","replayed":false}`},
		{"fractional cents", `{"dh_inventory_id":147840,"listing_price_cents":53116.5,"replayed":false}`},
		{"string replayed", `{"dh_inventory_id":147840,"listing_price_cents":53116,"replayed":"false"}`},
		{"integer replayed", `{"dh_inventory_id":147840,"listing_price_cents":53116,"replayed":0}`},
		{"null", `null`},
		{"empty", `{}`},
		{"invalid JSON", `broken`},
		{"trailing JSON", `{"dh_inventory_id":147840,"listing_price_cents":53116,"replayed":false}{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, tt.receipt)
			}))
			defer server.Close()
			result, err := retryTestClient(server.URL, time.Second).UpdateLivePrice(context.Background(), 147840, "persisted-key", 53116)
			require.Error(t, err)
			require.Nil(t, result)
			var failure *httpx.RequestError
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.Uncertain, "bad receipts cannot prove a mutation did not happen")
			require.True(t, apperrors.HasErrorCode(err, apperrors.ErrCodeProviderInvalidResp))
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestUpdateLivePriceCanceledAndTransport(t *testing.T) {
	tests := []struct {
		name   string
		cancel bool
	}{{"canceled before dispatch", true}, {"lost response", false}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancel {
				cancel()
			}
			result, err := retryTestClient(server.URL, time.Second).UpdateLivePrice(ctx, 147840, "persisted-key", 53116)
			require.Error(t, err)
			require.Nil(t, result)
			if tt.cancel {
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, calls.Load())
			} else {
				require.Equal(t, int32(1), calls.Load())
				var failure *httpx.RequestError
				require.ErrorAs(t, err, &failure)
				require.True(t, failure.Uncertain)
				var upstream *httpx.UpstreamError
				require.False(t, errors.As(err, &upstream))
			}
		})
	}
}
