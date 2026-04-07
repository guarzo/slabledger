package handlers

import (
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// PriceFlagsHandler handles admin price flag review endpoints.
type PriceFlagsHandler struct {
	service campaigns.Service
	logger  observability.Logger
}

// NewPriceFlagsHandler creates a new PriceFlagsHandler.
func NewPriceFlagsHandler(service campaigns.Service, logger observability.Logger) *PriceFlagsHandler {
	return &PriceFlagsHandler{service: service, logger: logger}
}

// HandleListPriceFlags handles GET /api/admin/price-flags?status=open|resolved|all.
func (h *PriceFlagsHandler) HandleListPriceFlags(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "open", "resolved", "all":
		// valid
	case "":
		status = "open"
	default:
		writeError(w, http.StatusBadRequest, "invalid status: must be open, resolved, or all")
		return
	}
	flags, ok := serviceCall(w, r.Context(), h.logger, "failed to list price flags", func() ([]campaigns.PriceFlagWithContext, error) {
		return h.service.ListPriceFlags(r.Context(), status)
	})
	if !ok {
		return
	}
	if flags == nil {
		flags = []campaigns.PriceFlagWithContext{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"flags": flags,
		"total": len(flags),
	})
}

// HandleResolvePriceFlag handles PATCH /api/admin/price-flags/{flagId}/resolve.
func (h *PriceFlagsHandler) HandleResolvePriceFlag(w http.ResponseWriter, r *http.Request) {
	flagIDStr := r.PathValue("flagId")
	flagID, err := strconv.ParseInt(flagIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid flag ID")
		return
	}
	user := requireUser(w, r)
	if user == nil {
		return
	}
	if err := h.service.ResolvePriceFlag(r.Context(), flagID, user.ID); err != nil {
		if campaigns.IsPriceFlagNotFound(err) {
			writeError(w, http.StatusNotFound, "Price flag not found or already resolved")
			return
		}
		h.logger.Error(r.Context(), "failed to resolve price flag", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
