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
		serverBody   interface{} // marshaled as JSON response body
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
			serverBody:   map[string]interface{}{"error": map[string]string{"message": "not shared"}},
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
				w.WriteHeader(tt.serverStatus)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.serverBody)
			}))
			defer server.Close()

			client := &Client{
				baseURL: server.URL,
				token:   &cachedToken{},
				logger:  newTestLogger(),
			}
			client.token.set("test-token", testFutureTime())

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
