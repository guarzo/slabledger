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
		apiKey:     "test_api_key",
		baseURL:    serverURL,
		httpClient: httpClient,
		limiter:    rate.NewLimiter(rate.Inf, 1),
		timeout:    5 * time.Second,
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
		if limit := r.URL.Query().Get("limit"); limit != "10" {
			t.Errorf("expected limit=10, got %q", limit)
		}

		resp := SearchResponse{
			Cards: []SearchCard{
				{
					ID:         "card_001",
					Title:      "Charizard Base Set 4/102",
					SetName:    "Base Set",
					SetCode:    "BS",
					CardNumber: "4",
					Rarity:     "Holo Rare",
					ImageURL:   "https://example.com/charizard.png",
				},
			},
			Total: 1,
			Query: "Charizard Base Set",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.Search(context.Background(), "Charizard Base Set", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
	if resp.Query != "Charizard Base Set" {
		t.Errorf("Query = %q, want Charizard Base Set", resp.Query)
	}
	if len(resp.Cards) != 1 {
		t.Fatalf("len(Cards) = %d, want 1", len(resp.Cards))
	}
	card := resp.Cards[0]
	if card.ID != "card_001" {
		t.Errorf("ID = %q, want card_001", card.ID)
	}
	if card.Title != "Charizard Base Set 4/102" {
		t.Errorf("Title = %q, want Charizard Base Set 4/102", card.Title)
	}
	if card.SetName != "Base Set" {
		t.Errorf("SetName = %q, want Base Set", card.SetName)
	}
	if card.Rarity != "Holo Rare" {
		t.Errorf("Rarity = %q, want Holo Rare", card.Rarity)
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
		if tier := r.URL.Query().Get("tier"); tier != "tier3" {
			t.Errorf("expected tier=tier3, got %q", tier)
		}

		resp := MarketDataResponse{
			Tier:           3,
			HasData:        true,
			CardID:         "card_123",
			CardTitle:      "Charizard Base Set 4/102",
			CurrentPrice:   14875.00,
			PeriodLow:      12000.00,
			PeriodHigh:     16500.00,
			PriceChange:    875.00,
			PriceChangePct: 6.25,
			Periods: map[string]Period{
				"30d": {
					CurrentPrice:   14875.00,
					PeriodLow:      13500.00,
					PeriodHigh:     15200.00,
					PriceChange:    500.00,
					PriceChangePct: 3.48,
				},
			},
			RecentSales: []RecentSale{
				{
					SoldAt:         "2026-03-30",
					GradingCompany: "PSA",
					Grade:          "10",
					Price:          15200.00,
					Platform:       "eBay",
				},
			},
			Sentiment: &SentimentData{
				Score:        0.82,
				MentionCount: 45,
				Trend:        "bullish",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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
	if resp.CardID != "card_123" {
		t.Errorf("CardID = %q, want card_123", resp.CardID)
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

	// Verify periods parsed
	period30d, ok := resp.Periods["30d"]
	if !ok {
		t.Fatal("expected 30d period")
	}
	if period30d.PriceChange != 500.00 {
		t.Errorf("30d PriceChange = %v, want 500.00", period30d.PriceChange)
	}

	// Verify recent sales
	if len(resp.RecentSales) != 1 {
		t.Fatalf("len(RecentSales) = %d, want 1", len(resp.RecentSales))
	}
	if resp.RecentSales[0].Platform != "eBay" {
		t.Errorf("RecentSales[0].Platform = %q, want eBay", resp.RecentSales[0].Platform)
	}

	// Verify sentiment parsed
	if resp.Sentiment == nil {
		t.Fatal("expected non-nil Sentiment")
	}
	if resp.Sentiment.Score != 0.82 {
		t.Errorf("Sentiment.Score = %v, want 0.82", resp.Sentiment.Score)
	}
	if resp.Sentiment.MentionCount != 45 {
		t.Errorf("Sentiment.MentionCount = %d, want 45", resp.Sentiment.MentionCount)
	}
	if resp.Sentiment.Trend != "bullish" {
		t.Errorf("Sentiment.Trend = %q, want bullish", resp.Sentiment.Trend)
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
