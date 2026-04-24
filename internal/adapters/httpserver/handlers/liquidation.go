package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/liquidation"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type LiquidationHandler struct {
	svc    liquidation.Service
	logger observability.Logger
}

func NewLiquidationHandler(svc liquidation.Service, logger observability.Logger) *LiquidationHandler {
	return &LiquidationHandler{svc: svc, logger: logger}
}

func (h *LiquidationHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	var req liquidation.PreviewRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	resp, err := h.svc.Preview(r.Context(), req)
	if err != nil {
		h.logger.Error(r.Context(), "liquidation preview failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "preview failed")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *LiquidationHandler) HandleApply(w http.ResponseWriter, r *http.Request) {
	var req liquidation.ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "no items to apply")
		return
	}
	result, err := h.svc.Apply(r.Context(), req)
	if err != nil {
		h.logger.Error(r.Context(), "liquidation apply failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "apply failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
