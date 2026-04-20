package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/insights"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// InsightsHandler serves the aggregate /api/insights/overview endpoint.
type InsightsHandler struct {
	svc    insights.Service
	logger observability.Logger
}

// NewInsightsHandler wires the insights service into the HTTP layer.
func NewInsightsHandler(svc insights.Service, logger observability.Logger) *InsightsHandler {
	return &InsightsHandler{svc: svc, logger: logger}
}

// HandleOverview serves GET /api/insights/overview.
func (h *InsightsHandler) HandleOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.svc.GetOverview(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "insights overview failed",
			observability.String("err", err.Error()))
		http.Error(w, "insights overview failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(overview); err != nil {
		h.logger.Error(r.Context(), "insights overview encode failed",
			observability.String("err", err.Error()))
	}
}
