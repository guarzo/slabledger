package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newCardHandler(opts ...mocks.MockOption) *Handler {
	cardProv := mocks.NewMockCardProvider(opts...)
	searchSvc := cards.NewSearchService(cardProv)
	return NewHandler(cardProv, searchSvc, mocks.NewMockLogger())
}

func TestHandleCardSearch_GET_ValidQuery(t *testing.T) {
	h := newCardHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/search?q=Test", nil)
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp CardSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Cards) == 0 {
		t.Error("expected non-empty cards")
	}
	if resp.Total == 0 {
		t.Error("expected total > 0")
	}

	// Verify response shape — check first card has expected fields
	c := resp.Cards[0]
	if c.ID == "" {
		t.Error("expected non-empty id")
	}
	if c.Name == "" {
		t.Error("expected non-empty name")
	}
}

func TestHandleCardSearch_GET_EmptyQuery(t *testing.T) {
	h := newCardHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/search?q=", nil)
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCardSearch_GET_MissingQuery(t *testing.T) {
	h := newCardHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/search", nil)
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCardSearch_GET_SearchError(t *testing.T) {
	h := newCardHandler(mocks.WithError(errors.New("search failed")))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/search?q=Charizard", nil)
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleCardSearch_GET_EmptyResults(t *testing.T) {
	h := newCardHandler(mocks.WithEmptyData())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/search?q=Nonexistent", nil)
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp CardSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Cards) != 0 {
		t.Errorf("expected 0 cards, got %d", len(resp.Cards))
	}
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
}

func TestHandleCardSearch_POST_ValidJSON(t *testing.T) {
	h := newCardHandler()

	body := `{"query":"Test","limit":5}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cards/search", strings.NewReader(body))
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCardSearch_POST_EmptyQuery(t *testing.T) {
	h := newCardHandler()

	body := `{"query":"","limit":5}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cards/search", strings.NewReader(body))
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCardSearch_POST_BadJSON(t *testing.T) {
	h := newCardHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cards/search", strings.NewReader("{invalid"))
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCardSearch_POST_LimitClamped(t *testing.T) {
	h := newCardHandler()

	body := `{"query":"Test","limit":200}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cards/search", strings.NewReader(body))
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp CardSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// With limit clamped to 10, we should get at most 10 results
	if len(resp.Cards) > 10 {
		t.Errorf("expected at most 10 cards (clamped limit), got %d", len(resp.Cards))
	}
}

func TestHandleCardSearch_PUT_MethodNotAllowed(t *testing.T) {
	h := newCardHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/cards/search", nil)
	h.HandleCardSearch(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
