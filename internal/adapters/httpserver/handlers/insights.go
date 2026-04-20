package handlers

import (
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
	if requireUser(w, r) == nil {
		return
	}
	overview, ok := serviceCall(w, r.Context(), h.logger, "insights overview failed", func() (*insights.Overview, error) {
		return h.svc.GetOverview(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, overview)
}
