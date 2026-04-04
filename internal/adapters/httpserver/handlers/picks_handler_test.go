package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/picks"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newTestPicksHandler(svc *mocks.MockPicksService) *PicksHandler {
	return NewPicksHandler(svc, mocks.NewMockLogger())
}

func samplePick() picks.Pick {
	return picks.Pick{
		ID:                1,
		Date:              time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		CardName:          "Charizard",
		SetName:           "Base Set",
		Grade:             "PSA 10",
		Direction:         picks.DirectionBuy,
		Confidence:        picks.ConfidenceHigh,
		BuyThesis:         "Strong demand",
		TargetBuyPrice:    15000,
		ExpectedSellPrice: 22500,
		Signals:           []picks.Signal{{Factor: "population", Direction: picks.SignalBullish, Title: "Low pop", Detail: "Only 50 PSA 10s"}},
		Rank:              1,
		Source:            picks.SourceAI,
		CreatedAt:         time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
	}
}

func TestHandleGetPicks(t *testing.T) {
	tests := []struct {
		name           string
		getLatestFn    func(context.Context) ([]picks.Pick, error)
		expectedStatus int
		expectedCount  int
		expectError    bool
		checkNotNil    bool
	}{
		{
			name: "success returns picks",
			getLatestFn: func(_ context.Context) ([]picks.Pick, error) {
				return []picks.Pick{samplePick()}, nil
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name: "empty returns empty array",
			getLatestFn: func(_ context.Context) ([]picks.Pick, error) {
				return nil, nil
			},
			expectedStatus: http.StatusOK,
			expectedCount:  0,
			checkNotNil:    true,
		},
		{
			name: "service error returns 500",
			getLatestFn: func(_ context.Context) ([]picks.Pick, error) {
				return nil, fmt.Errorf("database error")
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockPicksService{
				GetLatestPicksFn: tt.getLatestFn,
			}
			h := newTestPicksHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/picks", nil)
			rec := httptest.NewRecorder()
			h.HandleGetPicks(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectError {
				decodeErrorResponse(t, rec)
				return
			}

			var result struct {
				Picks []pickResponse `json:"picks"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if tt.checkNotNil && result.Picks == nil {
				t.Error("expected empty array, got nil")
			}
			if len(result.Picks) != tt.expectedCount {
				t.Fatalf("expected %d pick(s), got %d", tt.expectedCount, len(result.Picks))
			}

			if tt.expectedCount > 0 {
				if result.Picks[0].CardName != "Charizard" {
					t.Errorf("expected CardName=Charizard, got %q", result.Picks[0].CardName)
				}
				if result.Picks[0].TargetBuyPrice != 150.00 {
					t.Errorf("expected TargetBuyPrice=150.00, got %v", result.Picks[0].TargetBuyPrice)
				}
			}
		})
	}
}

func TestHandleGetPickHistory(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		expectedDays int
	}{
		{
			name:         "default 7 days",
			query:        "",
			expectedDays: 7,
		},
		{
			name:         "custom 30 days",
			query:        "?days=30",
			expectedDays: 30,
		},
		{
			name:         "max 90 days",
			query:        "?days=90",
			expectedDays: 90,
		},
		{
			name:         "invalid range uses default",
			query:        "?days=200",
			expectedDays: 7,
		},
		{
			name:         "zero uses default",
			query:        "?days=0",
			expectedDays: 7,
		},
		{
			name:         "negative uses default",
			query:        "?days=-5",
			expectedDays: 7,
		},
		{
			name:         "non-numeric uses default",
			query:        "?days=abc",
			expectedDays: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedDays int
			svc := &mocks.MockPicksService{
				GetPickHistoryFn: func(_ context.Context, days int) ([]picks.Pick, error) {
					capturedDays = days
					return []picks.Pick{samplePick()}, nil
				},
			}
			h := newTestPicksHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/picks/history"+tt.query, nil)
			rec := httptest.NewRecorder()
			h.HandleGetPickHistory(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			if capturedDays != tt.expectedDays {
				t.Errorf("expected days=%d, got %d", tt.expectedDays, capturedDays)
			}
		})
	}
}

func TestHandleGetPickHistory_Error(t *testing.T) {
	svc := &mocks.MockPicksService{
		GetPickHistoryFn: func(_ context.Context, _ int) ([]picks.Pick, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/picks/history", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPickHistory(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetWatchlist(t *testing.T) {
	tests := []struct {
		name          string
		mockFn        func(_ context.Context) ([]picks.WatchlistItem, error)
		wantStatus    int
		wantCount     int
		wantFirstCard string
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) ([]picks.WatchlistItem, error) {
				return []picks.WatchlistItem{
					{
						ID:        1,
						CardName:  "Pikachu",
						SetName:   "Base Set",
						Grade:     "PSA 9",
						Source:    picks.WatchlistManual,
						Active:    true,
						AddedAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
						UpdatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					},
				}, nil
			},
			wantStatus:    http.StatusOK,
			wantCount:     1,
			wantFirstCard: "Pikachu",
		},
		{
			name: "empty",
			mockFn: func(_ context.Context) ([]picks.WatchlistItem, error) {
				return nil, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) ([]picks.WatchlistItem, error) {
				return nil, fmt.Errorf("db error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockPicksService{GetWatchlistFn: tt.mockFn}
			h := newTestPicksHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/picks/watchlist", nil)
			rec := httptest.NewRecorder()
			h.HandleGetWatchlist(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			if tt.wantStatus == http.StatusOK {
				var result struct {
					Items []watchlistItemResponse `json:"items"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(result.Items) != tt.wantCount {
					t.Errorf("expected %d item(s), got %d", tt.wantCount, len(result.Items))
				}
				if tt.wantFirstCard != "" && len(result.Items) > 0 && result.Items[0].CardName != tt.wantFirstCard {
					t.Errorf("expected CardName=%s, got %q", tt.wantFirstCard, result.Items[0].CardName)
				}
			}
		})
	}
}

func TestHandleAddWatchlistItem(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		mockFn     func(_ context.Context, _ picks.WatchlistItem) error
		wantStatus int
	}{
		{
			name: "success",
			body: `{"card_name":"Charizard","set_name":"Base Set","grade":"PSA 10"}`,
			mockFn: func(_ context.Context, item picks.WatchlistItem) error {
				if item.CardName != "Charizard" {
					t.Errorf("expected CardName=Charizard, got %q", item.CardName)
				}
				return nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid body",
			body:       "{bad",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing card_name",
			body:       `{"set_name":"Base Set","grade":"PSA 10"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing set_name",
			body:       `{"card_name":"Charizard","grade":"PSA 10"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing grade",
			body:       `{"card_name":"Charizard","set_name":"Base Set"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "all empty",
			body:       `{"card_name":"","set_name":"","grade":""}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate",
			body: `{"card_name":"Charizard","set_name":"Base Set","grade":"PSA 10"}`,
			mockFn: func(_ context.Context, _ picks.WatchlistItem) error {
				return picks.ErrWatchlistDuplicate
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "service error",
			body: `{"card_name":"Charizard","set_name":"Base Set","grade":"PSA 10"}`,
			mockFn: func(_ context.Context, _ picks.WatchlistItem) error {
				return fmt.Errorf("db error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockPicksService{}
			if tt.mockFn != nil {
				svc.AddToWatchlistFn = tt.mockFn
			}
			h := newTestPicksHandler(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/picks/watchlist", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			h.HandleAddWatchlistItem(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleDeleteWatchlistItem(t *testing.T) {
	tests := []struct {
		name       string
		pathID     string
		mockFn     func(_ context.Context, _ int) error
		wantStatus int
	}{
		{
			name:   "success",
			pathID: "42",
			mockFn: func(_ context.Context, id int) error {
				if id != 42 {
					t.Errorf("expected id=42, got %d", id)
				}
				return nil
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "invalid ID",
			pathID:     "abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "not found",
			pathID: "99",
			mockFn: func(_ context.Context, _ int) error {
				return picks.ErrWatchlistItemNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "service error",
			pathID: "1",
			mockFn: func(_ context.Context, _ int) error {
				return fmt.Errorf("db error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockPicksService{}
			if tt.mockFn != nil {
				svc.RemoveFromWatchlistFn = tt.mockFn
			}
			h := newTestPicksHandler(svc)

			req := httptest.NewRequest(http.MethodDelete, "/api/picks/watchlist/"+tt.pathID, nil)
			req.SetPathValue("id", tt.pathID)
			rec := httptest.NewRecorder()
			h.HandleDeleteWatchlistItem(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}
