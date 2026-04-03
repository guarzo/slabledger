package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// OpportunitiesHandler serves cross-campaign arbitrage opportunity endpoints.
type OpportunitiesHandler struct {
	svc    campaigns.Service
	logger observability.Logger
}

// NewOpportunitiesHandler creates a new OpportunitiesHandler.
func NewOpportunitiesHandler(svc campaigns.Service, logger observability.Logger) *OpportunitiesHandler {
	return &OpportunitiesHandler{svc: svc, logger: logger}
}

// HandleGetAcquisitionTargets returns raw-to-graded arbitrage opportunities.
func (h *OpportunitiesHandler) HandleGetAcquisitionTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := h.svc.GetAcquisitionTargets(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "get acquisition targets", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get acquisition targets")
		return
	}
	writeJSONList(w, http.StatusOK, targets)
}

// HandleGetCrackOpportunities returns cross-campaign crack arbitrage candidates.
func (h *OpportunitiesHandler) HandleGetCrackOpportunities(w http.ResponseWriter, r *http.Request) {
	results, err := h.svc.GetCrackOpportunities(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "get crack opportunities", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get crack opportunities")
		return
	}
	writeJSONList(w, http.StatusOK, results)
}
