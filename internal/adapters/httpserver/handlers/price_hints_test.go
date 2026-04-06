package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// mockPriceHintResolver implements pricing.PriceHintResolver for testing.
type mockPriceHintResolver struct {
	getHintFn    func(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
	saveHintFn   func(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
	deleteHintFn func(ctx context.Context, cardName, setName, collectorNumber, provider string) error
	listHintsFn  func(ctx context.Context) ([]pricing.HintMapping, error)
}

var _ pricing.PriceHintResolver = (*mockPriceHintResolver)(nil)

func (m *mockPriceHintResolver) GetHint(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error) {
	if m.getHintFn != nil {
		return m.getHintFn(ctx, cardName, setName, collectorNumber, provider)
	}
	return "", nil
}

func (m *mockPriceHintResolver) SaveHint(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error {
	if m.saveHintFn != nil {
		return m.saveHintFn(ctx, cardName, setName, collectorNumber, provider, externalID)
	}
	return nil
}

func (m *mockPriceHintResolver) DeleteHint(ctx context.Context, cardName, setName, collectorNumber, provider string) error {
	if m.deleteHintFn != nil {
		return m.deleteHintFn(ctx, cardName, setName, collectorNumber, provider)
	}
	return nil
}

func (m *mockPriceHintResolver) ListHints(ctx context.Context) ([]pricing.HintMapping, error) {
	if m.listHintsFn != nil {
		return m.listHintsFn(ctx)
	}
	return nil, nil
}

func newPriceHintsHandler(resolver *mockPriceHintResolver) *PriceHintsHandler {
	return NewPriceHintsHandler(resolver, mocks.NewMockLogger())
}

// --- GET (handleList) ---

func TestHandlePriceHints_GET_ListSuccess(t *testing.T) {
	resolver := &mockPriceHintResolver{
		listHintsFn: func(_ context.Context) ([]pricing.HintMapping, error) {
			return []pricing.HintMapping{
				{CardName: "Charizard", SetName: "Base Set", CollectorNumber: "4", Provider: "pricecharting", ExternalID: "123"},
			}, nil
		},
	}
	h := newPriceHintsHandler(resolver)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/price-hints", nil)
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result []priceHintResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(result))
	}
	if result[0].CardName != "Charizard" {
		t.Errorf("expected CardName Charizard, got %s", result[0].CardName)
	}
	if result[0].ExternalID != "123" {
		t.Errorf("expected ExternalID 123, got %s", result[0].ExternalID)
	}
}

func TestHandlePriceHints_GET_EmptyList(t *testing.T) {
	resolver := &mockPriceHintResolver{
		listHintsFn: func(_ context.Context) ([]pricing.HintMapping, error) {
			return []pricing.HintMapping{}, nil
		},
	}
	h := newPriceHintsHandler(resolver)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/price-hints", nil)
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("expected [], got %s", body)
	}
}

