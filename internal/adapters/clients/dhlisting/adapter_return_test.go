package dhlisting_test

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

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestInventoryAdapterReturn(t *testing.T) {
	tests := []struct {
		name     string
		id       int
		external int64
		status   string
		restored bool
	}{
		{"Charizard", 147840, 443, "in_stock", true},
		{"Spheal", 364577, 848, "in_stock", false},
		{"listed replay is preserved", 147840, 443, "listed", false},
		{"sold replay is preserved", 147840, 443, "sold", false},
		{"unknown replay is preserved", 147840, 443, "future_status", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, fmt.Sprintf("/api/v1/enterprise/inventory/%d/return-to-stock", tt.id), r.URL.Path)
				require.Equal(t, "persisted-key", r.Header.Get("Idempotency-Key"))
				_, _ = fmt.Fprintf(w, `{"dh_inventory_id":%d,"external_sale_id":%d,"item_status":%q,"restored":%t}`, tt.id, tt.external, tt.status, tt.restored)
			}))
			defer server.Close()
			var returner inventory.DHReturner = adapter.NewInventoryAdapter(dh.NewClient(server.URL, dh.WithEnterpriseKey("local-test-key")))
			result, err := returner.ReturnInventoryToStock(context.Background(), tt.id, "persisted-key")
			require.NoError(t, err)
			require.Equal(t, &inventory.DHReturnResult{DHInventoryID: tt.id, ItemStatus: tt.status, ExternalSaleID: tt.external, Restored: tt.restored}, result)
			require.Equal(t, int32(1), calls.Load(), "Return must not sync or list")
		})
	}
}

func TestInventoryAdapterReturnRejections(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		code       string
		definitive bool
	}{
		{"confirmation", 400, "return_confirmation_required", true},
		{"invalid key", 422, "invalid_idempotency_key", true},
		{"not found", 404, "inventory_not_found", true},
		{"not sold", 409, "not_sold", true},
		{"missing attribution", 409, "sale_attribution_missing", true},
		{"ambiguous attribution", 409, "sale_attribution_ambiguous", true},
		{"sale inactive", 409, "sale_not_active", true},
		{"unsafe listing", 409, "unsafe_listing_state", true},
		{"collision", 409, "reversal_would_collide", true},
		{"key conflict", 409, "idempotency_conflict", true},
		{"unknown 400", 400, "unknown", false},
		{"unknown 404", 404, "unknown", false},
		{"unknown 409", 409, "unknown", false},
		{"in progress", 409, "idempotency_in_progress", false},
		{"code on wrong status", 400, "sale_attribution_missing", false},
		{"5xx", 500, "sale_attribution_missing", false},
		{"empty code", 400, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := strings.Repeat("specific upstream detail ", 15)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprintf(w, `{"error":%q,"code":%q}`, message, tt.code)
			}))
			defer server.Close()
			ctx := httpx.WithNoRetry(context.Background())
			result, err := adapter.NewInventoryAdapter(dh.NewClient(server.URL, dh.WithEnterpriseKey("test-key"))).ReturnInventoryToStock(ctx, 147840, "persisted-key")
			require.Nil(t, result)
			var rejection *inventory.DHReturnError
			require.ErrorAs(t, err, &rejection)
			require.Equal(t, tt.code, rejection.Code)
			require.Equal(t, message, rejection.Message)
			require.Equal(t, tt.status, rejection.Status)
			require.Contains(t, rejection.Error(), message)
			require.Equal(t, tt.definitive, inventory.IsDefinitiveDHReturnRejection(fmt.Errorf("outer: %w", err)))
			var upstream *httpx.UpstreamError
			require.ErrorAs(t, err, &upstream, "preserve the original error chain")
		})
	}
	require.False(t, inventory.IsDefinitiveDHReturnRejection(errors.New("transport timeout")))
	require.False(t, inventory.IsDefinitiveDHReturnRejection(nil))
}

func TestInventoryAdapterReturnPriorUncertainty(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"code":"lock_contention","error":"Inventory is busy"}`)
			return
		}
		w.WriteHeader(409)
		_, _ = io.WriteString(w, `{"code":"not_sold","error":"This item is not sold."}`)
	}))
	defer server.Close()
	_, err := adapter.NewInventoryAdapter(dh.NewClient(server.URL, dh.WithEnterpriseKey("test-key"))).ReturnInventoryToStock(context.Background(), 147840, "persisted-key")
	var rejection *inventory.DHReturnError
	require.ErrorAs(t, err, &rejection)
	require.Equal(t, "not_sold", rejection.Code)
	require.True(t, rejection.Uncertain)
	require.False(t, inventory.IsDefinitiveDHReturnRejection(err))
	require.Equal(t, int32(2), calls.Load())
}

func TestInventoryAdapterReturnMalformedOrTruncatedResponse(t *testing.T) {
	tests := []struct {
		name, body string
		truncate   bool
	}{
		{"missing attribution", `{"dh_inventory_id":147840,"item_status":"in_stock","restored":true}`, false},
		{"missing false boolean", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":443}`, false},
		{"wrong target", `{"dh_inventory_id":364577,"item_status":"in_stock","external_sale_id":443,"restored":false}`, false},
		{"response read failure", `short`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.truncate {
					w.Header().Set("Content-Length", "100")
				}
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			_, err := adapter.NewInventoryAdapter(dh.NewClient(server.URL, dh.WithEnterpriseKey("test-key"))).ReturnInventoryToStock(httpx.WithNoRetry(context.Background()), 147840, "persisted-key")
			require.Error(t, err)
			require.False(t, inventory.IsDefinitiveDHReturnRejection(err))
			var failure *httpx.RequestError
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.Uncertain)
		})
	}
}

