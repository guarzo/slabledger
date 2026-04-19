package dh

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
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

func TestClient_RecentSales(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test_api_key" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/api/v1/enterprise/cards/42/recent-sales" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"sales": [
				{"sold_at": "2026-04-01T12:00:00Z", "grading_company": "PSA", "grade": "10", "price": 500.00, "platform": "eBay"},
				{"sold_at": "2026-03-28T10:00:00Z", "grading_company": "PSA", "grade": "9", "price": 250.00, "platform": "TCGPlayer"}
			]
		}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	sales, err := c.RecentSales(context.Background(), 42)
	if err != nil {
		t.Fatalf("RecentSales() error = %v", err)
	}
	if len(sales) != 2 {
		t.Fatalf("len(sales) = %d, want 2", len(sales))
	}
	if sales[0].Price != 500.00 {
		t.Errorf("sales[0].Price = %v, want 500.00", sales[0].Price)
	}
	if sales[1].GradingCompany != "PSA" {
		t.Errorf("sales[1].GradingCompany = %q, want PSA", sales[1].GradingCompany)
	}
}

func TestClient_MarketDataEnterprise(t *testing.T) {
	t.Run("full response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.URL.Path == "/api/v1/enterprise/cards/lookup":
				_, _ = w.Write([]byte(`{
					"card": {"id": 42, "name": "Charizard"},
					"market_data": {"mid_price": 500.00, "low_price": 400.00, "high_price": 600.00}
				}`))
			case strings.HasSuffix(r.URL.Path, "/recent-sales"):
				_, _ = w.Write([]byte(`{
					"sales": [{"sold_at": "2026-04-01T12:00:00Z", "grading_company": "PSA", "grade": "10", "price": 500.00, "platform": "eBay"}]
				}`))
			default:
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		c := newTestClient(server.URL)
		resp, err := c.MarketDataEnterprise(context.Background(), 42)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if !resp.HasData {
			t.Error("expected HasData = true")
		}
		if resp.CurrentPrice != 500.00 {
			t.Errorf("CurrentPrice = %v, want 500.00", resp.CurrentPrice)
		}
		if resp.PeriodLow != 400.00 {
			t.Errorf("PeriodLow = %v, want 400.00", resp.PeriodLow)
		}
		if resp.PeriodHigh != 600.00 {
			t.Errorf("PeriodHigh = %v, want 600.00", resp.PeriodHigh)
		}
		if len(resp.RecentSales) != 1 {
			t.Fatalf("len(RecentSales) = %d, want 1", len(resp.RecentSales))
		}
	})

	t.Run("recent sales failure returns partial data", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/v1/enterprise/cards/lookup":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"card": {"id": 42, "name": "Charizard"},
					"market_data": {"mid_price": 500.00}
				}`))
			case strings.HasSuffix(r.URL.Path, "/recent-sales"):
				w.WriteHeader(http.StatusInternalServerError)
			default:
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		c := newTestClient(server.URL)
		resp, err := c.MarketDataEnterprise(context.Background(), 42)
		if err != nil {
			t.Fatalf("error = %v (expected partial success)", err)
		}
		if !resp.HasData {
			t.Error("expected HasData = true")
		}
		if resp.CurrentPrice != 500.00 {
			t.Errorf("CurrentPrice = %v, want 500.00", resp.CurrentPrice)
		}
		if len(resp.RecentSales) != 0 {
			t.Errorf("expected empty RecentSales on failure, got %d", len(resp.RecentSales))
		}
	})
}

func TestClient_NotAvailable(t *testing.T) {
	c := NewClient("")
	_, err := c.CardLookup(context.Background(), 1)
	if err == nil {
		t.Error("expected error when enterprise key not available")
	}
}

func TestCardLookup_ValidatesCardID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a response with Card.ID = 0 (invalid)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CardLookupResponse{})
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	_, err := c.CardLookup(context.Background(), 123)
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeProviderInvalidResp {
		t.Errorf("expected ProviderInvalidResponse, got: %v", err)
	}
}

func TestResolveCert_ValidatesStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CertResolution{Status: "unexpected_value"})
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	_, err := c.ResolveCert(context.Background(), CertResolveRequest{CertNumber: "12345678"})
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeProviderInvalidResp {
		t.Errorf("expected ProviderInvalidResponse, got: %v", err)
	}
}

func TestResolveCert_AcceptsValidStatuses(t *testing.T) {
	for _, status := range []string{"matched", "ambiguous", "not_found", "pending"} {
		t.Run(status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(CertResolution{Status: status})
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			resp, err := c.ResolveCert(context.Background(), CertResolveRequest{CertNumber: "12345678"})
			if err != nil {
				t.Errorf("expected no error for status %q, got: %v", status, err)
			}
			if resp.Status != status {
				t.Errorf("expected status %q, got %q", status, resp.Status)
			}
		})
	}
}

func TestRecentSales_ValidatesSaleFields(t *testing.T) {
	t.Run("zero price", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sales": [{"sold_at": "2026-04-01T12:00:00Z", "price": 0}]}`))
		}))
		defer server.Close()

		c := newTestClient(server.URL)
		_, err := c.RecentSales(context.Background(), 42)
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeProviderInvalidResp {
			t.Errorf("expected ProviderInvalidResponse for zero price, got: %v", err)
		}
	})

	t.Run("empty date", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sales": [{"sold_at": "", "price": 100.00}]}`))
		}))
		defer server.Close()

		c := newTestClient(server.URL)
		_, err := c.RecentSales(context.Background(), 42)
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeProviderInvalidResp {
			t.Errorf("expected ProviderInvalidResponse for empty date, got: %v", err)
		}
	})

	t.Run("empty sales list is valid", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sales": []}`))
		}))
		defer server.Close()

		c := newTestClient(server.URL)
		sales, err := c.RecentSales(context.Background(), 42)
		if err != nil {
			t.Errorf("expected no error for empty sales, got: %v", err)
		}
		if len(sales) != 0 {
			t.Errorf("expected 0 sales, got %d", len(sales))
		}
	})
}