func TestHandlePriceHints_GET_ResolverError(t *testing.T) {
	resolver := &mockPriceHintResolver{
		listHintsFn: func(_ context.Context) ([]pricing.HintMapping, error) {
			return nil, errors.New("db error")
		},
	}
	h := newPriceHintsHandler(resolver)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/price-hints", nil)
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- POST (handleSave) ---

func TestHandlePriceHints_POST_SaveSuccess(t *testing.T) {
	var savedCard, savedProvider, savedID string
	resolver := &mockPriceHintResolver{
		saveHintFn: func(_ context.Context, cardName, _, _, provider, externalID string) error {
			savedCard = cardName
			savedProvider = provider
			savedID = externalID
			return nil
		},
	}
	h := newPriceHintsHandler(resolver)

	body := `{"cardName":"Charizard","setName":"Base Set","cardNumber":"4","provider":"pricecharting","externalId":"abc"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if savedCard != "Charizard" {
		t.Errorf("expected saved card Charizard, got %s", savedCard)
	}
	if savedProvider != "pricecharting" {
		t.Errorf("expected provider pricecharting, got %s", savedProvider)
	}
	if savedID != "abc" {
		t.Errorf("expected externalId abc, got %s", savedID)
	}
}

func TestHandlePriceHints_POST_BadJSON(t *testing.T) {
	h := newPriceHintsHandler(&mockPriceHintResolver{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/price-hints", strings.NewReader("{bad"))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePriceHints_POST_MissingCardName(t *testing.T) {
	h := newPriceHintsHandler(&mockPriceHintResolver{})

	body := `{"setName":"Base Set","cardNumber":"4","provider":"pricecharting","externalId":"abc"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePriceHints_POST_MissingExternalId(t *testing.T) {
	h := newPriceHintsHandler(&mockPriceHintResolver{})

	body := `{"cardName":"Charizard","setName":"Base Set","cardNumber":"4","provider":"pricecharting"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePriceHints_POST_InvalidProvider(t *testing.T) {
	h := newPriceHintsHandler(&mockPriceHintResolver{})

	body := `{"cardName":"Charizard","setName":"Base Set","cardNumber":"4","provider":"tcgplayer","externalId":"abc"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePriceHints_POST_DoubleHoloProvider(t *testing.T) {
	resolver := &mockPriceHintResolver{
		saveHintFn: func(_ context.Context, _, _, _, _, _ string) error { return nil },
	}
	h := newPriceHintsHandler(resolver)

	body := `{"cardName":"Pikachu","setName":"Base Set","cardNumber":"58","provider":"doubleholo","externalId":"xyz"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for doubleholo provider, got %d", rec.Code)
	}
}

func TestHandlePriceHints_POST_ResolverError(t *testing.T) {
	resolver := &mockPriceHintResolver{
		saveHintFn: func(_ context.Context, _, _, _, _, _ string) error {
			return errors.New("db error")
		},
	}
	h := newPriceHintsHandler(resolver)

	body := `{"cardName":"X","setName":"Y","cardNumber":"1","provider":"pricecharting","externalId":"z"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- DELETE (handleDelete) ---

func TestHandlePriceHints_DELETE_Success(t *testing.T) {
	var deletedCard, deletedProvider string
	resolver := &mockPriceHintResolver{
		deleteHintFn: func(_ context.Context, cardName, _, _, provider string) error {
			deletedCard = cardName
			deletedProvider = provider
			return nil
		},
	}
	h := newPriceHintsHandler(resolver)

	body := `{"cardName":"Charizard","setName":"Base Set","cardNumber":"4","provider":"pricecharting"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if deletedCard != "Charizard" {
		t.Errorf("expected deleted card Charizard, got %s", deletedCard)
	}
	if deletedProvider != "pricecharting" {
		t.Errorf("expected provider pricecharting, got %s", deletedProvider)
	}
}

func TestHandlePriceHints_DELETE_BadJSON(t *testing.T) {
	h := newPriceHintsHandler(&mockPriceHintResolver{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/price-hints", strings.NewReader("{bad"))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePriceHints_DELETE_MissingProvider(t *testing.T) {
	h := newPriceHintsHandler(&mockPriceHintResolver{})

	body := `{"cardName":"Charizard","setName":"Base Set","cardNumber":"4"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePriceHints_DELETE_InvalidProvider(t *testing.T) {
	h := newPriceHintsHandler(&mockPriceHintResolver{})

	body := `{"cardName":"Charizard","setName":"Base Set","cardNumber":"4","provider":"ebay"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePriceHints_DELETE_NoExternalIdOK(t *testing.T) {
	resolver := &mockPriceHintResolver{
		deleteHintFn: func(_ context.Context, _, _, _, _ string) error { return nil },
	}
	h := newPriceHintsHandler(resolver)

	// externalId is NOT required for delete
	body := `{"cardName":"Charizard","setName":"Base Set","cardNumber":"4","provider":"pricecharting"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandlePriceHints_DELETE_ResolverError(t *testing.T) {
	resolver := &mockPriceHintResolver{
		deleteHintFn: func(_ context.Context, _, _, _, _ string) error {
			return errors.New("db error")
		},
	}
	h := newPriceHintsHandler(resolver)

	body := `{"cardName":"X","setName":"Y","cardNumber":"1","provider":"pricecharting"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/price-hints", strings.NewReader(body))
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- Method guard ---

func TestHandlePriceHints_PUT_MethodNotAllowed(t *testing.T) {
	h := newPriceHintsHandler(&mockPriceHintResolver{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/price-hints", nil)
	h.HandlePriceHints(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
