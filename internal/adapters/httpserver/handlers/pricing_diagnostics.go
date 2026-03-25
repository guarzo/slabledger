package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// PricingDiagnosticsHandler serves pricing data quality diagnostics.
type PricingDiagnosticsHandler struct {
	provider pricing.PricingDiagnosticsProvider
	logger   observability.Logger
}

// NewPricingDiagnosticsHandler creates a new handler for the diagnostics endpoint.
func NewPricingDiagnosticsHandler(provider pricing.PricingDiagnosticsProvider, logger observability.Logger) *PricingDiagnosticsHandler {
	return &PricingDiagnosticsHandler{provider: provider, logger: logger}
}

// HandlePricingDiagnostics handles GET /api/admin/pricing-diagnostics.
func (h *PricingDiagnosticsHandler) HandlePricingDiagnostics(w http.ResponseWriter, r *http.Request) {
	diag, err := h.provider.GetPricingDiagnostics(r.Context())
	if err != nil {
		h.logger.Warn(r.Context(), "failed to get pricing diagnostics", observability.Err(err))
		http.Error(w, "Failed to get pricing diagnostics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(diag); err != nil {
		h.logger.Warn(r.Context(), "failed to encode diagnostics response", observability.Err(err))
	}
}
