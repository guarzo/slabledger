package justtcg

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

func TestSearchCards(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		setupServer func(t *testing.T) *httptest.Server
		wantErr     bool
		wantCode    apperrors.ErrorCode
		wantCards   int
	}{
		{
			name:   "success",
			apiKey: "test-key",
			setupServer: func(t *testing.T) *httptest.Server {
				t.Helper()
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/v1/cards" {
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
					q := r.URL.Query()
					if got := q.Get("q"); got != "charizard" {
						t.Errorf("q = %q, want charizard", got)
					}
					if got := q.Get("game"); got != "pokemon" {
						t.Errorf("game = %q, want pokemon", got)
					}
					if got := q.Get("set"); got != "base-set" {
						t.Errorf("set = %q, want base-set", got)
					}
					resp := cardsResponse{
						Data: []Card{{CardID: "card-1", Name: "Charizard", SetName: "Base Set"}},
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp) //nolint:errcheck
				}))
			},
			wantCards: 1,
		},
		{
			name:     "no API key",
			apiKey:   "",
			wantErr:  true,
			wantCode: apperrors.ErrCodeConfigMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.apiKey)
			if tt.setupServer != nil {
				server := tt.setupServer(t)
				defer server.Close()
				client.baseURL = server.URL + "/v1"
			}

			cards, err := client.SearchCards(context.Background(), "charizard", "base-set")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				var appErr *apperrors.AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("expected *apperrors.AppError, got %T", err)
				}
				if appErr.Code != tt.wantCode {
					t.Errorf("code = %v, want %v", appErr.Code, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cards) != tt.wantCards {
				t.Fatalf("got %d cards, want %d", len(cards), tt.wantCards)
			}
			if tt.wantCards > 0 && cards[0].CardID != "card-1" {
				t.Errorf("card ID = %q, want card-1", cards[0].CardID)
			}
		})
	}
}

func TestSearchSets_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := setsResponse{
			Data: []Set{{ID: "set-1", Name: "Base Set"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL + "/v1"

	sets, err := client.SearchSets(context.Background(), "base")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("got %d sets, want 1", len(sets))
	}
	if sets[0].Name != "Base Set" {
		t.Errorf("set name = %q, want Base Set", sets[0].Name)
	}
}

func TestBatchLookup(t *testing.T) {
	tests := []struct {
		name        string
		ids         []string
		setupServer func() *httptest.Server
		wantErr     bool
		wantCards   int
		wantNil     bool
	}{
		{
			name: "success",
			ids:  []string{"card-1", "card-2"},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Errorf("method = %s, want POST", r.Method)
					}
					resp := cardsResponse{
						Data: []Card{
							{CardID: "card-1", Name: "Charizard"},
							{CardID: "card-2", Name: "Blastoise"},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp) //nolint:errcheck
				}))
			},
			wantCards: 2,
		},
		{
			name:    "empty input",
			ids:     nil,
			wantNil: true,
		},
		{
			name:    "too many IDs",
			ids:     make([]string, 101),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("test-key")
			if tt.setupServer != nil {
				server := tt.setupServer()
				defer server.Close()
				client.baseURL = server.URL + "/v1"
			}

			cards, err := client.BatchLookup(context.Background(), tt.ids)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if cards != nil {
					t.Errorf("expected nil, got %d cards", len(cards))
				}
				return
			}
			if len(cards) != tt.wantCards {
				t.Fatalf("got %d cards, want %d", len(cards), tt.wantCards)
			}
		})
	}
}

func TestClient_429RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL + "/v1"

	_, err := client.SearchCards(context.Background(), "test", "")
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
	tests := []struct {
		name     string
		card     Card
		printing string
		method   string // "NMPrice" or "BestNMPrice"
		want     float64
	}{
		{
			name: "NMPrice no matching printing",
			card: Card{Variants: []Variant{
				{Condition: "LP", Printing: "Normal", Price: 10.00},
				{Condition: "NM", Printing: "Holo", Price: 25.00},
			}},
			printing: "Normal",
			method:   "NMPrice",
			want:     0,
		},
		{
			name: "BestNMPrice no NM variants",
			card: Card{Variants: []Variant{
				{Condition: "LP", Printing: "Normal", Price: 10.00},
				{Condition: "MP", Printing: "Holo", Price: 5.00},
			}},
			method: "BestNMPrice",
			want:   0,
		},
		{
			name:     "NMPrice empty variants",
			card:     Card{},
			printing: "Normal",
			method:   "NMPrice",
			want:     0,
		},
		{
			name:   "BestNMPrice empty variants",
			card:   Card{},
			method: "BestNMPrice",
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got float64
			if tt.method == "NMPrice" {
				got = tt.card.NMPrice(tt.printing)
			} else {
				got = tt.card.BestNMPrice()
			}
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}
