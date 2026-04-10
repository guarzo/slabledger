package gsheets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func newTestClient(t *testing.T) *httpx.Client {
	t.Helper()
	cfg := httpx.DefaultConfig("test")
	cfg.DefaultTimeout = 5 * time.Second
	cfg.RetryPolicy.MaxRetries = 0
	return httpx.NewClient(cfg)
}

func TestClient_ReadSheet(t *testing.T) {
	successBody := sheetsValueRange{
		Range: "Sheet1!A1:Z",
		Values: [][]string{
			{"Cert Number", "Listing Title", "Grade", "Price Paid"},
			{"12345678", "2023 Pokemon Charizard PSA 10", "10", "$125.00"},
			{"87654321", "2000 Pokemon Pikachu PSA 9", "9", "$35.00"},
		},
	}

	tests := []struct {
		name         string
		serverStatus int
		serverBody   any // marshaled as JSON response body
		sheetName    string
		wantRows     int
		wantErr      bool
		checkPath    string // if non-empty, assert request path equals this
	}{
		{
			name:         "success",
			serverStatus: http.StatusOK,
			serverBody:   successBody,
			sheetName:    "Sheet1",
			wantRows:     3,
		},
		{
			name:         "empty tab defaults to Sheet1",
			serverStatus: http.StatusOK,
			serverBody:   sheetsValueRange{Values: [][]string{{"A"}}},
			sheetName:    "",
			wantRows:     1,
			checkPath:    "/v4/spreadsheets/abc123/values/Sheet1",
		},
		{
			name:         "HTTP 403 error",
			serverStatus: http.StatusForbidden,
			serverBody:   map[string]any{"error": map[string]string{"message": "not shared"}},
			sheetName:    "Sheet1",
			wantErr:      true,
		},
		{
			name:         "empty response",
			serverStatus: http.StatusOK,
			serverBody:   sheetsValueRange{Values: nil},
			sheetName:    "Sheet1",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				if auth := r.Header.Get("Authorization"); auth == "" {
					t.Error("missing Authorization header")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverStatus)
				json.NewEncoder(w).Encode(tt.serverBody)
			}))
			defer server.Close()

			testHTTPX := newTestClient(t)
			client := &Client{
				baseURL:    server.URL,
				dataClient: testHTTPX,
				token:      &cachedToken{},
				logger:     observability.NewNoopLogger(),
			}
			client.token.set("test-token", time.Now().Add(1*time.Hour))

			spreadsheetID := "abc123"
			rows, err := client.ReadSheet(context.Background(), spreadsheetID, tt.sheetName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(rows) != tt.wantRows {
				t.Fatalf("got %d rows, want %d", len(rows), tt.wantRows)
			}
			if tt.checkPath != "" && capturedPath != tt.checkPath {
				t.Errorf("path = %q, want %q", capturedPath, tt.checkPath)
			}
		})
	}
}

func TestClient_ReadSheet_UsesHTTPX(t *testing.T) {
	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sheetsValueRange{
			Values: [][]string{{"header"}, {"row1"}},
		})
	}))
	defer server.Close()

	testHTTPX := newTestClient(t)
	client := &Client{
		baseURL:    server.URL,
		dataClient: testHTTPX,
		token:      &cachedToken{},
		logger:     observability.NewNoopLogger(),
	}
	client.token.set("test-token", time.Now().Add(1*time.Hour))

	rows, err := client.ReadSheet(context.Background(), "sheet-id", "Tab1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// httpx sets User-Agent automatically
	if gotUserAgent == "" {
		t.Error("expected User-Agent header from httpx client, got empty")
	}
}
