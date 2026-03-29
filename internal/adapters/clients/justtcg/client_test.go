package justtcg

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

func newTestClient(serverURL string) *Client {
	config := httpx.DefaultConfig("JustTCG")
	config.DefaultTimeout = 5 * time.Second
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	return &Client{
		apiKey:      "test_key",
		baseURL:     serverURL,
		httpClient:  httpx.NewClient(config),
		rateLimiter: rate.NewLimiter(rate.Inf, 1),
	}
}

func TestSearchCards(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test_key" {
			t.Errorf("expected X-API-Key=test_key, got %q", r.Header.Get("X-API-Key"))
		}
		if r.URL.Path != "/cards" {
			t.Errorf("expected path /cards, got %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("q") != "Charizard" {
			t.Errorf("expected q=Charizard, got %q", q.Get("q"))
		}
		if q.Get("game") != "pokemon" {
			t.Errorf("expected game=pokemon, got %q", q.Get("game"))
		}
		if q.Get("set") != "Base Set" {
			t.Errorf("expected set=Base Set, got %q", q.Get("set"))
		}
		if q.Get("limit") != "20" {
			t.Errorf("expected limit=20, got %q", q.Get("limit"))
		}

		resp := cardsResponse{
			Data: []Card{
				{
					CardID:  "card-001",
					Name:    "Charizard",
					SetID:   "base",
					SetName: "Base Set",
					Number:  "4",
					Variants: []Variant{
						{Condition: "NM", Printing: "Normal", Price: 45.00},
						{Condition: "LP", Printing: "Normal", Price: 38.00},
						{Condition: "NM", Printing: "Holo", Price: 75.00},
					},
				},
			},
			Meta: responseMeta{Total: 1, Page: 1, PageSize: 20},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	cards, err := c.SearchCards(context.Background(), "Charizard", "Base Set")
	if err != nil {
		t.Fatalf("SearchCards() error = %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("len(cards) = %d, want 1", len(cards))
	}
	if cards[0].CardID != "card-001" {
		t.Errorf("CardID = %q, want card-001", cards[0].CardID)
	}
	if cards[0].Name != "Charizard" {
		t.Errorf("Name = %q, want Charizard", cards[0].Name)
	}
	if len(cards[0].Variants) != 3 {
		t.Errorf("len(Variants) = %d, want 3", len(cards[0].Variants))
	}
}

func TestSearchSets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/sets" {
			t.Errorf("expected path /sets, got %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("game") != "pokemon" {
			t.Errorf("expected game=pokemon, got %q", q.Get("game"))
		}
		if q.Get("q") != "Base" {
			t.Errorf("expected q=Base, got %q", q.Get("q"))
		}

		resp := setsResponse{
			Data: []Set{
				{ID: "base", Name: "Base Set", CardCount: 102},
				{ID: "base2", Name: "Base Set 2", CardCount: 130},
			},
			Meta: responseMeta{Total: 2, Page: 1, PageSize: 20},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	sets, err := c.SearchSets(context.Background(), "Base")
	if err != nil {
		t.Fatalf("SearchSets() error = %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("len(sets) = %d, want 2", len(sets))
	}
	if sets[0].ID != "base" {
		t.Errorf("sets[0].ID = %q, want base", sets[0].ID)
	}
	if sets[0].CardCount != 102 {
		t.Errorf("sets[0].CardCount = %d, want 102", sets[0].CardCount)
	}
}

func TestBatchLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/cards" {
			t.Errorf("expected path /cards, got %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test_key" {
			t.Errorf("expected X-API-Key=test_key, got %q", r.Header.Get("X-API-Key"))
		}

		var body []batchLookupItem
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if len(body) != 2 {
			t.Errorf("expected 2 items in body, got %d", len(body))
		}
		if body[0].CardID != "card-001" {
			t.Errorf("body[0].CardID = %q, want card-001", body[0].CardID)
		}
		if body[1].CardID != "card-002" {
			t.Errorf("body[1].CardID = %q, want card-002", body[1].CardID)
		}

		resp := cardsResponse{
			Data: []Card{
				{CardID: "card-001", Name: "Charizard", Variants: []Variant{
					{Condition: "NM", Printing: "Normal", Price: 45.00},
				}},
				{CardID: "card-002", Name: "Pikachu", Variants: []Variant{
					{Condition: "NM", Printing: "Normal", Price: 12.50},
				}},
			},
			Meta: responseMeta{Total: 2},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	cards, err := c.BatchLookup(context.Background(), []string{"card-001", "card-002"})
	if err != nil {
		t.Fatalf("BatchLookup() error = %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("len(cards) = %d, want 2", len(cards))
	}
	if cards[0].CardID != "card-001" {
		t.Errorf("cards[0].CardID = %q, want card-001", cards[0].CardID)
	}
	if cards[1].Name != "Pikachu" {
		t.Errorf("cards[1].Name = %q, want Pikachu", cards[1].Name)
	}
}

func TestBatchLookup_Empty(t *testing.T) {
	c := newTestClient("http://unused")
	cards, err := c.BatchLookup(context.Background(), nil)
	if err != nil {
		t.Fatalf("BatchLookup(nil) error = %v", err)
	}
	if cards != nil {
		t.Errorf("expected nil for empty input, got %v", cards)
	}
}

func TestClient_NotAvailable(t *testing.T) {
	c := NewClient("")
	_, err := c.SearchCards(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error when not available")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.AppError, got %T", err)
	}
	if appErr.Code != apperrors.ErrCodeConfigMissing {
		t.Errorf("code = %v, want %v", appErr.Code, apperrors.ErrCodeConfigMissing)
	}
}

func TestClient_429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	_, err := c.SearchCards(context.Background(), "Charizard", "")
	if err == nil {
		t.Fatal("expected error on 429")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.AppError, got %T", err)
	}
	if appErr.Code != apperrors.ErrCodeProviderRateLimit {
		t.Errorf("code = %v, want %v", appErr.Code, apperrors.ErrCodeProviderRateLimit)
	}
}

func TestNMPrice_NotFound(t *testing.T) {
	t.Run("NMPrice no matching printing", func(t *testing.T) {
		card := Card{
			Variants: []Variant{
				{Condition: "LP", Printing: "Normal", Price: 10.00},
				{Condition: "NM", Printing: "Holo", Price: 25.00},
			},
		}
		price := card.NMPrice("Normal")
		if price != 0 {
			t.Errorf("NMPrice(Normal) = %v, want 0 (no NM Normal variant)", price)
		}
	})

	t.Run("BestNMPrice no NM variants", func(t *testing.T) {
		card := Card{
			Variants: []Variant{
				{Condition: "LP", Printing: "Normal", Price: 10.00},
				{Condition: "MP", Printing: "Holo", Price: 5.00},
			},
		}
		price := card.BestNMPrice()
		if price != 0 {
			t.Errorf("BestNMPrice() = %v, want 0 (no NM variants)", price)
		}
	})

	t.Run("NMPrice empty variants", func(t *testing.T) {
		card := Card{}
		if p := card.NMPrice("Normal"); p != 0 {
			t.Errorf("NMPrice on empty card = %v, want 0", p)
		}
		if p := card.BestNMPrice(); p != 0 {
			t.Errorf("BestNMPrice on empty card = %v, want 0", p)
		}
	})
}
