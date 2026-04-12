package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type dhDismissRequest struct {
	PurchaseID string `json:"purchaseId"`
}

// HandleDismissMatch marks an unmatched purchase as "dismissed" (not matchable).
func (h *DHHandler) HandleDismissMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	var req dhDismissRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PurchaseID == "" {
		writeError(w, http.StatusBadRequest, "purchaseId is required")
		return
	}

	// Verify the purchase exists and is currently unmatched.
	p, err := h.purchaseLister.GetPurchase(ctx, req.PurchaseID)
	if err != nil {
		h.logger.Error(ctx, "dismiss: get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get purchase")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "purchase not found")
		return
	}
	if p.DHPushStatus != inventory.DHPushStatusUnmatched {
		writeError(w, http.StatusConflict, "purchase is not in unmatched state")
		return
	}

	if h.pushStatusUpdater == nil {
		writeError(w, http.StatusInternalServerError, "push status updater not configured")
		return
	}
	if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, req.PurchaseID, inventory.DHPushStatusDismissed); err != nil {
		h.logger.Error(ctx, "dismiss: update push status", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to dismiss purchase")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

// HandleUndismissMatch restores a dismissed purchase back to "unmatched" so it
// can be re-attempted or manually matched.
func (h *DHHandler) HandleUndismissMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	var req dhDismissRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PurchaseID == "" {
		writeError(w, http.StatusBadRequest, "purchaseId is required")
		return
	}

	// Verify the purchase exists and is currently dismissed.
	p, err := h.purchaseLister.GetPurchase(ctx, req.PurchaseID)
	if err != nil {
		h.logger.Error(ctx, "undismiss: get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get purchase")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "purchase not found")
		return
	}
	if p.DHPushStatus != inventory.DHPushStatusDismissed {
		writeError(w, http.StatusConflict, "purchase is not in dismissed state")
		return
	}

	if h.pushStatusUpdater == nil {
		writeError(w, http.StatusInternalServerError, "push status updater not configured")
		return
	}
	if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, req.PurchaseID, inventory.DHPushStatusUnmatched); err != nil {
		h.logger.Error(ctx, "undismiss: update push status", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to undismiss purchase")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unmatched"})
}
