package dh

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/stretchr/testify/require"
)

func TestReturnInventoryToStockContract(t *testing.T) {
	tests := []struct {
		name, status string
		restored     bool
	}{
		{"first return", "in_stock", true},
		{"replay in stock", "in_stock", false},
		{"replay listed", "listed", false},
		{"replay sold", "sold", false},
		{"replay unknown", "future_status", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/api/v1/enterprise/inventory/147840/return-to-stock", r.URL.Path)
				require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))
				require.Equal(t, "persisted-return-key", r.Header.Get("Idempotency-Key"))
				require.Equal(t, "application/json", r.Header.Get("Content-Type"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"return_confirmed":true}`, string(body))
				w.Header().Set("Content-Type", "application/json")
				restored := "false"
				if tt.restored {
					restored = "true"
				}
				_, _ = io.WriteString(w, `{"dh_inventory_id":147840,"external_sale_id":443,"item_status":"`+tt.status+`","restored":`+restored+`}`)
			}))
			defer server.Close()
			result, err := newTestClient(server.URL).ReturnInventoryToStock(context.Background(), 147840, "persisted-return-key")
			require.NoError(t, err)
			require.Equal(t, 147840, result.DHInventoryID)
			require.Equal(t, int64(443), result.ExternalSaleID)
			require.Equal(t, tt.status, result.ItemStatus)
			require.Equal(t, tt.restored, result.Restored)
		})
	}
}

func TestReturnInventoryToStockValidation(t *testing.T) {
	tests := []struct{ name, body string }{
		{"missing target", `{"item_status":"in_stock","external_sale_id":443,"restored":false}`},
		{"null target", `{"dh_inventory_id":null,"item_status":"in_stock","external_sale_id":443,"restored":false}`},
		{"wrong target", `{"dh_inventory_id":364577,"item_status":"in_stock","external_sale_id":443,"restored":false}`},
		{"missing status", `{"dh_inventory_id":147840,"external_sale_id":443,"restored":false}`},
		{"null status", `{"dh_inventory_id":147840,"item_status":null,"external_sale_id":443,"restored":false}`},
		{"blank status", `{"dh_inventory_id":147840,"item_status":" ","external_sale_id":443,"restored":false}`},
		{"missing sale", `{"dh_inventory_id":147840,"item_status":"in_stock","restored":false}`},
		{"null sale", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":null,"restored":false}`},
		{"zero sale", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":0,"restored":false}`},
		{"negative sale", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":-1,"restored":false}`},
		{"fractional sale", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":1.5,"restored":false}`},
		{"string sale", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":"443","restored":false}`},
		{"overflow sale", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":9223372036854775808,"restored":false}`},
		{"missing restored", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":443}`},
		{"null restored", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":443,"restored":null}`},
		{"string restored", `{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":443,"restored":"false"}`},
		{"invalid JSON", `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			result, err := newTestClient(server.URL).ReturnInventoryToStock(context.Background(), 147840, "persisted-key")
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}

func TestReturnInventoryToStockInvalidRequestDoesNotDispatch(t *testing.T) {
	tests := []struct {
		name string
		id   int
		key  string
	}{
		{"zero target", 0, "key"}, {"negative target", -1, "key"},
		{"empty key", 1, ""}, {"blank key", 1, " \t"},
		{"long key", 1, strings.Repeat("x", 256)},
		{"key normalization forbidden", 1, " key "},
		{"invalid header", 1, "key\r\nOther: value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1) }))
			defer server.Close()
			_, err := newTestClient(server.URL).ReturnInventoryToStock(context.Background(), tt.id, tt.key)
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}

// Source-derived fixtures, not captured production POST responses. The local
// upstream controller/service supplied these status/code/error combinations.
func TestReturnInventoryToStockUpstreamErrors(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		code, message string
	}{
		{"confirmation", 400, "return_confirmation_required", "Explicit physical return confirmation is required"},
		{"attribution", 409, "sale_attribution_missing", "This item has no attributable external sale."},
		{"not found", 404, "inventory_not_found", "Inventory item not found"},
		{"contention", 503, "lock_contention", "Could not update the item because another request held the same rows. Retry this item."},
		{"long message", 409, "sale_attribution_ambiguous", strings.Repeat("attribution detail ", 30)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"error":"`+tt.message+`","code":"`+tt.code+`"}`)
			}))
			defer server.Close()
			result, err := newTestClient(server.URL).ReturnInventoryToStock(context.Background(), 147840, "persisted-key")
			require.Nil(t, result)
			var upstream *httpx.UpstreamError
			require.ErrorAs(t, err, &upstream)
			require.Equal(t, tt.status, upstream.StatusCode)
			require.Equal(t, tt.code, upstream.Code)
			require.Equal(t, tt.message, upstream.Message)
		})
	}
}
