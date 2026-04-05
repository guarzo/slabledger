package dh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

func newTestClient(serverURL string) *Client {
	config := httpx.DefaultConfig("DH")
	config.DefaultTimeout = 5 * time.Second
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	httpClient := httpx.NewClient(config)

	return &Client{
		apiKey:        "test_api_key",
		enterpriseKey: "test_api_key",
		baseURL:       serverURL,
		httpClient:    httpClient,
		limiter:       rate.NewLimiter(rate.Inf, 1),
		timeout:       5 * time.Second,
	}
}

func TestClient_Available(t *testing.T) {
	t.Run("with key", func(t *testing.T) {
		c := NewClient("", "test_key")
		if !c.Available() {
			t.Error("expected Available() = true")
		}
	})

	t.Run("without key", func(t *testing.T) {
		c := NewClient("", "")
		if c.Available() {
			t.Error("expected Available() = false")
		}
	})
}

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get(apiKeyHeader) != "test_api_key" {
			t.Errorf("expected %s header = test_api_key, got %q", apiKeyHeader, r.Header.Get(apiKeyHeader))
		}
		if r.URL.Path != "/api/v1/integrations/catalog/search" {
			t.Errorf("expected path /api/v1/integrations/catalog/search, got %s", r.URL.Path)
		}
		if q := r.URL.Query().Get("q"); q != "Charizard Base Set" {
			t.Errorf("expected q=Charizard Base Set, got %q", q)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"cards": [
				{
					"id": 247,
					"name": "Charizard [1st Edition]",
					"set": "Pokemon Base Set",
					"number": "4",
					"image_url": "https://example.com/charizard.png"
				}
			]
		}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.Search(context.Background(), "Charizard Base Set", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(resp.Cards) != 1 {
		t.Fatalf("len(Cards) = %d, want 1", len(resp.Cards))
	}
	card := resp.Cards[0]
	if card.ID != 247 {
		t.Errorf("ID = %d, want 247", card.ID)
	}
	if card.Name != "Charizard [1st Edition]" {
		t.Errorf("Name = %q, want Charizard [1st Edition]", card.Name)
	}
	if card.SetName != "Pokemon Base Set" {
		t.Errorf("SetName = %q, want Pokemon Base Set", card.SetName)
	}
	if card.Number != "4" {
		t.Errorf("Number = %q, want 4", card.Number)
	}
	if card.ImageURL != "https://example.com/charizard.png" {
		t.Errorf("ImageURL = %q, want https://example.com/charizard.png", card.ImageURL)
	}
}

func TestClient_Match(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get(apiKeyHeader) != "test_api_key" {
			t.Errorf("expected %s header = test_api_key, got %q", apiKeyHeader, r.Header.Get(apiKeyHeader))
		}
		if r.URL.Path != "/api/v1/integrations/match" {
			t.Errorf("expected path /api/v1/integrations/match, got %s", r.URL.Path)
		}

		var req MatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if req.Title != "Charizard Base Set PSA 10" {
			t.Errorf("expected title='Charizard Base Set PSA 10', got %q", req.Title)
		}
		if req.SKU != "PSA-12345" {
			t.Errorf("expected sku='PSA-12345', got %q", req.SKU)
		}

		resp := MatchResponse{
			Success:     true,
			CardID:      42,
			CardTitle:   "Charizard Base Set 4/102",
			Confidence:  0.95,
			MatchMethod: "title_match",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.Match(context.Background(), "Charizard Base Set PSA 10", "PSA-12345")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if !resp.Success {
		t.Error("expected Success = true")
	}
	if resp.CardID != 42 {
		t.Errorf("CardID = %d, want 42", resp.CardID)
	}
	if resp.CardTitle != "Charizard Base Set 4/102" {
		t.Errorf("CardTitle = %q, want Charizard Base Set 4/102", resp.CardTitle)
	}
	if resp.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", resp.Confidence)
	}
	if resp.MatchMethod != "title_match" {
		t.Errorf("MatchMethod = %q, want title_match", resp.MatchMethod)
	}
}

