package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// mockCertPriceLookup implements CertPriceLookup for tests.
type mockCertPriceLookup struct {
	purchases map[string]*campaigns.Purchase
	err       error
}

func (m *mockCertPriceLookup) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*campaigns.Purchase, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make(map[string]*campaigns.Purchase)
	for _, cn := range certNumbers {
		if p, ok := m.purchases[cn]; ok {
			result[cn] = p
		}
	}
	return result, nil
}

func TestHandleSinglePrice_Found(t *testing.T) {
	mock := &mockCertPriceLookup{
		purchases: map[string]*campaigns.Purchase{
			"12345678": {CertNumber: "12345678", CLValueCents: 9473},
		},
	}
	h := NewPricingAPIHandler(mock, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prices/12345678", nil)
	req.SetPathValue("certNumber", "12345678")
	rec := httptest.NewRecorder()

	h.HandleSinglePrice(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["certNumber"] != "12345678" {
		t.Errorf("expected certNumber '12345678', got %v", resp["certNumber"])
	}
	if resp["suggestedPrice"] != 94.73 {
		t.Errorf("expected suggestedPrice 94.73, got %v", resp["suggestedPrice"])
	}
	if resp["currency"] != "USD" {
		t.Errorf("expected currency 'USD', got %v", resp["currency"])
	}
}

func TestHandleSinglePrice_NotFound(t *testing.T) {
	mock := &mockCertPriceLookup{purchases: map[string]*campaigns.Purchase{}}
	h := NewPricingAPIHandler(mock, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prices/99999999", nil)
	req.SetPathValue("certNumber", "99999999")
	rec := httptest.NewRecorder()

	h.HandleSinglePrice(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "no_data" {
		t.Errorf("expected error 'no_data', got %q", resp["error"])
	}
}

func TestHandleSinglePrice_ZeroCLValue(t *testing.T) {
	mock := &mockCertPriceLookup{
		purchases: map[string]*campaigns.Purchase{
			"12345678": {CertNumber: "12345678", CLValueCents: 0},
		},
	}
	h := NewPricingAPIHandler(mock, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prices/12345678", nil)
	req.SetPathValue("certNumber", "12345678")
	rec := httptest.NewRecorder()

	h.HandleSinglePrice(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for zero CLValue, got %d", rec.Code)
	}
}

func TestHandleSinglePrice_DBError(t *testing.T) {
	mock := &mockCertPriceLookup{err: fmt.Errorf("database connection lost")}
	h := NewPricingAPIHandler(mock, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prices/12345678", nil)
	req.SetPathValue("certNumber", "12345678")
	rec := httptest.NewRecorder()

	h.HandleSinglePrice(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "internal_error" {
		t.Errorf("expected 'internal_error', got %q", resp["error"])
	}
}

func TestHandleBatchPrices_HappyPath(t *testing.T) {
	mock := &mockCertPriceLookup{
		purchases: map[string]*campaigns.Purchase{
			"11111111": {CertNumber: "11111111", CLValueCents: 5000},
			"22222222": {CertNumber: "22222222", CLValueCents: 10050},
		},
	}
	h := NewPricingAPIHandler(mock, nil)

	body := `{"certNumbers": ["11111111", "22222222", "33333333"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prices/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleBatchPrices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results        []priceResult `json:"results"`
		NotFound       []string      `json:"notFound"`
		TotalRequested int           `json:"totalRequested"`
		TotalFound     int           `json:"totalFound"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.TotalRequested != 3 {
		t.Errorf("expected totalRequested 3, got %d", resp.TotalRequested)
	}
	if resp.TotalFound != 2 {
		t.Errorf("expected totalFound 2, got %d", resp.TotalFound)
	}
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(resp.Results))
	}
	if len(resp.NotFound) != 1 || resp.NotFound[0] != "33333333" {
		t.Errorf("expected notFound [33333333], got %v", resp.NotFound)
	}
}

func TestHandleBatchPrices_Deduplication(t *testing.T) {
	mock := &mockCertPriceLookup{
		purchases: map[string]*campaigns.Purchase{
			"11111111": {CertNumber: "11111111", CLValueCents: 5000},
		},
	}
	h := NewPricingAPIHandler(mock, nil)

	body := `{"certNumbers": ["11111111", "11111111", "11111111"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleBatchPrices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		TotalRequested int `json:"totalRequested"`
		TotalFound     int `json:"totalFound"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// totalRequested reflects original array length (before dedup)
	if resp.TotalRequested != 3 {
		t.Errorf("expected totalRequested 3 (pre-dedup), got %d", resp.TotalRequested)
	}
	if resp.TotalFound != 1 {
		t.Errorf("expected totalFound 1, got %d", resp.TotalFound)
	}
}

func TestHandleBatchPrices_EmptyArray(t *testing.T) {
	mock := &mockCertPriceLookup{}
	h := NewPricingAPIHandler(mock, nil)

	body := `{"certNumbers": []}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleBatchPrices(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleBatchPrices_TooManyItems(t *testing.T) {
	mock := &mockCertPriceLookup{}
	h := NewPricingAPIHandler(mock, nil)

	certs := make([]string, 101)
	for i := range certs {
		certs[i] = fmt.Sprintf("%08d", i)
	}
	bodyBytes, _ := json.Marshal(map[string][]string{"certNumbers": certs})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prices/batch", strings.NewReader(string(bodyBytes)))
	rec := httptest.NewRecorder()

	h.HandleBatchPrices(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleBatchPrices_MissingField(t *testing.T) {
	mock := &mockCertPriceLookup{}
	h := NewPricingAPIHandler(mock, nil)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleBatchPrices(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleBatchPrices_EmptyStringInArray(t *testing.T) {
	mock := &mockCertPriceLookup{}
	h := NewPricingAPIHandler(mock, nil)

	body := `{"certNumbers": ["11111111", "", "22222222"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleBatchPrices(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty string in array, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "validation_error" {
		t.Errorf("expected 'validation_error', got %q", resp["error"])
	}
}

func TestPricingAPI_EndToEnd(t *testing.T) {
	mock := &mockCertPriceLookup{
		purchases: map[string]*campaigns.Purchase{
			"11111111": {CertNumber: "11111111", CLValueCents: 9473},
			"22222222": {CertNumber: "22222222", CLValueCents: 4500},
		},
	}
	h := NewPricingAPIHandler(mock, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", h.HandleHealth)
	mux.HandleFunc("GET /api/v1/prices/{certNumber}", h.HandleSinglePrice)
	mux.HandleFunc("POST /api/v1/prices/batch", h.HandleBatchPrices)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Test health
	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health: expected 200, got %d", resp.StatusCode)
	}

	// Test single price
	resp, err = http.Get(srv.URL + "/api/v1/prices/11111111")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("single: expected 200, got %d", resp.StatusCode)
	}
	var single priceResult
	if err := json.NewDecoder(resp.Body).Decode(&single); err != nil {
		t.Fatalf("decode single response: %v", err)
	}
	if single.SuggestedPrice != 94.73 {
		t.Errorf("expected 94.73, got %v", single.SuggestedPrice)
	}

	// Test batch
	batchBody := `{"certNumbers": ["11111111", "22222222", "99999999"]}`
	resp, err = http.Post(srv.URL+"/api/v1/prices/batch", "application/json", strings.NewReader(batchBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("batch: expected 200, got %d", resp.StatusCode)
	}
	var batch batchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batch); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if batch.TotalFound != 2 {
		t.Errorf("expected totalFound 2, got %d", batch.TotalFound)
	}
	if batch.TotalRequested != 3 {
		t.Errorf("expected totalRequested 3, got %d", batch.TotalRequested)
	}
	if len(batch.NotFound) != 1 {
		t.Errorf("expected 1 notFound, got %d", len(batch.NotFound))
	}
}
