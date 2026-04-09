package gsheets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func newTestLogger() observability.Logger {
	return observability.NewNoopLogger()
}

func testFutureTime() time.Time {
	return time.Now().Add(1 * time.Hour)
}

func TestClient_ReadSheet(t *testing.T) {
	sheetData := sheetsValueRange{
		Range: "Sheet1!A1:Z",
		Values: [][]string{
			{"Cert Number", "Listing Title", "Grade", "Price Paid"},
			{"12345678", "2023 Pokemon Charizard PSA 10", "10", "$125.00"},
			{"87654321", "2000 Pokemon Pikachu PSA 9", "9", "$35.00"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Error("missing Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sheetData)
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   &cachedToken{},
		logger:  newTestLogger(),
	}
	client.token.set("test-token", testFutureTime())

	rows, err := client.ReadSheet(context.Background(), "spreadsheet-id", "Sheet1")
	if err != nil {
		t.Fatalf("ReadSheet: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0][0] != "Cert Number" {
		t.Errorf("got header %q, want 'Cert Number'", rows[0][0])
	}
	if rows[1][0] != "12345678" {
		t.Errorf("got cert %q, want '12345678'", rows[1][0])
	}
}

func TestClient_ReadSheet_EmptyTab(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		json.NewEncoder(w).Encode(sheetsValueRange{Values: [][]string{{"A"}}})
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   &cachedToken{},
		logger:  newTestLogger(),
	}
	client.token.set("test-token", testFutureTime())

	_, err := client.ReadSheet(context.Background(), "abc123", "")
	if err != nil {
		t.Fatalf("ReadSheet: %v", err)
	}
	want := "/v4/spreadsheets/abc123/values/Sheet1"
	if capturedPath != want {
		t.Errorf("path = %q, want %q", capturedPath, want)
	}
}

func TestClient_ReadSheet_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": {"message": "not shared"}}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   &cachedToken{},
		logger:  newTestLogger(),
	}
	client.token.set("test-token", testFutureTime())

	_, err := client.ReadSheet(context.Background(), "id", "Sheet1")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestClient_ReadSheet_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sheetsValueRange{Values: nil})
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   &cachedToken{},
		logger:  newTestLogger(),
	}
	client.token.set("test-token", testFutureTime())

	_, err := client.ReadSheet(context.Background(), "id", "Sheet1")
	if err == nil {
		t.Fatal("expected error for empty sheet")
	}
}
