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
		enterpriseKey: "test_api_key",
		baseURL:       serverURL,
		httpClient:    httpClient,
		limiter:       rate.NewLimiter(rate.Inf, 1),
		timeout:       5 * time.Second,
	}
}

func TestClient_EnterpriseAvailable(t *testing.T) {
	t.Run("with key", func(t *testing.T) {
		c := NewClient("", WithEnterpriseKey("test_key"))
		if !c.EnterpriseAvailable() {
			t.Error("expected EnterpriseAvailable() = true")
		}
	})

	t.Run("without key", func(t *testing.T) {
		c := NewClient("")
		if c.EnterpriseAvailable() {
			t.Error("expected EnterpriseAvailable() = false")
		}
	})
}

func TestClient_Suggestions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test_api_key" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/api/v1/enterprise/suggestions" {
			t.Errorf("expected path /api/v1/enterprise/suggestions, got %s", r.URL.Path)
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

func TestClient_CardLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test_api_key" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/api/v1/enterprise/cards/lookup" {
			t.Errorf("expected path /api/v1/enterprise/cards/lookup, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("card_id") != "247" {
			t.Errorf("expected card_id=247, got %q", r.URL.Query().Get("card_id"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"card": {
				"id": 247,
				"name": "Charizard [1st Edition]",
				"set_name": "Pokemon Base Set",
				"number": "4",
				"rarity": "Holo Rare",
				"language": "en",
				"era": "WOTC",
				"year": "1999",
				"artist": "Mitsuhiro Arita",
				"image_url": "https://example.com/charizard.png",
				"slug": "charizard-1st-edition",
				"pricecharting_id": "pc-123",
				"tcgplayer_product_id": null
			},
			"market_data": {
				"best_bid": 12000.00,
				"best_ask": 15000.00,
				"spread": 3000.00,
				"last_sale": 14000.00,
				"last_sale_date": "2026-04-01",
				"low_price": 11000.00,
				"mid_price": 13500.00,
				"high_price": 16000.00,
				"active_bids": 5,
				"active_asks": 8,
				"24h_volume": 2,
				"24h_change": 3.5,
				"7d_change": -1.2,
				"30d_change": 8.0
			}
		}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.CardLookup(context.Background(), 247)
	if err != nil {
		t.Fatalf("CardLookup() error = %v", err)
	}
	if resp.Card.ID != 247 {
		t.Errorf("Card.ID = %d, want 247", resp.Card.ID)
	}
	if resp.Card.Name != "Charizard [1st Edition]" {
		t.Errorf("Card.Name = %q, want Charizard [1st Edition]", resp.Card.Name)
	}
	if resp.MarketData.MidPrice == nil || *resp.MarketData.MidPrice != 13500.00 {
		t.Errorf("MarketData.MidPrice = %v, want 13500.00", resp.MarketData.MidPrice)
	}
}

func TestClient_NotAvailable(t *testing.T) {
	c := NewClient("")
	_, err := c.CardLookup(context.Background(), 1)
	if err == nil {
		t.Error("expected error when enterprise key not available")
	}
}