func TestSearchCards_ValidatesResultNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SearchResponse{
			Results: []SearchResult{
				{ID: 1, Name: "Valid Card"},
				{ID: 2, Name: ""},
			},
		})
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	_, err := c.SearchCards(context.Background(), SearchFilters{Query: "test"})
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeProviderInvalidResp {
		t.Errorf("expected ProviderInvalidResponse for empty name, got: %v", err)
	}
}

func TestUpdateInventoryWithRotation(t *testing.T) {
	authErr := apperrors.ProviderAuthFailed(providerName, errors.New("HTTP 401: unauthorized"))
	rateLimitErr := errors.New("PSA API rate limit exceeded")
	successResult := &InventoryResult{DHInventoryID: 1, Status: "listed"}

	tests := []struct {
		name          string
		updateErrs    []error // sequence of errors returned from successive update calls
		updateResult  *InventoryResult
		rotateReturns []bool // sequence of RotatePSAKey return values
		wantErr       error  // sentinel to check via errors.Is (nil = expect success)
		wantCalls     int    // how many times updateFn should have been invoked
	}{
		{
			name:         "success on first try — no rotation",
			updateErrs:   []error{nil},
			updateResult: successResult,
			wantErr:      nil,
			wantCalls:    1,
		},
		{
			name:          "401 then success after one rotation",
			updateErrs:    []error{authErr, nil},
			updateResult:  successResult,
			rotateReturns: []bool{true},
			wantErr:       nil,
			wantCalls:     2,
		},
		{
			name:          "422 rate limit then success after one rotation",
			updateErrs:    []error{rateLimitErr, nil},
			updateResult:  successResult,
			rotateReturns: []bool{true},
			wantErr:       nil,
			wantCalls:     2,
		},
		{
			name:          "401 through all keys — exhausted",
			updateErrs:    []error{authErr, authErr, authErr},
			rotateReturns: []bool{true, true, false},
			wantErr:       ErrPSAKeysExhausted,
			wantCalls:     3,
		},
		{
			name:       "non-PSA error does NOT rotate and returns raw error",
			updateErrs: []error{errors.New("HTTP 500: internal server error")},
			wantErr:    nil, // expect passthrough non-sentinel error
			wantCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			doUpdate := func(_ context.Context, _ int, _ InventoryUpdate) (*InventoryResult, error) {
				idx := callCount
				callCount++
				if idx < len(tt.updateErrs) {
					return tt.updateResult, tt.updateErrs[idx]
				}
				return tt.updateResult, nil
			}
			rotateIdx := 0
			rotateFn := func() bool {
				if rotateIdx < len(tt.rotateReturns) {
					r := tt.rotateReturns[rotateIdx]
					rotateIdx++
					return r
				}
				return false
			}

			_, err := UpdateInventoryWithRotation(
				context.Background(),
				1,
				InventoryUpdate{Status: "listed"},
				doUpdate,
				rotateFn,
				nopLogger{},
				"test",
			)

			require.Equal(t, tt.wantCalls, callCount)
			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.name == "non-PSA error does NOT rotate and returns raw error":
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrPSAKeysExhausted)
			default:
				require.NoError(t, err)
			}
		})
	}
}

// nopLogger is a no-op observability.Logger for tests.
type nopLogger struct{}

func (nopLogger) Debug(context.Context, string, ...observability.Field) {}
func (nopLogger) Info(context.Context, string, ...observability.Field)  {}
func (nopLogger) Warn(context.Context, string, ...observability.Field)  {}
func (nopLogger) Error(context.Context, string, ...observability.Field) {}
func (nopLogger) With(context.Context, ...observability.Field) observability.Logger {
	return nopLogger{}
}
