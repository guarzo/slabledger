package dh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_BatchAnalytics_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/cards/batch_analytics", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))

		var body struct {
			CardIDs []int    `json:"card_ids"`
			Fields  []string `json:"fields"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, []int{1, 2}, body.CardIDs)
		require.Equal(t, []string{"velocity", "trend"}, body.Fields)

		resp := BatchAnalyticsResponse{
			Results: []CardAnalytics{
				{
					CardID:     1,
					CardName:   "Dreepy",
					ComputedAt: "2026-04-15T00:00:00Z",
					Velocity: &VelocityMetrics{
						MedianDaysToSell: "12.5",
						AvgDaysToSell:    "14.0",
						SellThrough:      map[string]string{"30d": "0.42", "60d": "0.61", "90d": "0.78"},
						SampleSize:       18,
					},
					Trend: &TrendMetrics{
						Direction7d:  "up",
						ChangePct7d:  "3.4",
						Volume7d:     12,
						Direction30d: "flat",
						ChangePct30d: "0.2",
						Volume30d:    45,
						Direction90d: "up",
						ChangePct90d: "8.1",
						Volume90d:    120,
					},
				},
				{
					CardID: 2,
					Error:  "analytics_not_computed",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.BatchAnalytics(context.Background(), []int{1, 2}, []string{"velocity", "trend"})
	require.NoError(t, err)
	require.Len(t, resp.Results, 2)

	first := resp.Results[0]
	require.Equal(t, 1, first.CardID)
	require.NotNil(t, first.Velocity)
	require.Equal(t, "12.5", first.Velocity.MedianDaysToSell)
	require.Equal(t, 18, first.Velocity.SampleSize)
	require.NotNil(t, first.Trend)
	require.Equal(t, "up", first.Trend.Direction7d)

	second := resp.Results[1]
	require.Equal(t, 2, second.CardID)
	require.Nil(t, second.Velocity)
	require.Equal(t, "analytics_not_computed", second.Error)
}

func TestClient_BatchAnalytics_AutoChunks150To2Calls(t *testing.T) {
	var (
		callCount  int32
		mu         sync.Mutex
		seenBodies []struct {
			CardIDs []int    `json:"card_ids"`
			Fields  []string `json:"fields"`
		}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		var body struct {
			CardIDs []int    `json:"card_ids"`
			Fields  []string `json:"fields"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		seenBodies = append(seenBodies, body)
		mu.Unlock()

		results := make([]CardAnalytics, 0, len(body.CardIDs))
		for _, id := range body.CardIDs {
			results = append(results, CardAnalytics{CardID: id})
		}
		_ = json.NewEncoder(w).Encode(BatchAnalyticsResponse{Results: results})
	}))
	defer server.Close()

	ids := make([]int, 150)
	for i := range ids {
		ids[i] = i + 1
	}

	c := newTestClient(server.URL)
	resp, err := c.BatchAnalytics(context.Background(), ids, []string{"velocity"})
	require.NoError(t, err)

	require.EqualValues(t, 2, atomic.LoadInt32(&callCount))
	require.Len(t, seenBodies, 2)
	require.Len(t, seenBodies[0].CardIDs, 100)
	require.Len(t, seenBodies[1].CardIDs, 50)
	require.Len(t, resp.Results, 150)
	require.Equal(t, 1, resp.Results[0].CardID)
	require.Equal(t, 150, resp.Results[149].CardID)
}

func TestClient_BatchAnalytics_EmptyInput(t *testing.T) {
	// No HTTP calls should occur for empty input.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call for empty input")
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.BatchAnalytics(context.Background(), nil, []string{"velocity"})
	require.NoError(t, err)
	require.Empty(t, resp.Results)
}

