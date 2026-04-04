package dh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_PushInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/inventory", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))

		var req InventoryPushRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Len(t, req.Items, 1)
		item := req.Items[0]
		require.Equal(t, 51942, item.DHCardID)
		require.Equal(t, "12345678", item.CertNumber)
		require.Equal(t, "psa", item.GradingCompany)
		require.Equal(t, 9.0, item.Grade)
		require.Equal(t, 5000, item.CostBasisCents)

		resp := InventoryPushResponse{
			Results: []InventoryResult{
				{
					DHInventoryID:      98765,
					Status:             "active",
					AssignedPriceCents: 7500,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	items := []InventoryItem{
		{
			DHCardID:       51942,
			CertNumber:     "12345678",
			GradingCompany: "psa",
			Grade:          9.0,
			CostBasisCents: 5000,
		},
	}
	resp, err := c.PushInventory(context.Background(), items)
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	result := resp.Results[0]
	require.Equal(t, 98765, result.DHInventoryID)
	require.Equal(t, "active", result.Status)
	require.Equal(t, 7500, result.AssignedPriceCents)
}

func TestClient_ListInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/enterprise/inventory", r.URL.Path)
		require.Equal(t, "active", r.URL.Query().Get("status"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{
					"dh_inventory_id": 98765,
					"dh_card_id": 52304,
					"cert_number": "12345678",
					"card_name": "Dreepy",
					"set_name": "Pokemon Ascended Heroes",
					"card_number": "247",
					"grading_company": "psa",
					"grade": "9.0",
					"status": "active",
					"listing_price_cents": 10688,
					"cost_basis_cents": 893,
					"channels": [{"name": "ebay", "status": "error"}],
					"created_at": "2026-04-04T21:50:42Z",
					"updated_at": "2026-04-04T21:50:42Z"
				}
			],
			"meta": {"page": 1, "per_page": 25, "total_count": 1}
		}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.ListInventory(context.Background(), InventoryFilters{Status: "active"})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	item := resp.Items[0]
	require.Equal(t, 98765, item.DHInventoryID)
	require.Equal(t, 52304, item.DHCardID)
	require.Equal(t, "12345678", item.CertNumber)
	require.Equal(t, "Dreepy", item.CardName)
	require.Equal(t, "psa", item.GradingCompany)
	require.Equal(t, "9.0", item.Grade)
	require.Equal(t, "active", item.Status)
	require.Equal(t, 10688, item.ListingPriceCents)
	require.Len(t, item.Channels, 1)
	require.Equal(t, "ebay", item.Channels[0].Name)
}

func TestClient_GetOrders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/enterprise/orders", r.URL.Path)
		require.Equal(t, "2026-01-01T00:00:00Z", r.URL.Query().Get("since"))

		resp := OrdersResponse{
			Orders: []Order{
				{
					OrderID:        "dh-12345",
					SalePriceCents: 7500,
					Channel:        "ebay",
					Fees: OrderFees{
						ChannelFeeCents: intPtr(994),
					},
					NetAmountCents: intPtr(6506),
				},
			},
			Meta: PaginationMeta{
				Page:       1,
				PerPage:    25,
				TotalCount: 1,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.GetOrders(context.Background(), OrderFilters{Since: "2026-01-01T00:00:00Z"})
	require.NoError(t, err)
	require.Len(t, resp.Orders, 1)
	order := resp.Orders[0]
	require.Equal(t, "dh-12345", order.OrderID)
	require.Equal(t, 7500, order.SalePriceCents)
	require.Equal(t, "ebay", order.Channel)
	require.NotNil(t, order.Fees.ChannelFeeCents)
	require.Equal(t, 994, *order.Fees.ChannelFeeCents)
	require.NotNil(t, order.NetAmountCents)
	require.Equal(t, 6506, *order.NetAmountCents)
}

func intPtr(v int) *int { return &v }
