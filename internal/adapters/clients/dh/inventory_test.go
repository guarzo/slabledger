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
		require.Equal(t, InventoryStatusInStock, item.Status)

		resp := InventoryPushResponse{
			Results: []InventoryResult{
				{
					DHInventoryID:      98765,
					Status:             InventoryStatusInStock,
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
			Status:         InventoryStatusInStock,
		},
	}
	resp, err := c.PushInventory(context.Background(), items)
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	result := resp.Results[0]
	require.Equal(t, 98765, result.DHInventoryID)
	require.Equal(t, InventoryStatusInStock, result.Status)
	require.Equal(t, 7500, result.AssignedPriceCents)
}

func TestClient_ListInventory(t *testing.T) {
	tests := []struct {
		name           string
		filters        InventoryFilters
		serverResponse string
		wantLen        int
		wantFirst      *InventoryListItem
	}{
		{
			name:    "active inventory with channels",
			filters: InventoryFilters{Status: InventoryStatusListed},
			serverResponse: `{
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
						"status": "listed",
						"listing_price_cents": 10688,
						"cost_basis_cents": 893,
						"channels": [{"name": "ebay", "status": "error"}],
						"created_at": "2026-04-04T21:50:42Z",
						"updated_at": "2026-04-04T21:50:42Z"
					}
				],
				"meta": {"page": 1, "per_page": 25, "total_count": 1}
			}`,
			wantLen: 1,
			wantFirst: &InventoryListItem{
				DHInventoryID:     98765,
				DHCardID:          52304,
				CertNumber:        "12345678",
				CardName:          "Dreepy",
				GradingCompany:    "psa",
				Grade:             "9.0",
				Status:            InventoryStatusListed,
				ListingPriceCents: 10688,
				Channels:          []InventoryChannelStatus{{Name: "ebay", Status: "error"}},
			},
		},
		{
			name:           "empty results",
			filters:        InventoryFilters{},
			serverResponse: `{"results":[],"meta":{"page":1,"per_page":25,"total_count":0}}`,
			wantLen:        0,
			wantFirst:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/v1/enterprise/inventory", r.URL.Path)
				if tc.filters.Status != "" {
					require.Equal(t, tc.filters.Status, r.URL.Query().Get("status"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.serverResponse))
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			resp, err := c.ListInventory(context.Background(), tc.filters)
			require.NoError(t, err)
			require.Len(t, resp.Items, tc.wantLen)
			if tc.wantFirst != nil {
				item := resp.Items[0]
				require.Equal(t, tc.wantFirst.DHInventoryID, item.DHInventoryID)
				require.Equal(t, tc.wantFirst.DHCardID, item.DHCardID)
				require.Equal(t, tc.wantFirst.CertNumber, item.CertNumber)
				require.Equal(t, tc.wantFirst.CardName, item.CardName)
				require.Equal(t, tc.wantFirst.GradingCompany, item.GradingCompany)
				require.Equal(t, tc.wantFirst.Grade, item.Grade)
				require.Equal(t, tc.wantFirst.Status, item.Status)
				require.Equal(t, tc.wantFirst.ListingPriceCents, item.ListingPriceCents)
				require.Len(t, item.Channels, len(tc.wantFirst.Channels))
				require.Equal(t, tc.wantFirst.Channels[0].Name, item.Channels[0].Name)
			}
		})
	}
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

func TestClient_UpdateInventory(t *testing.T) {
	tests := []struct {
		name       string
		id         int
		update     InventoryUpdate
		statusCode int
		response   string
		wantID     int
		wantStatus string
		wantErr    bool
	}{
		{
			name:       "success",
			id:         98765,
			update:     InventoryUpdate{Status: InventoryStatusListed},
			statusCode: http.StatusOK,
			response:   `{"dh_inventory_id":98765,"status":"listed"}`,
			wantID:     98765,
			wantStatus: InventoryStatusListed,
		},
		{
			name:       "server error",
			id:         98765,
			update:     InventoryUpdate{Status: InventoryStatusListed},
			statusCode: http.StatusInternalServerError,
			response:   `{"error":"internal"}`,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "PATCH", r.Method)
				require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			result, err := c.UpdateInventory(context.Background(), tc.id, tc.update)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantID, result.DHInventoryID)
			require.Equal(t, tc.wantStatus, result.Status)
		})
	}
}

