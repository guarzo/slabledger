package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// OpportunitiesHandler serves cross-campaign arbitrage opportunity endpoints.
type OpportunitiesHandler struct {
	svc    arbitrage.Service
	logger observability.Logger
}

// NewOpportunitiesHandler creates a new OpportunitiesHandler.
func NewOpportunitiesHandler(svc arbitrage.Service, logger observability.Logger) *OpportunitiesHandler {
	return &OpportunitiesHandler{svc: svc, logger: logger}
}

// HandleGetAcquisitionTargets returns raw-to-graded arbitrage opportunities.
func (h *OpportunitiesHandler) HandleGetAcquisitionTargets(w http.ResponseWriter, r *http.Request) {
	targets, ok := serviceCall(w, r.Context(), h.logger, "get acquisition targets", func() ([]arbitrage.AcquisitionOpportunity, error) {
		return h.svc.GetAcquisitionTargets(r.Context())
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, targets)
}

// HandleGetCrackOpportunities returns cross-campaign crack arbitrage candidates.
func (h *OpportunitiesHandler) HandleGetCrackOpportunities(w http.ResponseWriter, r *http.Request) {
	results, ok := serviceCall(w, r.Context(), h.logger, "get crack opportunities", func() ([]arbitrage.CrackAnalysis, error) {
		return h.svc.GetCrackOpportunities(r.Context())
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, results)
}
