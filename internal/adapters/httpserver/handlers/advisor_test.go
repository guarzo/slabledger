package handlers

import (
	"bytes"
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
	return NewAdvisorHandler(svc, nil, mocks.NewMockLogger())
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

// --- HandleCampaignAnalysis ---

func TestHandleCampaignAnalysis_MissingCampaignID(t *testing.T) {
	h := newAdvisorHandler(&mocks.MockAdvisorService{})

	body := `{"campaignId":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/advisor/campaign", bytes.NewBufferString(body))
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleCampaignAnalysis(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCampaignAnalysis_Success(t *testing.T) {
	svc := &mocks.MockAdvisorService{
		AnalyzeCampaignFn: func(_ context.Context, campaignID string, stream func(advisor.StreamEvent)) error {
			stream(advisor.StreamEvent{Type: advisor.EventDelta, Content: "campaign analysis for " + campaignID})
			return nil
		},
	}
	h := newAdvisorHandler(svc)

	body := `{"campaignId":"c1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/advisor/campaign", bytes.NewBufferString(body))
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleCampaignAnalysis(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Errorf("expected DONE sentinel in SSE body")
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