func TestClient_SyncChannels(t *testing.T) {
	tests := []struct {
		name         string
		id           int
		channels     []string
		statusCode   int
		response     string
		wantID       int
		wantChannels int
		wantErr      bool
	}{
		{
			name:         "success",
			id:           98765,
			channels:     []string{"ebay", "shopify"},
			statusCode:   http.StatusOK,
			response:     `{"dh_inventory_id":98765,"status":"listed","channels":[{"name":"ebay","status":"pending"},{"name":"shopify","status":"pending"}]}`,
			wantID:       98765,
			wantChannels: 2,
		},
		{
			name:       "server error",
			id:         98765,
			channels:   []string{"ebay"},
			statusCode: http.StatusInternalServerError,
			response:   `{"error":"internal"}`,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			resp, err := c.SyncChannels(context.Background(), tc.id, tc.channels)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantID, resp.DHInventoryID)
			require.Len(t, resp.Channels, tc.wantChannels)
		})
	}
}

func TestClient_DelistChannels(t *testing.T) {
	tests := []struct {
		name         string
		id           int
		channels     []string
		statusCode   int
		response     string
		wantID       int
		wantChannels int
		wantErr      bool
	}{
		{
			name:         "success",
			id:           98765,
			channels:     []string{"ebay"},
			statusCode:   http.StatusOK,
			response:     `{"dh_inventory_id":98765,"status":"listed","channels":[{"name":"shopify","status":"active"}]}`,
			wantID:       98765,
			wantChannels: 1,
		},
		{
			name:       "server error",
			id:         98765,
			channels:   []string{"ebay"},
			statusCode: http.StatusInternalServerError,
			response:   `{"error":"internal"}`,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "DELETE", r.Method)
				require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			resp, err := c.DelistChannels(context.Background(), tc.id, tc.channels)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantID, resp.DHInventoryID)
			require.Len(t, resp.Channels, tc.wantChannels)
		})
	}
}

func intPtr(v int) *int { return &v }

func TestClient_UpdateInventory_PSAHeaderOnListed(t *testing.T) {
	tests := []struct {
		name       string
		update     InventoryUpdate
		psaKeys    string
		wantPSAHdr string
	}{
		{
			name:       "status=listed with keys sends PSA header",
			update:     InventoryUpdate{Status: "listed"},
			psaKeys:    "key_alpha,key_beta",
			wantPSAHdr: "key_alpha",
		},
		{
			name:       "status=in_stock with keys does NOT send PSA header",
			update:     InventoryUpdate{Status: "in_stock"},
			psaKeys:    "key_alpha",
			wantPSAHdr: "",
		},
		{
			name:       "status=listed without keys does NOT send PSA header",
			update:     InventoryUpdate{Status: "listed"},
			psaKeys:    "",
			wantPSAHdr: "",
		},
		{
			name:       "status=sold with keys does NOT send PSA header",
			update:     InventoryUpdate{Status: "sold"},
			psaKeys:    "key_alpha",
			wantPSAHdr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPSAHdr string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPatch, r.Method)
				gotPSAHdr = r.Header.Get("X-PSA-API-Key")
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(InventoryResult{DHInventoryID: 1, Status: tt.update.Status})
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			if tt.psaKeys != "" {
				WithPSAKeys(tt.psaKeys)(c)
			}
			_, err := c.UpdateInventory(context.Background(), 1, tt.update)
			require.NoError(t, err)
			require.Equal(t, tt.wantPSAHdr, gotPSAHdr)
		})
	}
}

