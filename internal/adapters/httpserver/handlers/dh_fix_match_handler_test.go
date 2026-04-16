package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// fixMatchValidationHandler builds a DHHandler with no production deps wired,
// suitable for testing the request-validation paths that short-circuit before
// any service call.
func fixMatchValidationHandler() *DHHandler {
	return NewDHHandler(DHHandlerDeps{
		Logger:  mocks.NewMockLogger(),
		BaseCtx: context.Background(),
	})
}

// TestHandleFixMatch_AuthAndBody covers the early request-validation guards
// that don't touch any service dependency: missing user → 401 and malformed
// JSON body → 400.
func TestHandleFixMatch_AuthAndBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		withAuth bool
		wantCode int
	}{
		{
			name:     "no user → 401",
			body:     `{"purchaseId":"p1","dhUrl":"https://doubleholo.com/card/123/foo"}`,
			withAuth: false,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "malformed JSON body → 400",
			body:     `{not json`,
			withAuth: true,
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := fixMatchValidationHandler()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/dh/fix-match", strings.NewReader(tt.body))
			if tt.withAuth {
				req = withUser(req)
			}
			h.HandleFixMatch(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
		})
	}
}

func TestHandleFixMatch_RequiredFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"missing purchaseId", `{"dhUrl":"https://doubleholo.com/card/123/x"}`, "purchaseId is required"},
		{"empty purchaseId", `{"purchaseId":"","dhUrl":"https://doubleholo.com/card/123/x"}`, "purchaseId is required"},
		{"missing dhUrl", `{"purchaseId":"p1"}`, "dhUrl is required"},
		{"empty dhUrl", `{"purchaseId":"p1","dhUrl":""}`, "dhUrl is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := fixMatchValidationHandler()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/dh/fix-match", strings.NewReader(tt.body))
			req = withUser(req)
			h.HandleFixMatch(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d, want 400 (body=%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.want)
			}
		})
	}
}

func TestHandleFixMatch_InvalidDHURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"wrong domain", "https://example.com/card/123/foo"},
		{"missing card path", "https://doubleholo.com/products/123"},
		{"missing id segment", "https://doubleholo.com/card/foo"},
		{"empty path", "https://doubleholo.com/"},
		{"plain text", "not a url at all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := fixMatchValidationHandler()

			body := `{"purchaseId":"p1","dhUrl":"` + tt.url + `"}`
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/dh/fix-match", strings.NewReader(body))
			req = withUser(req)
			h.HandleFixMatch(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("url %q: got %d, want 400 (body=%s)", tt.url, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "invalid DH URL") {
				t.Errorf("url %q: body should mention invalid DH URL, got %s", tt.url, rec.Body.String())
			}
		})
	}
}

func TestHandleFixMatch_ZeroCardIDRejected(t *testing.T) {
	h := fixMatchValidationHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dh/fix-match",
		strings.NewReader(`{"purchaseId":"p1","dhUrl":"https://doubleholo.com/card/0/zero"}`))
	req = withUser(req)
	h.HandleFixMatch(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid card ID") {
		t.Errorf("body should mention invalid card ID, got %s", rec.Body.String())
	}
}