func TestClient_MarketData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get(apiKeyHeader) != "test_api_key" {
			t.Errorf("expected %s header = test_api_key, got %q", apiKeyHeader, r.Header.Get(apiKeyHeader))
		}
		if r.URL.Path != "/api/v1/integrations/market_data/card_123" {
			t.Errorf("expected path /api/v1/integrations/market_data/card_123, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tier": 3,
			"has_data": true,
			"card_id": 247,
			"card_title": "Charizard Base Set 4/102",
			"current_price": 14875.00,
			"period_low": 12000.00,
			"period_high": 16500.00,
			"price_change": 875.00,
			"price_change_pct": 6.25,
			"price_history": [],
			"periods": {
				"30d": {
					"current_price": 14875.00,
					"period_low": 13500.00,
					"period_high": 15200.00,
					"price_change": 500.00,
					"price_change_pct": 3.48,
					"price_history": []
				}
			},
			"recent_sales": [
				{
					"sold_at": "2026-03-30",
					"grading_company": "PSA",
					"grade": "10",
					"price": 15200.00,
					"platform": "eBay"
				}
			],
			"population": [],
			"insights": null,
			"sentiment": {
				"score": 0.82,
				"mention_count": 45,
				"trend": "bullish"
			},
			"grading_roi": {
				"card": {"id": 247, "name": "Charizard", "set_name": "Base Set"},
				"roi_data": [
					{"grade": "PSA 10", "avg_sale_price": 15000.00, "roi": 0.42}
				]
			},
			"price_forecast": null
		}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.MarketData(context.Background(), "card_123")
	if err != nil {
		t.Fatalf("MarketData() error = %v", err)
	}
	if !resp.HasData {
		t.Error("expected HasData = true")
	}
	if resp.CardID != 247 {
		t.Errorf("CardID = %d, want 247", resp.CardID)
	}
	if resp.CurrentPrice != 14875.00 {
		t.Errorf("CurrentPrice = %v, want 14875.00", resp.CurrentPrice)
	}
	if resp.Tier != 3 {
		t.Errorf("Tier = %d, want 3", resp.Tier)
	}
	if resp.PriceChangePct != 6.25 {
		t.Errorf("PriceChangePct = %v, want 6.25", resp.PriceChangePct)
	}

	period30d, ok := resp.Periods["30d"]
	if !ok {
		t.Fatal("expected 30d period")
	}
	if period30d.PriceChange != 500.00 {
		t.Errorf("30d PriceChange = %v, want 500.00", period30d.PriceChange)
	}

	if len(resp.RecentSales) != 1 {
		t.Fatalf("len(RecentSales) = %d, want 1", len(resp.RecentSales))
	}
	if resp.RecentSales[0].Platform != "eBay" {
		t.Errorf("RecentSales[0].Platform = %q, want eBay", resp.RecentSales[0].Platform)
	}

	if resp.Sentiment == nil {
		t.Fatal("expected non-nil Sentiment")
	}
	if resp.Sentiment.Score != 0.82 {
		t.Errorf("Sentiment.Score = %v, want 0.82", resp.Sentiment.Score)
	}
	if resp.Sentiment.MentionCount != 45 {
		t.Errorf("Sentiment.MentionCount = %d, want 45", resp.Sentiment.MentionCount)
	}

	if resp.GradingROI == nil {
		t.Fatal("expected non-nil GradingROI")
	}
	if len(resp.GradingROI.ROIData) != 1 {
		t.Fatalf("len(GradingROI.ROIData) = %d, want 1", len(resp.GradingROI.ROIData))
	}
	if resp.GradingROI.ROIData[0].Grade != "PSA 10" {
		t.Errorf("GradingROI.ROIData[0].Grade = %q, want PSA 10", resp.GradingROI.ROIData[0].Grade)
	}
	if resp.GradingROI.ROIData[0].ROI != 0.42 {
		t.Errorf("GradingROI.ROIData[0].ROI = %v, want 0.42", resp.GradingROI.ROIData[0].ROI)
	}
}

func TestClient_Suggestions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get(apiKeyHeader) != "test_api_key" {
			t.Errorf("expected %s header = test_api_key, got %q", apiKeyHeader, r.Header.Get(apiKeyHeader))
		}
		if r.URL.Path != "/api/v1/integrations/suggestions" {
			t.Errorf("expected path /api/v1/integrations/suggestions, got %s", r.URL.Path)
		}

		resp := SuggestionsResponse{
			Cards: SuggestionGroup{
				HottestCards: []SuggestionItem{
					{
						Rank: 1,
						Card: SuggestionCard{
							ID:           101,
							Name:         "Charizard",
							SetName:      "Base Set",
							Number:       "4",
							CurrentPrice: 14875.00,
						},
						ConfidenceScore: 0.92,
						Reasoning:       "Strong upward trend",
					},
				},
				ConsiderSelling: []SuggestionItem{
					{
						Rank: 1,
						Card: SuggestionCard{
							ID:           202,
							Name:         "Pikachu",
							SetName:      "Jungle",
							Number:       "60",
							CurrentPrice: 250.00,
						},
						ConfidenceScore: 0.78,
						Reasoning:       "Price peaked, declining volume",
						Sentiment: &SuggestionSentiment{
							Score:        -0.3,
							Trend:        -0.15,
							MentionCount: 12,
						},
					},
				},
			},
			GeneratedAt:    "2026-04-02T10:00:00Z",
			SuggestionDate: "2026-04-02",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.Suggestions(context.Background())
	if err != nil {
		t.Fatalf("Suggestions() error = %v", err)
	}
	if resp.GeneratedAt != "2026-04-02T10:00:00Z" {
		t.Errorf("GeneratedAt = %q, want 2026-04-02T10:00:00Z", resp.GeneratedAt)
	}
	if resp.SuggestionDate != "2026-04-02" {
		t.Errorf("SuggestionDate = %q, want 2026-04-02", resp.SuggestionDate)
	}

	// Verify hottest cards
	if len(resp.Cards.HottestCards) != 1 {
		t.Fatalf("len(HottestCards) = %d, want 1", len(resp.Cards.HottestCards))
	}
	hot := resp.Cards.HottestCards[0]
	if hot.Card.Name != "Charizard" {
		t.Errorf("HottestCards[0].Card.Name = %q, want Charizard", hot.Card.Name)
	}
	if hot.ConfidenceScore != 0.92 {
		t.Errorf("HottestCards[0].ConfidenceScore = %v, want 0.92", hot.ConfidenceScore)
	}

	// Verify consider selling
	if len(resp.Cards.ConsiderSelling) != 1 {
		t.Fatalf("len(ConsiderSelling) = %d, want 1", len(resp.Cards.ConsiderSelling))
	}
	sell := resp.Cards.ConsiderSelling[0]
	if sell.Card.Name != "Pikachu" {
		t.Errorf("ConsiderSelling[0].Card.Name = %q, want Pikachu", sell.Card.Name)
	}
	if sell.Sentiment == nil {
		t.Fatal("expected non-nil Sentiment on ConsiderSelling[0]")
	}
	if sell.Sentiment.MentionCount != 12 {
		t.Errorf("ConsiderSelling[0].Sentiment.MentionCount = %d, want 12", sell.Sentiment.MentionCount)
	}
}

func TestClient_NotAvailable(t *testing.T) {
	c := NewClient("", "")
	_, err := c.Search(context.Background(), "test", 10)
	if err == nil {
		t.Error("expected error when not available")
	}
}