func TestGetReturnInventoryStatusExactTarget(t *testing.T) {
	tests := []struct {
		name, body, want string
		wantErr          bool
	}{
		{"not first cert result", `{"results":[{"dh_inventory_id":999,"cert_number":"160944741","status":"listed"},{"dh_inventory_id":147840,"cert_number":"160944741","status":"sold"}]}`, "sold", false},
		{"already in stock", `{"results":[{"dh_inventory_id":147840,"cert_number":"160944741","status":"in_stock"}]}`, "in_stock", false},
		{"already listed", `{"results":[{"dh_inventory_id":147840,"cert_number":"160944741","status":"listed"}]}`, "listed", false},
		{"unknown state", `{"results":[{"dh_inventory_id":147840,"cert_number":"160944741","status":"future_status"}]}`, "future_status", false},
		{"target absent", `{"results":[{"dh_inventory_id":999,"cert_number":"160944741","status":"in_stock"}]}`, "", true},
		{"wrong cert", `{"results":[{"dh_inventory_id":147840,"cert_number":"162787413","status":"in_stock"}]}`, "", true},
		{"missing status", `{"results":[{"dh_inventory_id":147840,"cert_number":"160944741"}]}`, "", true},
		{"ambiguous duplicate target", `{"results":[{"dh_inventory_id":147840,"cert_number":"160944741","status":"sold"},{"dh_inventory_id":147840,"cert_number":"160944741","status":"in_stock"}]}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/api/v1/enterprise/inventory", r.URL.Path, "no GET inventory/:id route exists")
				require.Equal(t, "160944741", r.URL.Query().Get("cert_number"))
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			status, err := adapter.NewInventoryAdapter(dh.NewClient(server.URL, dh.WithEnterpriseKey("test-key"))).GetReturnInventoryStatus(context.Background(), 147840, "160944741")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.want, status)
			require.Equal(t, int32(1), calls.Load(), "preflight must never dispatch return or sync")
		})
	}
}

func TestGetReturnInventoryStatusPaginationAndFailures(t *testing.T) {
	tests := []struct {
		name      string
		invalidID bool
		cert      string
		fail      bool
	}{
		{"second page", false, "160944741", false},
		{"read error", false, "160944741", true},
		{"invalid id", true, "160944741", false},
		{"empty cert", false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if tt.fail {
					w.WriteHeader(404)
					return
				}
				if r.URL.Query().Get("page") == "1" {
					_, _ = io.WriteString(w, `{"results":[`)
					for i := 0; i < 100; i++ {
						if i > 0 {
							_, _ = io.WriteString(w, ",")
						}
						_, _ = fmt.Fprintf(w, `{"dh_inventory_id":%d,"cert_number":"160944741","status":"listed"}`, i+1)
					}
					_, _ = io.WriteString(w, `],"meta":{"total_count":1}}`)
					return
				}
				_, _ = io.WriteString(w, `{"results":[{"dh_inventory_id":147840,"cert_number":"160944741","status":"sold"}]}`)
			}))
			defer server.Close()
			id := 147840
			if tt.invalidID {
				id = 0
			}
			status, err := adapter.NewInventoryAdapter(dh.NewClient(server.URL, dh.WithEnterpriseKey("test-key"), dh.WithRateLimitRPS(1000))).GetReturnInventoryStatus(context.Background(), id, tt.cert)
			if tt.invalidID || tt.cert == "" {
				require.Error(t, err)
				require.Zero(t, calls.Load())
				return
			}
			if tt.fail {
				require.Error(t, err)
				require.Empty(t, status)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "sold", status)
			require.Equal(t, int32(2), calls.Load())
		})
	}
}

func TestGetReturnInventoryStatusRejectsDuplicateAcrossPages(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/enterprise/inventory", r.URL.Path)
		require.Equal(t, "160944741", r.URL.Query().Get("cert_number"))
		calls.Add(1)
		if r.URL.Query().Get("page") == "1" {
			_, _ = io.WriteString(w, `{"results":[`)
			for i := 0; i < 100; i++ {
				if i != 0 {
					_, _ = io.WriteString(w, ",")
				}
				id := i + 1
				if i == 0 {
					id = 147840
				}
				_, _ = fmt.Fprintf(w, `{"dh_inventory_id":%d,"cert_number":"160944741","status":"sold"}`, id)
			}
			_, _ = io.WriteString(w, `]}`)
			return
		}
		_, _ = io.WriteString(w, `{"results":[{"dh_inventory_id":147840,"cert_number":"160944741","status":"sold"}]}`)
	}))
	defer server.Close()
	status, err := adapter.NewInventoryAdapter(dh.NewClient(server.URL, dh.WithEnterpriseKey("test-key"), dh.WithRateLimitRPS(1000))).GetReturnInventoryStatus(context.Background(), 147840, "160944741")
	require.ErrorContains(t, err, "conflict")
	require.Empty(t, status)
	require.Equal(t, int32(2), calls.Load())
}

func TestInventoryAdapterMissingReturnCapabilityFailsClosed(t *testing.T) {
	// Existing centralized-compatible sale/list adapters need not acquire new
	// methods just to compile. Missing optional return capability is an error.
	returner := adapter.NewInventoryAdapter(nil)
	_, err := returner.ReturnInventoryToStock(context.Background(), 147840, "key")
	require.Error(t, err)
	_, err = returner.GetReturnInventoryStatus(context.Background(), 147840, "160944741")
	require.Error(t, err)
}
