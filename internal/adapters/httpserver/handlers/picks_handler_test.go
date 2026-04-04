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

func TestHandleGetPicks_Success(t *testing.T) {
	svc := &mocks.MockPicksService{
		GetLatestPicksFn: func(_ context.Context) ([]picks.Pick, error) {
			return []picks.Pick{samplePick()}, nil
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/picks", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPicks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result struct {
		Picks []pickResponse `json:"picks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Picks) != 1 {
		t.Fatalf("expected 1 pick, got %d", len(result.Picks))
	}
	if result.Picks[0].CardName != "Charizard" {
		t.Errorf("expected CardName=Charizard, got %q", result.Picks[0].CardName)
	}
	if result.Picks[0].TargetBuyPrice != 150.00 {
		t.Errorf("expected TargetBuyPrice=150.00, got %v", result.Picks[0].TargetBuyPrice)
	}
}

func TestHandleGetPicks_Empty(t *testing.T) {
	svc := &mocks.MockPicksService{
		GetLatestPicksFn: func(_ context.Context) ([]picks.Pick, error) {
			return nil, nil
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/picks", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPicks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result struct {
		Picks []pickResponse `json:"picks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Picks == nil {
		t.Error("expected empty array, got nil")
	}
	if len(result.Picks) != 0 {
		t.Errorf("expected 0 picks, got %d", len(result.Picks))
	}
}

func TestHandleGetPicks_Error(t *testing.T) {
	svc := &mocks.MockPicksService{
		GetLatestPicksFn: func(_ context.Context) ([]picks.Pick, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/picks", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPicks(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
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
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleGetWatchlist_Success(t *testing.T) {
	svc := &mocks.MockPicksService{
		GetWatchlistFn: func(_ context.Context) ([]picks.WatchlistItem, error) {
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
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/picks/watchlist", nil)
	rec := httptest.NewRecorder()
	h.HandleGetWatchlist(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result struct {
		Items []watchlistItemResponse `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].CardName != "Pikachu" {
		t.Errorf("expected CardName=Pikachu, got %q", result.Items[0].CardName)
	}
}

func TestHandleGetWatchlist_Empty(t *testing.T) {
	svc := &mocks.MockPicksService{
		GetWatchlistFn: func(_ context.Context) ([]picks.WatchlistItem, error) {
			return nil, nil
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/picks/watchlist", nil)
	rec := httptest.NewRecorder()
	h.HandleGetWatchlist(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result struct {
		Items []watchlistItemResponse `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
}

func TestHandleGetWatchlist_Error(t *testing.T) {
	svc := &mocks.MockPicksService{
		GetWatchlistFn: func(_ context.Context) ([]picks.WatchlistItem, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/picks/watchlist", nil)
	rec := httptest.NewRecorder()
	h.HandleGetWatchlist(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleAddWatchlistItem_Success(t *testing.T) {
	svc := &mocks.MockPicksService{
		AddToWatchlistFn: func(_ context.Context, item picks.WatchlistItem) error {
			if item.CardName != "Charizard" {
				t.Errorf("expected CardName=Charizard, got %q", item.CardName)
			}
			return nil
		},
	}
	h := newTestPicksHandler(svc)

	body := `{"card_name":"Charizard","set_name":"Base Set","grade":"PSA 10"}`
	req := httptest.NewRequest(http.MethodPost, "/api/picks/watchlist", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleAddWatchlistItem(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddWatchlistItem_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing card_name", `{"set_name":"Base Set","grade":"PSA 10"}`},
		{"missing set_name", `{"card_name":"Charizard","grade":"PSA 10"}`},
		{"missing grade", `{"card_name":"Charizard","set_name":"Base Set"}`},
		{"all empty", `{"card_name":"","set_name":"","grade":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestPicksHandler(&mocks.MockPicksService{})

			req := httptest.NewRequest(http.MethodPost, "/api/picks/watchlist", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			h.HandleAddWatchlistItem(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleAddWatchlistItem_Duplicate(t *testing.T) {
	svc := &mocks.MockPicksService{
		AddToWatchlistFn: func(_ context.Context, _ picks.WatchlistItem) error {
			return picks.ErrWatchlistDuplicate
		},
	}
	h := newTestPicksHandler(svc)

	body := `{"card_name":"Charizard","set_name":"Base Set","grade":"PSA 10"}`
	req := httptest.NewRequest(http.MethodPost, "/api/picks/watchlist", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleAddWatchlistItem(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestHandleAddWatchlistItem_InvalidBody(t *testing.T) {
	h := newTestPicksHandler(&mocks.MockPicksService{})

	req := httptest.NewRequest(http.MethodPost, "/api/picks/watchlist", bytes.NewBufferString("{bad"))
	rec := httptest.NewRecorder()
	h.HandleAddWatchlistItem(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAddWatchlistItem_ServiceError(t *testing.T) {
	svc := &mocks.MockPicksService{
		AddToWatchlistFn: func(_ context.Context, _ picks.WatchlistItem) error {
			return fmt.Errorf("db error")
		},
	}
	h := newTestPicksHandler(svc)

	body := `{"card_name":"Charizard","set_name":"Base Set","grade":"PSA 10"}`
	req := httptest.NewRequest(http.MethodPost, "/api/picks/watchlist", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleAddWatchlistItem(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleDeleteWatchlistItem_Success(t *testing.T) {
	svc := &mocks.MockPicksService{
		RemoveFromWatchlistFn: func(_ context.Context, id int) error {
			if id != 42 {
				t.Errorf("expected id=42, got %d", id)
			}
			return nil
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/picks/watchlist/42", nil)
	req.SetPathValue("id", "42")
	rec := httptest.NewRecorder()
	h.HandleDeleteWatchlistItem(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteWatchlistItem_InvalidID(t *testing.T) {
	h := newTestPicksHandler(&mocks.MockPicksService{})

	req := httptest.NewRequest(http.MethodDelete, "/api/picks/watchlist/abc", nil)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	h.HandleDeleteWatchlistItem(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleDeleteWatchlistItem_NotFound(t *testing.T) {
	svc := &mocks.MockPicksService{
		RemoveFromWatchlistFn: func(_ context.Context, _ int) error {
			return picks.ErrWatchlistItemNotFound
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/picks/watchlist/99", nil)
	req.SetPathValue("id", "99")
	rec := httptest.NewRecorder()
	h.HandleDeleteWatchlistItem(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleDeleteWatchlistItem_ServiceError(t *testing.T) {
	svc := &mocks.MockPicksService{
		RemoveFromWatchlistFn: func(_ context.Context, _ int) error {
			return fmt.Errorf("db error")
		},
	}
	h := newTestPicksHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/picks/watchlist/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	h.HandleDeleteWatchlistItem(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
