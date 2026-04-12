package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SellSheetItemsHandler handles sell sheet item persistence endpoints.
type SellSheetItemsHandler struct {
	repo   inventory.SellSheetRepository
	logger observability.Logger
}

// NewSellSheetItemsHandler creates a new sell sheet items handler.
func NewSellSheetItemsHandler(repo inventory.SellSheetRepository, logger observability.Logger) *SellSheetItemsHandler {
	return &SellSheetItemsHandler{repo: repo, logger: logger}
}

// HandleGetItems handles GET /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleGetItems(w http.ResponseWriter, r *http.Request) {
	ids, ok := serviceCall(w, r.Context(), h.logger, "get sell sheet items failed", func() ([]string, error) {
		return h.repo.GetSellSheetItems(r.Context())
	})
	if !ok {
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"purchaseIds": ids})
}

// HandleAddItems handles PUT /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleAddItems(w http.ResponseWriter, r *http.Request) {
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
	if !serviceCallVoid(w, r.Context(), h.logger, "add sell sheet items failed", func() error {
		return h.repo.AddSellSheetItems(r.Context(), req.PurchaseIDs)
	}) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveItems handles DELETE /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleRemoveItems(w http.ResponseWriter, r *http.Request) {
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
	if !serviceCallVoid(w, r.Context(), h.logger, "remove sell sheet items failed", func() error {
		return h.repo.RemoveSellSheetItems(r.Context(), req.PurchaseIDs)
	}) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleClearItems handles DELETE /api/sell-sheet/items/all.
func (h *SellSheetItemsHandler) HandleClearItems(w http.ResponseWriter, r *http.Request) {
	if !serviceCallVoid(w, r.Context(), h.logger, "clear sell sheet failed", func() error {
		return h.repo.ClearSellSheet(r.Context())
	}) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
