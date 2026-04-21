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

// HandleUnmatchDH clears the DH match data for a purchase and resets its
// push status to "unmatched", allowing it to be retried or fixed manually.
//
// Ordering: clear local DB state first, then (best-effort) take down live
// channel listings and drop the cached mapping. If the DB updates fail we
// return an error without mutating external state, so retrying is safe.
// DH has no "delete inventory item" endpoint today — once it ships, add it
// alongside the channel delist call.
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

	// Capture the inventory ID before we clear it — we need it for the
	// post-commit delist call, and UpdatePurchaseDHFields below zeroes the
	// DB column (the in-memory struct isn't reloaded).
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

	// Local state is committed — now do the best-effort external and cache
	// cleanup. Failures here are logged but don't fail the request: the user
	// has successfully unmatched, external state can be reconciled out-of-band.
	if h.channelDelister != nil && dhID != 0 {
		if _, derr := h.channelDelister.DelistChannels(ctx, dhID, nil); derr != nil {
			h.logger.Warn(ctx, "unmatch dh: delist channels failed, continuing",
				observability.String("purchaseID", purchase.ID),
				observability.Int("dhInventoryID", dhID),
				observability.Err(derr))
		}
	}

	// Remove the auto card_id_mappings entry so the next push cycle doesn't
	// pull the known-bad dh_card_id out of the scheduler's mappedSet cache.
	// Without this, CL refresh will re-enroll the purchase to 'pending',
	// the scheduler will find the cached mapping, and DH gets the same
	// wrong (card_id, cert_number) pair on the next request.
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
