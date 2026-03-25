package cardhedger

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/platform/resilience"
	"golang.org/x/time/rate"
)

func newTestClient(serverURL string) *Client {
	config := httpx.DefaultConfig("CardHedger")
	config.DefaultTimeout = 5 * time.Second
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     0,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	httpClient := httpx.NewClient(config)

	c := &Client{
		apiKey:      "test_api_key",
		baseURL:     serverURL,
		httpClient:  httpClient,
		rateLimiter: rate.NewLimiter(rate.Inf, 1),
	}
	c.resetDailyCounterIfNeeded()
	return c
}

func TestNewClient(t *testing.T) {
	t.Run("with key", func(t *testing.T) {
		c := NewClient("test_key")
		if !c.Available() {
			t.Error("expected Available() = true")
		}
		if c.Name() != "cardhedger" {
			t.Errorf("Name() = %q, want cardhedger", c.Name())
		}
	})

	t.Run("without key", func(t *testing.T) {
		c := NewClient("")
		if c.Available() {
			t.Error("expected Available() = false")
		}
	})
}

func TestClient_SearchCard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test_api_key" {
			t.Errorf("expected X-API-Key header")
		}
		if r.URL.Path != "/cards/card-search" {
			t.Errorf("expected path /cards/card-search, got %s", r.URL.Path)
		}

		var req CardSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Search != "Charizard" {
			t.Errorf("expected search=Charizard, got %q", req.Search)
		}
		if req.Set != "Base Set" {
			t.Errorf("expected set=Base Set, got %q", req.Set)
		}
		if req.Category != "Pokemon" {
			t.Errorf("expected category=Pokemon, got %q", req.Category)
		}

		resp := CardSearchResponse{
			Pages: 1,
			Count: 1,
			Cards: []CardSearchCard{
				{
					CardID:      "1646615786118x244697357144328930",
					Description: "Charizard Base Set Pokemon",
					Player:      "Charizard",
					Set:         "Base Set",
					Number:      "4",
					Category:    "Pokemon",
					Sales7Day:   45,
					Sales30Day:  187,
					Prices: []CardPrice{
						{Grade: "PSA 10", Price: "14875"},
						{Grade: "PSA 9", Price: "2500"},
						{Grade: "Raw", Price: "525.82"},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	resp, statusCode, _, err := c.SearchCard(ctx, "Charizard", "Base Set", "Pokemon")
	if err != nil {
		t.Fatalf("SearchCard() error = %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", statusCode, http.StatusOK)
	}
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
	if len(resp.Cards) != 1 {
		t.Fatalf("len(Cards) = %d, want 1", len(resp.Cards))
	}
	card := resp.Cards[0]
	if card.CardID != "1646615786118x244697357144328930" {
		t.Errorf("CardID = %q, want 1646615786118x244697357144328930", card.CardID)
	}
	if card.Player != "Charizard" {
		t.Errorf("Player = %q, want Charizard", card.Player)
	}
	if len(card.Prices) != 3 {
		t.Errorf("len(Prices) = %d, want 3", len(card.Prices))
	}
}

func TestClient_GetAllPrices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/cards/all-prices-by-card" {
			t.Errorf("expected path /cards/all-prices-by-card, got %s", r.URL.Path)
		}

		var req AllPricesByCardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.CardID != "test_card_123" {
			t.Errorf("expected card_id=test_card_123, got %q", req.CardID)
		}

		resp := AllPricesByCardResponse{
			Prices: []GradePrice{
				{CardID: "test_card_123", Grade: "PSA 10", Grader: "PSA", Price: "14875.00"},
				{CardID: "test_card_123", Grade: "PSA 9", Grader: "PSA", Price: "2500.00"},
				{CardID: "test_card_123", Grade: "Raw", Grader: "Raw", Price: "525.82"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	resp, statusCode, _, err := c.GetAllPrices(ctx, "test_card_123")
	if err != nil {
		t.Fatalf("GetAllPrices() error = %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", statusCode, http.StatusOK)
	}
	if len(resp.Prices) != 3 {
		t.Fatalf("len(Prices) = %d, want 3", len(resp.Prices))
	}
	if resp.Prices[0].Grade != "PSA 10" {
		t.Errorf("Prices[0].Grade = %q, want PSA 10", resp.Prices[0].Grade)
	}
	if resp.Prices[0].Price != "14875.00" {
		t.Errorf("Prices[0].Price = %q, want 14875.00", resp.Prices[0].Price)
	}
	if resp.Prices[2].Grader != "Raw" {
		t.Errorf("Prices[2].Grader = %q, want Raw", resp.Prices[2].Grader)
	}
}

func TestClient_NotAvailable(t *testing.T) {
	c := NewClient("")
	_, _, _, err := c.SearchCard(context.Background(), "test", "set", "Pokemon")
	if err == nil {
		t.Error("expected error when not available")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.AppError, got %T", err)
	}
	if appErr.Code != apperrors.ErrCodeConfigMissing {
		t.Errorf("code = %v, want %v", appErr.Code, apperrors.ErrCodeConfigMissing)
	}
}

func TestClient_DailyCallsCounter(t *testing.T) {
	c := NewClient("test_key")
	if c.DailyCallsUsed() != 0 {
		t.Errorf("initial daily calls = %d, want 0", c.DailyCallsUsed())
	}

	c.incrementDailyCounter()
	c.incrementDailyCounter()
	c.incrementDailyCounter()
	if c.DailyCallsUsed() != 3 {
		t.Errorf("after 3 increments, daily calls = %d, want 3", c.DailyCallsUsed())
	}
}

func TestClient_MinuteCallsCounter(t *testing.T) {
	c := NewClient("test_key")
	if c.MinuteCallsUsed() != 0 {
		t.Errorf("initial minute calls = %d, want 0", c.MinuteCallsUsed())
	}

	c.incrementMinuteCounter()
	c.incrementMinuteCounter()
	if c.MinuteCallsUsed() != 2 {
		t.Errorf("after 2 increments, minute calls = %d, want 2", c.MinuteCallsUsed())
	}
}

func TestClient_429Tracking(t *testing.T) {
	c := NewClient("test_key")
	if c.RateLimitHits() != 0 {
		t.Errorf("initial 429 hits = %d, want 0", c.RateLimitHits())
	}
	if !c.Last429Time().IsZero() {
		t.Error("expected zero Last429Time initially")
	}

	c.record429(context.Background(), "/test")
	if c.RateLimitHits() != 1 {
		t.Errorf("after record429, hits = %d, want 1", c.RateLimitHits())
	}
	if c.Last429Time().IsZero() {
		t.Error("expected non-zero Last429Time after record429")
	}
}

func TestClient_CardMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cards/card-match" {
			t.Errorf("expected path /cards/card-match, got %s", r.URL.Path)
		}

		var req CardMatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Query != "Charizard Base Set 4" {
			t.Errorf("expected query='Charizard Base Set 4', got %q", req.Query)
		}
		if req.Category != "Pokemon" {
			t.Errorf("expected category='Pokemon', got %q", req.Category)
		}
		if req.MaxCandidates != 10 {
			t.Errorf("expected max_candidates=10, got %d", req.MaxCandidates)
		}

		resp := CardMatchResponse{
			Match: &CardMatchResult{
				CardID:     "card_123",
				Set:        "Base Set",
				Number:     "4",
				Player:     "Charizard",
				Confidence: 0.95,
				Reasoning:  "Exact match",
			},
			CandidatesEvaluated: 5,
			SearchQueryUsed:     "Charizard Base Set 4",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, statusCode, _, err := c.CardMatch(context.Background(), "Charizard Base Set 4", "Pokemon", 10)
	if err != nil {
		t.Fatalf("CardMatch() error = %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", statusCode, http.StatusOK)
	}
	if resp.Match == nil {
		t.Fatal("expected non-nil Match")
	}
	if resp.Match.CardID != "card_123" {
		t.Errorf("CardID = %q, want card_123", resp.Match.CardID)
	}
	if resp.Match.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", resp.Match.Confidence)
	}
}

func TestClient_DetailsByCerts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cards/details-by-certs" {
			t.Errorf("expected path /cards/details-by-certs, got %s", r.URL.Path)
		}

		var req DetailsByCertsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(req.Certs) != 2 {
			t.Errorf("expected 2 certs, got %d", len(req.Certs))
		}
		if req.Grader != "PSA" {
			t.Errorf("expected grader='PSA', got %q", req.Grader)
		}

		resp := DetailsByCertsResponse{
			Results: []CertDetailResult{
				{
					CertInfo: CertInfo{Cert: "12345", Grade: "PSA 10", Description: "Charizard Base Set"},
					Card:     &CardDetail{CardID: "card_a", Player: "Charizard", Set: "Base Set", Number: "4"},
				},
				{
					CertInfo: CertInfo{Cert: "67890", Grade: "PSA 9", Description: "Pikachu Base Set"},
					Card:     &CardDetail{CardID: "card_b", Player: "Pikachu", Set: "Base Set", Number: "58"},
				},
			},
			TotalRequested: 2,
			TotalFound:     2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, statusCode, _, err := c.DetailsByCerts(context.Background(), []string{"12345", "67890"}, "PSA")
	if err != nil {
		t.Fatalf("DetailsByCerts() error = %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", statusCode, http.StatusOK)
	}
	if resp.TotalFound != 2 {
		t.Errorf("TotalFound = %d, want 2", resp.TotalFound)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(resp.Results))
	}
	if resp.Results[0].Card == nil {
		t.Fatal("Results[0].Card is nil")
	}
	if resp.Results[0].Card.CardID != "card_a" {
		t.Errorf("Results[0].Card.CardID = %q, want card_a", resp.Results[0].Card.CardID)
	}
	if resp.Results[0].CertInfo.Cert != "12345" {
		t.Errorf("Results[0].CertInfo.Cert = %q, want 12345", resp.Results[0].CertInfo.Cert)
	}
}

