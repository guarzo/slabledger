package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SellSheetItemsHandler handles sell sheet item persistence endpoints.
type SellSheetItemsHandler struct {
	repo   campaigns.SellSheetRepository
	logger observability.Logger
}

// NewSellSheetItemsHandler creates a new sell sheet items handler.
func NewSellSheetItemsHandler(repo campaigns.SellSheetRepository, logger observability.Logger) *SellSheetItemsHandler {
	return &SellSheetItemsHandler{repo: repo, logger: logger}
}

// HandleGetItems handles GET /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleGetItems(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	ids, err := h.repo.GetSellSheetItems(r.Context(), user.ID)
	if err != nil {
		h.logger.Error(r.Context(), "get sell sheet items failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"purchaseIds": ids})
}

// HandleAddItems handles PUT /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleAddItems(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	var req struct {
		PurchaseIDs []string `json:"purchaseIds"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.PurchaseIDs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one purchase ID is required")
		return
	}
	if err := h.repo.AddSellSheetItems(r.Context(), user.ID, req.PurchaseIDs); err != nil {
		h.logger.Error(r.Context(), "add sell sheet items failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveItems handles DELETE /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleRemoveItems(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	var req struct {
		PurchaseIDs []string `json:"purchaseIds"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.PurchaseIDs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one purchase ID is required")
		return
	}
	if err := h.repo.RemoveSellSheetItems(r.Context(), user.ID, req.PurchaseIDs); err != nil {
		h.logger.Error(r.Context(), "remove sell sheet items failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleClearItems handles DELETE /api/sell-sheet/items/all.
func (h *SellSheetItemsHandler) HandleClearItems(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	if err := h.repo.ClearSellSheet(r.Context(), user.ID); err != nil {
		h.logger.Error(r.Context(), "clear sell sheet failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
