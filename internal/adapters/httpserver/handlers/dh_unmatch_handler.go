package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type unmatchDHRequest struct {
	PurchaseID string `json:"purchaseId"`
}

// HandleUnmatchDH clears the DH match data for a purchase and resets its
// push status to "unmatched", allowing it to be retried or fixed manually.
func (h *DHHandler) HandleUnmatchDH(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	var req unmatchDHRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.PurchaseID == "" {
		writeError(w, http.StatusBadRequest, "purchaseId is required")
		return
	}

	purchase, err := h.purchaseLister.GetPurchase(ctx, req.PurchaseID)
	if err != nil {
		if inventory.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "purchase not found")
			return
		}
		h.logger.Error(ctx, "unmatch dh: get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to look up purchase")
		return
	}

	// Clear DH card ID and inventory ID.
	if h.dhFieldsUpdater != nil {
		if err := h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, purchase.ID, inventory.DHFieldsUpdate{
			CardID:      0,
			InventoryID: 0,
		}); err != nil {
			h.logger.Error(ctx, "unmatch dh: clear dh fields", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "failed to clear DH fields")
			return
		}
	}

	// Reset push status to unmatched.
	if h.pushStatusUpdater != nil {
		if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, inventory.DHPushStatusUnmatched); err != nil {
			h.logger.Error(ctx, "unmatch dh: set unmatched status", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "failed to update push status")
			return
		}
	}

	// Clear any stored candidates.
	if h.candidatesSaver != nil {
		if err := h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, purchase.ID, ""); err != nil {
			h.logger.Warn(ctx, "unmatch dh: failed to clear candidates",
				observability.String("purchaseID", purchase.ID), observability.Err(err))
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
