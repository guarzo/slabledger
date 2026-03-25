package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHealthCheck_MethodNotAllowed(t *testing.T) {
	handler := &HealthHandler{}

	tests := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range tests {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/health", nil)
			w := httptest.NewRecorder()
			handler.HandleHealthCheck(w, req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("HandleHealthCheck with %s: got status %d, want %d",
					method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHandleHealthCheck_GET(t *testing.T) {
	handler := &HealthHandler{}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.HandleHealthCheck(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HandleHealthCheck GET: got status %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response body: %v", err)
	}

	if _, ok := resp["status"]; !ok {
		t.Error("Expected 'status' field in response")
	}
	if _, ok := resp["providers"]; !ok {
		t.Error("Expected 'providers' field in response")
	}
}

func TestHandleHealthCheck_HEAD(t *testing.T) {
	handler := &HealthHandler{}

	req := httptest.NewRequest(http.MethodHead, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.HandleHealthCheck(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HandleHealthCheck HEAD: got status %d, want %d", w.Code, http.StatusOK)
	}
}
