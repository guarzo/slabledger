package marketmovers_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
)

// buildTRPCResponse wraps data in the standard tRPC envelope.
func buildTRPCResponse(t *testing.T, data any) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"result": map[string]any{"data": data},
	})
	if err != nil {
		t.Fatalf("marshal trpc response: %v", err)
	}
	return payload
}

func TestClient_SearchCollectibles(t *testing.T) {
	cases := []struct {
		name        string
		handler     http.HandlerFunc
		wantErr     bool
		wantItems   int
		wantFirstID int64
	}{
		{
			name: "returns matching item",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/private.collectibles.search" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				data := map[string]any{
					"items": []map[string]any{
						{
							"item": map[string]any{
								"id":              float64(12345),
								"searchTitle":     "Charizard PSA 10",
								"collectibleType": "sports-card",
							},
						},
					},
				}
				if _, err := w.Write(buildTRPCResponse(t, data)); err != nil {
					t.Errorf("write response: %v", err)
				}
			},
			wantErr:     false,
			wantItems:   1,
			wantFirstID: 12345,
		},
		{
			name: "trpc error body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := fmt.Fprint(w, `{"error":{"message":"Unauthorized","code":-32003}}`); err != nil {
					t.Errorf("write: %v", err)
				}
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			c := marketmovers.NewClient(
				marketmovers.WithClientBaseURL(srv.URL),
				marketmovers.WithStaticToken("test-token"),
			)

			resp, err := c.SearchCollectibles(context.Background(), "Charizard PSA 10", 0, 5)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("SearchCollectibles: %v", err)
			}
			if len(resp.Items) != tc.wantItems {
				t.Fatalf("expected %d items, got %d", tc.wantItems, len(resp.Items))
			}
			if tc.wantItems > 0 && resp.Items[0].Item.ID != tc.wantFirstID {
				t.Errorf("expected ID %d, got %d", tc.wantFirstID, resp.Items[0].Item.ID)
			}
		})
	}
}

func TestClient_FetchCompletedSummaries(t *testing.T) {
	cases := []struct {
		name         string
		responseData map[string]any
		wantLen      int
		wantFirstAvg float64
	}{
		{
			name: "two items",
			responseData: map[string]any{
				"items": []map[string]any{
					{"collectibleId": float64(99), "formattedDate": "2026-04-01", "totalSalesCount": 3, "averageSalePrice": 150.0},
					{"collectibleId": float64(99), "formattedDate": "2026-04-02", "totalSalesCount": 2, "averageSalePrice": 160.0},
				},
				"totalCount": 2,
			},
			wantLen:      2,
			wantFirstAvg: 150.0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/private.sales.completedSummaries" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write(buildTRPCResponse(t, tc.responseData)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer srv.Close()

			c := marketmovers.NewClient(
				marketmovers.WithClientBaseURL(srv.URL),
				marketmovers.WithStaticToken("test-token"),
			)

			resp, err := c.FetchCompletedSummaries(context.Background(), 99, 30)
			if err != nil {
				t.Fatalf("FetchCompletedSummaries: %v", err)
			}
			if len(resp.Items) != tc.wantLen {
				t.Fatalf("expected %d items, got %d", tc.wantLen, len(resp.Items))
			}
			if resp.Items[0].AverageSalePrice != tc.wantFirstAvg {
				t.Errorf("expected avg %v, got %v", tc.wantFirstAvg, resp.Items[0].AverageSalePrice)
			}
		})
	}
}