func TestClient_DemandSignals_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/enterprise/market/demand_signals", r.URL.Path)
		require.Equal(t, "7d", r.URL.Query().Get("window"))
		require.ElementsMatch(t, []string{"1", "2"}, r.URL.Query()["card_ids[]"])

		resp := DemandSignalsResponse{
			DemandSignals: []DemandSignal{
				{
					CardID:      1,
					DemandScore: 0.78,
					DataQuality: "full",
					Views:       intPtr(1200),
					RawSales24h: 4,
				},
				{
					CardID:      2,
					DemandScore: 0.35,
					DataQuality: "proxy",
					RawSales24h: 0,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.DemandSignals(context.Background(), []int{1, 2}, "7d")
	require.NoError(t, err)
	require.Len(t, resp.DemandSignals, 2)
	require.Equal(t, "full", resp.DemandSignals[0].DataQuality)
	require.NotNil(t, resp.DemandSignals[0].Views)
	require.Equal(t, 1200, *resp.DemandSignals[0].Views)
	require.Equal(t, "proxy", resp.DemandSignals[1].DataQuality)
}

func TestClient_DemandSignals_AutoChunks120To3Calls(t *testing.T) {
	var (
		callCount int32
		mu        sync.Mutex
		sizes     []int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		ids := r.URL.Query()["card_ids[]"]
		mu.Lock()
		sizes = append(sizes, len(ids))
		mu.Unlock()

		signals := make([]DemandSignal, 0, len(ids))
		for _, id := range ids {
			signals = append(signals, DemandSignal{CardID: atoiOrZero(id), DemandScore: 0.5, DataQuality: "full"})
		}
		_ = json.NewEncoder(w).Encode(DemandSignalsResponse{DemandSignals: signals})
	}))
	defer server.Close()

	ids := make([]int, 120)
	for i := range ids {
		ids[i] = i + 1
	}

	c := newTestClient(server.URL)
	resp, err := c.DemandSignals(context.Background(), ids, "7d")
	require.NoError(t, err)

	require.EqualValues(t, 3, atomic.LoadInt32(&callCount))
	require.ElementsMatch(t, []int{50, 50, 20}, sizes)
	require.Len(t, resp.DemandSignals, 120)
}

