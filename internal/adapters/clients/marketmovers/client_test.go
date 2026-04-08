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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}))
	defer srv.Close()

	c := marketmovers.NewClient(
		marketmovers.WithClientBaseURL(srv.URL),
		marketmovers.WithStaticToken("test-token"),
	)

	resp, err := c.SearchCollectibles(context.Background(), "Charizard PSA 10", 0, 5)
	if err != nil {
		t.Fatalf("SearchCollectibles: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].Item.ID != 12345 {
		t.Errorf("expected ID 12345, got %d", resp.Items[0].Item.ID)
	}
}

func TestClient_FetchCompletedSummaries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/private.sales.completedSummaries" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data := map[string]any{
			"items": []map[string]any{
				{"collectibleId": float64(99), "formattedDate": "2026-04-01", "totalSalesCount": 3, "averageSalePrice": 150.0},
				{"collectibleId": float64(99), "formattedDate": "2026-04-02", "totalSalesCount": 2, "averageSalePrice": 160.0},
			},
			"totalCount": 2,
		}
		if _, err := w.Write(buildTRPCResponse(t, data)); err != nil {
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
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].AverageSalePrice != 150.0 {
		t.Errorf("expected avg 150.0, got %v", resp.Items[0].AverageSalePrice)
	}
}

func TestClient_FetchDailyStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/private.collectibles.stats.dailyStatsV2" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data := map[string]any{
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
		}
		if _, err := w.Write(buildTRPCResponse(t, data)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	c := marketmovers.NewClient(
		marketmovers.WithClientBaseURL(srv.URL),
		marketmovers.WithStaticToken("test-token"),
	)

	resp, err := c.FetchDailyStats(context.Background(), 77, time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchDailyStats: %v", err)
	}
	if len(resp.DailyStats) != 1 {
		t.Fatalf("expected 1 day, got %d", len(resp.DailyStats))
	}
	item := resp.DailyStats[0]
	if item.CollectibleID != 77 {
		t.Errorf("expected collectibleId 77, got %d", item.CollectibleID)
	}
	if item.AverageSalePrice != 250.0 {
		t.Errorf("expected avg 250.0, got %v", item.AverageSalePrice)
	}
	if item.TotalSalesCount != 5 {
		t.Errorf("expected count 5, got %d", item.TotalSalesCount)
	}
}

func TestClient_AvgRecentPrice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/private.collectibles.stats.dailyStatsV2" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data := map[string]any{
			"dailyStats": []map[string]any{
				{"collectibleId": float64(42), "formattedDate": "2026-04-01", "totalSalesCount": 4, "averageSalePrice": 200.0},
				{"collectibleId": float64(42), "formattedDate": "2026-04-02", "totalSalesCount": 1, "averageSalePrice": 100.0},
			},
		}
		if _, err := w.Write(buildTRPCResponse(t, data)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	c := marketmovers.NewClient(
		marketmovers.WithClientBaseURL(srv.URL),
		marketmovers.WithStaticToken("test-token"),
	)

	// 4 sales at $200 + 1 sale at $100 → weighted avg = $180
	avg, err := c.AvgRecentPrice(context.Background(), 42, 30)
	if err != nil {
		t.Fatalf("AvgRecentPrice: %v", err)
	}
	want := (4*200.0 + 1*100.0) / 5.0
	if avg != want {
		t.Errorf("expected avg %v, got %v", want, avg)
	}
}

func TestClient_AvgRecentPrice_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/private.collectibles.stats.dailyStatsV2" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data := map[string]any{"dailyStats": []any{}}
		if _, err := w.Write(buildTRPCResponse(t, data)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	c := marketmovers.NewClient(
		marketmovers.WithClientBaseURL(srv.URL),
		marketmovers.WithStaticToken("test-token"),
	)

	avg, err := c.AvgRecentPrice(context.Background(), 99, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if avg != 0 {
		t.Errorf("expected 0 for empty, got %v", avg)
	}
}

func TestClient_TRPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"error":{"message":"Unauthorized","code":-32003}}`); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	c := marketmovers.NewClient(
		marketmovers.WithClientBaseURL(srv.URL),
		marketmovers.WithStaticToken("test-token"),
	)

	_, err := c.SearchCollectibles(context.Background(), "Pikachu", 0, 5)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseJWTExpiry(t *testing.T) {
	// Build a minimal JWT: header.payload.sig
	// payload: {"exp": 9999999999}

	// Use the exported helper
	future := time.Unix(9999999999, 0)
	tok := buildJWT(t, 9999999999)
	expiry := marketmovers.ParseJWTExpiry(tok)

	// Allow ±5s tolerance
	if expiry.Before(future.Add(-5*time.Second)) || expiry.After(future.Add(5*time.Second)) {
		t.Errorf("expected expiry ~%v, got %v", future, expiry)
	}
}

func TestParseJWTExpiry_InvalidToken(t *testing.T) {
	expiry := marketmovers.ParseJWTExpiry("not.a.jwt")
	// Should fall back to ~55 min from now
	now := time.Now()
	if expiry.Before(now.Add(50*time.Minute)) || expiry.After(now.Add(60*time.Minute)) {
		t.Errorf("unexpected fallback expiry: %v", expiry)
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
