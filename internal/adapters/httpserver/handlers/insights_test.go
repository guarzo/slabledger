package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/domain/insights"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

type stubInsightsSvc struct {
	resp *insights.Overview
	err  error
}

func (s *stubInsightsSvc) GetOverview(ctx context.Context) (*insights.Overview, error) {
	return s.resp, s.err
}

func TestInsightsHandler_HandleOverview_Success(t *testing.T) {
	t.Parallel()
	svc := &stubInsightsSvc{resp: &insights.Overview{GeneratedAt: "2026-04-20T00:00:00Z"}}
	h := handlers.NewInsightsHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/insights/overview", nil)
	w := httptest.NewRecorder()
	h.HandleOverview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got insights.Overview
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.GeneratedAt != "2026-04-20T00:00:00Z" {
		t.Errorf("GeneratedAt = %q", got.GeneratedAt)
	}
}

func TestInsightsHandler_HandleOverview_ServiceError(t *testing.T) {
	t.Parallel()
	svc := &stubInsightsSvc{err: errors.New("boom")}
	h := handlers.NewInsightsHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/insights/overview", nil)
	w := httptest.NewRecorder()
	h.HandleOverview(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}
