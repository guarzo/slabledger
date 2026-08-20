package dh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_RecordInventorySale(t *testing.T) {
	var gotHeader, gotPath string
	var gotBody InventorySaleRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Idempotency-Key")
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(InventorySaleResponse{
			SaleID:              "sale_123",
			DHInventoryID:       98765,
			SoldInventoryID:     nil, // DH sends null; must parse to a nil pointer
			ItemStatus:          "sold",
			Delisted:            true,
			RealizedProfitCents: 1500,
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, WithEnterpriseKey("test_key"))
	resp, err := c.RecordInventorySale(context.Background(), 98765, "slabledger-sale-abc123", InventorySaleRequest{
		SalePriceCents:   45000,
		Quantity:         5, // deliberately wrong; must be forced to 1
		SoldAt:           "2026-08-15T14:22:00Z",
		CounterpartyName: "Jane Buyer",
	})

	require.NoError(t, err)
	require.Equal(t, "slabledger-sale-abc123", gotHeader)
	require.Equal(t, "/api/v1/enterprise/inventory/98765/sale", gotPath)
	require.Equal(t, 1, gotBody.Quantity, "quantity must always be forced to 1: every purchase is a single slab")
	require.Equal(t, "sale_123", resp.SaleID)
	require.Nil(t, resp.SoldInventoryID, "null sold_inventory_id must parse to a nil pointer, not a zero int")
	require.True(t, resp.Delisted)
	require.Equal(t, 1500, resp.RealizedProfitCents)
}

func TestClient_VoidInventorySale(t *testing.T) {
	var gotPath string
	var gotBody VoidSaleRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(VoidSaleResponse{Reversed: true})
	}))
	defer server.Close()

	c := NewClient(server.URL, WithEnterpriseKey("test_key"))
	resp, err := c.VoidInventorySale(context.Background(), "sale_123", VoidSaleRequest{Reason: "returned"})

	require.NoError(t, err)
	require.Equal(t, "/api/v1/enterprise/sales/sale_123/void", gotPath)
	require.Equal(t, "returned", gotBody.Reason)
	require.True(t, resp.Reversed)
}
