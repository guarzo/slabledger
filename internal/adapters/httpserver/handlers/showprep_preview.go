package handlers

import (
	"net/http"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

func (h *ShowPrepHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PurchaseID string `json:"purchaseId"`
		PriceCents int    `json:"priceCents"`
	}
	if !showPrepDecode(w, r, &req) {
		return
	}
	if !showPrepID(req.PurchaseID) || req.PriceCents < 1 || req.PriceCents > sp.MaxPreviewPriceCents {
		writeError(w, http.StatusBadRequest, "invalid purchase UUID or trial price cents")
		return
	}
	preview, err := h.svc.Preview(r.Context(), req.PurchaseID, req.PriceCents)
	h.respond(w, r, preview, err)
}