func TestClient_CharacterDemand_HappyPathWithByEra(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/enterprise/market/demand_signals/character_demand", r.URL.Path)
		require.Equal(t, "true", r.URL.Query().Get("by_era"))
		require.Equal(t, "30d", r.URL.Query().Get("window"))
		require.ElementsMatch(t, []string{"11", "22"}, r.URL.Query()["card_ids[]"])

		resp := CharacterDemandResponse{
			CharacterDemand: []CharacterDemandEntry{
				{
					CharacterName:     "Dreepy",
					CardCount:         3,
					AvgDemandScore:    0.71,
					TotalViews:        4800,
					TotalSearchClicks: 120,
					TotalWishlistAdds: 38,
					ByEra: map[string]ByEraEntry{
						"sword_shield": {
							CardCount:      2,
							AvgDemandScore: 0.68,
							TotalViews:     3000,
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.CharacterDemand(context.Background(), []int{11, 22}, "30d", true)
	require.NoError(t, err)
	require.Len(t, resp.CharacterDemand, 1)
	entry := resp.CharacterDemand[0]
	require.Equal(t, "Dreepy", entry.CharacterName)
	require.Equal(t, 3, entry.CardCount)
	require.Contains(t, entry.ByEra, "sword_shield")
	require.Equal(t, 2, entry.ByEra["sword_shield"].CardCount)
}

func TestClient_TopCharacters_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/enterprise/market/demand_signals/top_characters", r.URL.Path)
		require.Equal(t, "25", r.URL.Query().Get("limit"))
		require.Equal(t, "sword_shield", r.URL.Query().Get("era"))

		era := "sword_shield"
		resp := TopCharactersResponse{
			CharacterDemand: []CharacterDemandEntry{
				{CharacterName: "Charizard", CardCount: 5, AvgDemandScore: 0.9, TotalViews: 20000},
			},
			ComputedAt: "2026-04-15T02:00:00Z",
			Era:        &era,
			Limit:      25,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.TopCharacters(context.Background(), 25, "sword_shield")
	require.NoError(t, err)
	require.Len(t, resp.CharacterDemand, 1)
	require.Equal(t, "Charizard", resp.CharacterDemand[0].CharacterName)
	require.NotNil(t, resp.Era)
	require.Equal(t, "sword_shield", *resp.Era)
}

func TestClient_CharacterVelocity_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/enterprise/characters/velocity", r.URL.Path)
		require.Equal(t, "median_days_to_sell", r.URL.Query().Get("sort_by"))
		require.Equal(t, "2", r.URL.Query().Get("page"))
		require.Equal(t, "25", r.URL.Query().Get("per_page"))

		resp := CharacterVelocityResponse{
			Characters: []CharacterVelocityEntry{
				{
					CharacterName: "Dreepy",
					CardCount:     8,
					ComputedAt:    "2026-04-15T03:15:00Z",
					Velocity: CharacterVelocityFields{
						MedianDaysToSell:  14.0,
						AvgDaysToSell:     16.0,
						SellThrough:       map[string]float64{"30d": 0.4, "60d": 0.6, "90d": 0.8},
						SampleSize:        25,
						VelocityChangePct: 3.2,
					},
				},
			},
			Pagination: PaginationMeta{Page: 2, PerPage: 25, TotalCount: 200},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.CharacterVelocity(context.Background(), CharacterListOpts{
		SortBy:  "median_days_to_sell",
		Page:    2,
		PerPage: 25,
	})
	require.NoError(t, err)
	require.Len(t, resp.Characters, 1)
	require.Equal(t, 14.0, resp.Characters[0].Velocity.MedianDaysToSell)
	require.Equal(t, 2, resp.Pagination.Page)
}

func TestClient_CharacterSaturation_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/enterprise/characters/saturation", r.URL.Path)
		require.Equal(t, "active_listing_count", r.URL.Query().Get("sort_by"))

		resp := CharacterSaturationResponse{
			Characters: []CharacterSaturationEntry{
				{
					CharacterName: "Dreepy",
					CardCount:     8,
					ComputedAt:    "2026-04-15T03:15:00Z",
					Saturation: CharacterSaturationFields{
						ActiveListingCount: 1200,
						ActiveAskCount:     240,
						PriceSpreadPct:     0.18,
						LowestAskPrice:     12.50,
						HighestAskPrice:    25.00,
					},
				},
			},
			Pagination: PaginationMeta{Page: 1, PerPage: 25, TotalCount: 200},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.CharacterSaturation(context.Background(), CharacterListOpts{SortBy: "active_listing_count"})
	require.NoError(t, err)
	require.Len(t, resp.Characters, 1)
	require.Equal(t, 1200, resp.Characters[0].Saturation.ActiveListingCount)
}

func TestClient_CharacterLeaderboard_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/enterprise/characters/leaderboard", r.URL.Path)
		require.Equal(t, "velocity_change_pct", r.URL.Query().Get("metric"))
		require.Equal(t, "sword_shield", r.URL.Query().Get("era"))
		require.Equal(t, "10", r.URL.Query().Get("grade"))
		require.Equal(t, "25", r.URL.Query().Get("per_page"))

		resp := LeaderboardResponse{
			Leaderboard: []LeaderboardEntry{
				{Rank: 1, CharacterName: "Charizard", CardCount: 6, Value: 12.4, Metric: "velocity_change_pct"},
			},
			Metric:     "velocity_change_pct",
			Filters:    map[string]any{"era": "sword_shield", "grade": "10"},
			Pagination: PaginationMeta{Page: 1, PerPage: 25, TotalCount: 1},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.CharacterLeaderboard(context.Background(), "velocity_change_pct", "sword_shield", 10, 25)
	require.NoError(t, err)
	require.Len(t, resp.Leaderboard, 1)
	require.Equal(t, "Charizard", resp.Leaderboard[0].CharacterName)
	require.Equal(t, "velocity_change_pct", resp.Metric)
}

func TestClient_AnalyticsNotComputed_Sentinel(t *testing.T) {
	// Single-call endpoints should translate a 404 body of
	// `{"error":"analytics_not_computed"}` into ErrAnalyticsNotComputed.
	tests := []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "top_characters",
			call: func(c *Client) error {
				_, err := c.TopCharacters(context.Background(), 10, "")
				return err
			},
		},
		{
			name: "character_velocity",
			call: func(c *Client) error {
				_, err := c.CharacterVelocity(context.Background(), CharacterListOpts{})
				return err
			},
		},
		{
			name: "character_saturation",
			call: func(c *Client) error {
				_, err := c.CharacterSaturation(context.Background(), CharacterListOpts{})
				return err
			},
		},
		{
			name: "character_leaderboard",
			call: func(c *Client) error {
				_, err := c.CharacterLeaderboard(context.Background(), "", "", 0, 0)
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"error":"analytics_not_computed"}`)
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			err := tc.call(c)
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrAnalyticsNotComputed),
				"expected ErrAnalyticsNotComputed, got %v", err)
		})
	}
}

func TestClient_AnalyticsNotComputed_OtherErrorsPassThrough(t *testing.T) {
	// A 404 whose body is NOT analytics_not_computed should surface as the
	// normal provider-not-found error, not the sentinel.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"character_not_found"}`)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	_, err := c.TopCharacters(context.Background(), 10, "")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrAnalyticsNotComputed))
	require.Contains(t, strings.ToLower(err.Error()), "not found")
}

// atoiOrZero is a tiny helper used by the chunking test to turn query-string
// card_ids back into ints for fixture synthesis.
func atoiOrZero(s string) int {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0
	}
	return n
}