func TestClient_DetailsByCerts_TooMany(t *testing.T) {
	c := NewClient("test_key")
	certs := make([]string, 101)
	for i := range certs {
		certs[i] = "cert"
	}
	_, _, _, err := c.DetailsByCerts(context.Background(), certs, "PSA")
	if err == nil {
		t.Error("expected error for >100 certs")
	}
}

func TestClient_429ResponseTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	_, statusCode, _, err := c.SearchCard(context.Background(), "test", "set", "Pokemon")
	if err == nil {
		t.Error("expected error on 429")
	}
	if statusCode != 429 {
		t.Errorf("statusCode = %d, want 429", statusCode)
	}
	if c.RateLimitHits() != 1 {
		t.Errorf("RateLimitHits() = %d, want 1", c.RateLimitHits())
	}
	// Error responses should still be counted as API calls
	if c.DailyCallsUsed() != 1 {
		t.Errorf("DailyCallsUsed() = %d, want 1 (429 should still count)", c.DailyCallsUsed())
	}
}

func TestClient_Close(t *testing.T) {
	c := NewClient("test_key")
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestTypes_Serialization(t *testing.T) {
	t.Run("PriceEstimateResponse", func(t *testing.T) {
		data := `{"price":385.0,"price_low":362.08,"price_high":407.92,"confidence":0.3809,"method":"direct","freshness_days":2,"support_grades":1,"grade_label":"PSA 9","provider":"PSA","grade_value":9.0}`
		var resp PriceEstimateResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Price != 385.0 {
			t.Errorf("price = %v, want 385.0", resp.Price)
		}
		if resp.Confidence != 0.3809 {
			t.Errorf("confidence = %v, want 0.3809", resp.Confidence)
		}
		if resp.Method != "direct" {
			t.Errorf("method = %q, want direct", resp.Method)
		}
		if resp.GradeLabel != "PSA 9" {
			t.Errorf("grade_label = %q, want PSA 9", resp.GradeLabel)
		}
	})

	t.Run("BatchPriceEstimateResult with nulls", func(t *testing.T) {
		data := `{"card_id":"abc","grade":"PSA 10","price":null,"error":"no data available"}`
		var result BatchPriceEstimateResult
		if err := json.Unmarshal([]byte(data), &result); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if result.CardID != "abc" {
			t.Errorf("card_id = %q, want abc", result.CardID)
		}
		if result.Price != nil {
			t.Errorf("price = %v, want nil", result.Price)
		}
		if result.Error == nil || *result.Error != "no data available" {
			t.Errorf("error = %v, want 'no data available'", result.Error)
		}
	})

	t.Run("GradePrice string price", func(t *testing.T) {
		data := `{"card_id":"123","grade":"PSA 10","grader":"PSA","price":"16999.99","display_order":"1"}`
		var gp GradePrice
		if err := json.Unmarshal([]byte(data), &gp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if gp.Price != "16999.99" {
			t.Errorf("price = %q, want 16999.99", gp.Price)
		}
	})

	t.Run("PriceUpdate", func(t *testing.T) {
		data := `{"price":"53.88","sale_date":"2023-07-15","grade":"PSA 9","card_desc":"Test Card","card_set":"Test Set","card_number":"1","player":"Test","variant":"Base","card_id":"abc","update_timestamp":"2023-07-16T01:29:01.227Z"}`
		var pu PriceUpdate
		if err := json.Unmarshal([]byte(data), &pu); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if pu.Price != "53.88" {
			t.Errorf("price = %q, want 53.88", pu.Price)
		}
		if pu.Grade != "PSA 9" {
			t.Errorf("grade = %q, want PSA 9", pu.Grade)
		}
	})

	t.Run("CardMatchResponse", func(t *testing.T) {
		data := `{"match":{"card_id":"abc","confidence":0.85,"reasoning":"Good match"},"candidates_evaluated":10,"search_query_used":"Charizard"}`
		var resp CardMatchResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Match == nil {
			t.Fatal("expected non-nil Match")
		}
		if resp.Match.Confidence != 0.85 {
			t.Errorf("confidence = %v, want 0.85", resp.Match.Confidence)
		}
		if resp.CandidatesEvaluated != 10 {
			t.Errorf("candidates_evaluated = %d, want 10", resp.CandidatesEvaluated)
		}
	})

	t.Run("DetailsByCertsResponse nested format", func(t *testing.T) {
		data := `{"results":[{"cert_info":{"cert":"12345","grade":"PSA 10","description":"Test Card"},"card":{"card_id":"abc","player":"Test","set":"Base Set","number":"1"}}],"total_requested":1,"total_found":1}`
		var resp DetailsByCertsResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.TotalFound != 1 {
			t.Errorf("total_found = %d, want 1", resp.TotalFound)
		}
		if len(resp.Results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(resp.Results))
		}
		if resp.Results[0].CertInfo.Cert != "12345" {
			t.Errorf("cert = %q, want 12345", resp.Results[0].CertInfo.Cert)
		}
		if resp.Results[0].Card == nil {
			t.Fatal("expected non-nil Card")
		}
		if resp.Results[0].Card.CardID != "abc" {
			t.Errorf("card_id = %q, want abc", resp.Results[0].Card.CardID)
		}
	})

	t.Run("DetailsByCertsResponse null card", func(t *testing.T) {
		data := `{"results":[{"cert_info":{"cert":"99999","grade":"PSA 8","description":"Unknown Card"},"card":null}],"total_requested":1,"total_found":0}`
		var resp DetailsByCertsResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(resp.Results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(resp.Results))
		}
		if resp.Results[0].CertInfo.Cert != "99999" {
			t.Errorf("cert = %q, want 99999", resp.Results[0].CertInfo.Cert)
		}
		if resp.Results[0].Card != nil {
			t.Errorf("expected nil Card for unmatched cert, got %+v", resp.Results[0].Card)
		}
	})
}