func TestClient_FetchDailyStats(t *testing.T) {
	cases := []struct {
		name          string
		responseData  map[string]any
		collectibleID int64
		wantLen       int
		wantAvg       float64
		wantCount     int
	}{
		{
			name:          "single day",
			collectibleID: 77,
			responseData: map[string]any{
				"dailyStats": []map[string]any{
					{
						"collectibleId":    float64(77),
						"formattedDate":    "2026-03-09",
						"totalSalesCount":  5,
						"averageSalePrice": 250.0,
						"minSalePrice":     200.0,
						"maxSalePrice":     300.0,
					},
				},
			},
			wantLen:   1,
			wantAvg:   250.0,
			wantCount: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/private.collectibles.stats.dailyStatsV2" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write(buildTRPCResponse(t, tc.responseData)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer srv.Close()

			c := marketmovers.NewClient(
				marketmovers.WithClientBaseURL(srv.URL),
				marketmovers.WithStaticToken("test-token"),
			)

			resp, err := c.FetchDailyStats(context.Background(), tc.collectibleID, time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("FetchDailyStats: %v", err)
			}
			if len(resp.DailyStats) != tc.wantLen {
				t.Fatalf("expected %d day(s), got %d", tc.wantLen, len(resp.DailyStats))
			}
			item := resp.DailyStats[0]
			if item.CollectibleID != tc.collectibleID {
				t.Errorf("expected collectibleId %d, got %d", tc.collectibleID, item.CollectibleID)
			}
			if item.AverageSalePrice != tc.wantAvg {
				t.Errorf("expected avg %v, got %v", tc.wantAvg, item.AverageSalePrice)
			}
			if item.TotalSalesCount != tc.wantCount {
				t.Errorf("expected count %d, got %d", tc.wantCount, item.TotalSalesCount)
			}
		})
	}
}

func TestClient_AvgRecentPrice(t *testing.T) {
	cases := []struct {
		name          string
		dailyStats    any
		collectibleID int64
		wantAvg       float64
	}{
		{
			name:          "weighted average of multiple days",
			collectibleID: 42,
			dailyStats: []map[string]any{
				{"collectibleId": float64(42), "formattedDate": "2026-04-01", "totalSalesCount": 4, "averageSalePrice": 200.0},
				{"collectibleId": float64(42), "formattedDate": "2026-04-02", "totalSalesCount": 1, "averageSalePrice": 100.0},
			},
			// 4 sales at $200 + 1 sale at $100 → weighted avg = $180
			wantAvg: (4*200.0 + 1*100.0) / 5.0,
		},
		{
			name:          "empty — no sales",
			collectibleID: 99,
			dailyStats:    []any{},
			wantAvg:       0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/private.collectibles.stats.dailyStatsV2" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				data := map[string]any{"dailyStats": tc.dailyStats}
				if _, err := w.Write(buildTRPCResponse(t, data)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer srv.Close()

			c := marketmovers.NewClient(
				marketmovers.WithClientBaseURL(srv.URL),
				marketmovers.WithStaticToken("test-token"),
			)

			avg, err := c.AvgRecentPrice(context.Background(), tc.collectibleID, 30)
			if err != nil {
				t.Fatalf("AvgRecentPrice: %v", err)
			}
			if avg != tc.wantAvg {
				t.Errorf("expected avg %v, got %v", tc.wantAvg, avg)
			}
		})
	}
}

func TestParseJWTExpiry(t *testing.T) {
	cases := []struct {
		name      string
		token     string
		wantMin   time.Duration // minimum expected offset from now
		wantMax   time.Duration // maximum expected offset from now
		fixedTime *time.Time    // if non-nil, assert exact value instead of range
	}{
		{
			name:  "valid jwt with far-future exp",
			token: buildJWT(t, 9999999999),
			// should equal time.Unix(9999999999, 0) within ±5s
			fixedTime: func() *time.Time { t := time.Unix(9999999999, 0); return &t }(),
		},
		{
			name:    "invalid token falls back to ~55 min",
			token:   "not.a.jwt",
			wantMin: 50 * time.Minute,
			wantMax: 60 * time.Minute,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expiry := marketmovers.ParseJWTExpiry(tc.token)
			if tc.fixedTime != nil {
				if expiry.Before(tc.fixedTime.Add(-5*time.Second)) || expiry.After(tc.fixedTime.Add(5*time.Second)) {
					t.Errorf("expected expiry ~%v, got %v", *tc.fixedTime, expiry)
				}
				return
			}
			now := time.Now()
			if expiry.Before(now.Add(tc.wantMin)) || expiry.After(now.Add(tc.wantMax)) {
				t.Errorf("expected expiry in [now+%v, now+%v], got %v", tc.wantMin, tc.wantMax, expiry)
			}
		})
	}
}

// buildJWT creates a minimal JWT with the given exp claim (no signature verification).
func buildJWT(t *testing.T, exp int64) string {
	t.Helper()

	header := b64url(t, `{"alg":"ES256","typ":"JWT"}`)
	payload := b64url(t, fmt.Sprintf(`{"exp":%d}`, exp))
	return header + "." + payload + ".fakesig"
}

func b64url(t *testing.T, s string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
