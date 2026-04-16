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

// TestHandleFixMatch_InvalidURLOrCardID covers all 400-rejection paths driven
// by the dhUrl shape: wrong domain / wrong path / unparseable / a parseable
// URL whose card-id segment is zero. Each case asserts both the status code
// and the specific error-message snippet so a regression that swaps the two
// validation branches still fails the test.
func TestHandleFixMatch_InvalidURLOrCardID(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		wantBodyContains string
	}{
		{"wrong domain", "https://example.com/card/123/foo", "invalid DH URL"},
		{"missing card path", "https://doubleholo.com/products/123", "invalid DH URL"},
		{"missing id segment", "https://doubleholo.com/card/foo", "invalid DH URL"},
		{"empty path", "https://doubleholo.com/", "invalid DH URL"},
		{"plain text", "not a url at all", "invalid DH URL"},
		{"zero card id", "https://doubleholo.com/card/0/zero", "invalid card ID"},
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
			if !strings.Contains(rec.Body.String(), tt.wantBodyContains) {
				t.Errorf("url %q: body should contain %q, got %s", tt.url, tt.wantBodyContains, rec.Body.String())
			}
		})
	}
}
