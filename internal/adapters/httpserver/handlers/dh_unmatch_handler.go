package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

type unmatchDHRequest struct {
	PurchaseID string `json:"purchaseId"`
}

// HandleUnmatchDH clears the DH match data for a purchase and re-queues it
// for matching. For received items it sets status directly to "pending" so
// the push scheduler picks it up on the next cycle without waiting for a CL
// price change. For not-yet-received items it sets "unmatched".
//
// Ordering: clear local DB state first, then best-effort delete the DH
// inventory item (which also cancels any active market order) and drop the
// cached mapping. DB failures abort and return an error; DH-side failures are
// logged and do not fail the request so the caller can retry.
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

	if purchase.DHPushStatus != inventory.DHPushStatusMatched {
		h.logger.Warn(ctx, "unmatch dh: invalid state for unmatch",
			observability.String("purchaseID", purchase.ID),
			observability.String("dhPushStatus", purchase.DHPushStatus))
		writeError(w, http.StatusConflict, "invalid purchase state for unmatch: purchase is not matched")
		return
	}

	// Capture the inventory ID before we clear it — UpdatePurchaseDHFields
	// zeroes the DB column and we need it for the post-commit DH delete.
	dhID := purchase.DHInventoryID

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

	// Re-queue: received items go straight to pending so the push scheduler
	// retries on the next cycle. Not-yet-received items stay unmatched (they
	// can't be pushed until the card arrives).
	newStatus := inventory.DHPushStatusUnmatched
	if purchase.ReceivedAt != nil {
		newStatus = inventory.DHPushStatusPending
	}
	if h.pushStatusUpdater != nil {
		if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, newStatus); err != nil {
			h.logger.Error(ctx, "unmatch dh: set push status", observability.Err(err))
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

	// Local state is committed — now do best-effort external and cache cleanup.
	// Failures are logged but don't fail the request.

	// Delete the DH inventory item. This cancels any active market order and
	// delists from all channels in one transaction on DH's side.
	if h.inventoryDeleter != nil && dhID != 0 {
		if derr := h.inventoryDeleter.DeleteInventory(ctx, dhID); derr != nil {
			h.logger.Warn(ctx, "unmatch dh: delete inventory failed, continuing",
				observability.String("purchaseID", purchase.ID),
				observability.Int("dhInventoryID", dhID),
				observability.Err(derr))
		}
	}

	// Remove the auto card_id_mappings entry so the push scheduler doesn't
	// reuse a known-bad dh_card_id on the next cycle.
	if h.mappingDeleter != nil {
		if rows, derr := h.mappingDeleter.DeleteAutoMapping(ctx, purchase.CardName, purchase.SetName, purchase.CardNumber, pricing.SourceDH); derr != nil {
			h.logger.Warn(ctx, "unmatch dh: failed to delete auto card id mapping, continuing",
				observability.String("purchaseID", purchase.ID),
				observability.String("cardName", purchase.CardName),
				observability.String("setName", purchase.SetName),
				observability.String("cardNumber", purchase.CardNumber),
				observability.Err(derr))
		} else if rows > 0 {
			h.logger.Info(ctx, "unmatch dh: removed auto card id mapping",
				observability.String("purchaseID", purchase.ID),
				observability.String("cardName", purchase.CardName),
				observability.String("setName", purchase.SetName),
				observability.String("cardNumber", purchase.CardNumber),
				observability.Int64("rows", rows))
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
