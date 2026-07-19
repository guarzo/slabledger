package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// newAdvisorHandler creates an AdvisorHandler for testing.
func newAdvisorHandler(svc advisor.Service) *AdvisorHandler {
	return NewAdvisorHandler(svc, mocks.NewMockLogger())
}

// --- HandleDigest ---

func TestHandleDigest_RequiresUser(t *testing.T) {
	h := newAdvisorHandler(&mocks.MockAdvisorService{})

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/digest", nil)
	rec := httptest.NewRecorder()
	h.HandleDigest(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleDigest_Success(t *testing.T) {
	svc := &mocks.MockAdvisorService{
		GenerateDigestFn: func(_ context.Context, stream func(advisor.StreamEvent)) error {
			stream(advisor.StreamEvent{Type: advisor.EventDelta, Content: "digest text"})
			return nil
		},
	}
	h := newAdvisorHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/digest", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleDigest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("expected 'data: [DONE]' in SSE body, got: %s", body)
	}
}

// --- HandleLiquidationAnalysis ---

func TestHandleLiquidationAnalysis_Success(t *testing.T) {
	svc := &mocks.MockAdvisorService{
		AnalyzeLiquidationFn: func(_ context.Context, stream func(advisor.StreamEvent)) error {
			stream(advisor.StreamEvent{Type: advisor.EventDelta, Content: "liquidation candidates"})
			return nil
		},
	}
	h := newAdvisorHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/liquidation", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleLiquidationAnalysis(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleLiquidationAnalysis_RequiresUser(t *testing.T) {
	h := newAdvisorHandler(&mocks.MockAdvisorService{})

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/liquidation", nil)
	rec := httptest.NewRecorder()
	h.HandleLiquidationAnalysis(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