func TestInventoryItem_ListingPriceCents_Serialization(t *testing.T) {
	tests := []struct {
		name              string
		listingPriceCents *int
		wantContains      string
		wantNotContains   string
	}{
		{
			name:              "with listing price",
			listingPriceCents: intPtr(45000),
			wantContains:      `"listing_price_cents":45000`,
			wantNotContains:   "market_value_cents",
		},
		{
			name:            "without listing price omits field",
			wantNotContains: "listing_price_cents",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := InventoryItem{
				DHCardID:          12345,
				CertNumber:        "98765",
				GradingCompany:    "psa",
				Grade:             9,
				CostBasisCents:    15000,
				ListingPriceCents: tc.listingPriceCents,
				Status:            InventoryStatusInStock,
			}
			b, err := json.Marshal(item)
			require.NoError(t, err)
			if tc.wantContains != "" {
				require.Contains(t, string(b), tc.wantContains)
			}
			if tc.wantNotContains != "" {
				require.NotContains(t, string(b), tc.wantNotContains)
			}
		})
	}

	t.Run("IntPtr helper", func(t *testing.T) {
		require.Nil(t, IntPtr(0))
		require.Equal(t, 45000, *IntPtr(45000))
	})
}

func TestInventoryItem_CertImageURL_Serialization(t *testing.T) {
	tests := []struct {
		name      string
		item      InventoryItem
		wantFront string
		wantBack  string
		hasFront  bool
		hasBack   bool
	}{
		{
			name: "both URLs set",
			item: InventoryItem{
				DHCardID:          1,
				CertNumber:        "c1",
				GradingCompany:    "psa",
				Grade:             9.0,
				CostBasisCents:    100,
				CertImageURLFront: "https://example.com/front.jpg",
				CertImageURLBack:  "https://example.com/back.jpg",
			},
			wantFront: "https://example.com/front.jpg",
			wantBack:  "https://example.com/back.jpg",
			hasFront:  true,
			hasBack:   true,
		},
		{
			name: "only front URL set",
			item: InventoryItem{
				DHCardID:          1,
				CertNumber:        "c1",
				GradingCompany:    "psa",
				Grade:             9.0,
				CostBasisCents:    100,
				CertImageURLFront: "https://example.com/front.jpg",
			},
			wantFront: "https://example.com/front.jpg",
			hasFront:  true,
			hasBack:   false,
		},
		{
			name: "neither URL set",
			item: InventoryItem{
				DHCardID:       1,
				CertNumber:     "c1",
				GradingCompany: "psa",
				Grade:          9.0,
				CostBasisCents: 100,
			},
			hasFront: false,
			hasBack:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.item)
			require.NoError(t, err)

			var got map[string]any
			require.NoError(t, json.Unmarshal(b, &got))

			if tt.hasFront {
				require.Equal(t, tt.wantFront, got["cert_image_url_front"])
			} else {
				_, present := got["cert_image_url_front"]
				require.False(t, present, "cert_image_url_front should be omitted when empty")
			}
			if tt.hasBack {
				require.Equal(t, tt.wantBack, got["cert_image_url_back"])
			} else {
				_, present := got["cert_image_url_back"]
				require.False(t, present, "cert_image_url_back should be omitted when empty")
			}
		})
	}
}

func TestInventoryUpdate_CertImageURL_Serialization(t *testing.T) {
	upd := InventoryUpdate{
		Status:            "listed",
		CertImageURLFront: "https://example.com/front.jpg",
		CertImageURLBack:  "https://example.com/back.jpg",
	}
	b, err := json.Marshal(upd)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, "https://example.com/front.jpg", got["cert_image_url_front"])
	require.Equal(t, "https://example.com/back.jpg", got["cert_image_url_back"])

	emptyUpd := InventoryUpdate{Status: "listed"}
	b, err = json.Marshal(emptyUpd)
	require.NoError(t, err)
	got = nil
	require.NoError(t, json.Unmarshal(b, &got))
	_, frontPresent := got["cert_image_url_front"]
	_, backPresent := got["cert_image_url_back"]
	require.False(t, frontPresent)
	require.False(t, backPresent)
}
