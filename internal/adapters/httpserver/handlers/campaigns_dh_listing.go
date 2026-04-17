package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleListPurchaseOnDH handles POST /api/purchases/{purchaseId}/list-on-dh.
// Manually transitions a purchase from in_stock to listed on DH. The purchase
// must be received and already pushed to DH inventory.
func (h *CampaignsHandler) HandleListPurchaseOnDH(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	if h.dhListingSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "DH listing service not configured")
		return
	}

	p, err := h.service.GetPurchase(r.Context(), purchaseID)
	if err != nil {
		if inventory.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get purchase for DH listing",
			observability.Err(err), observability.String("purchaseId", purchaseID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if p.ReceivedAt == nil {
		writeError(w, http.StatusConflict, "Purchase has not been received yet")
		return
	}
	if p.DHStatus == inventory.DHStatusListed {
		writeError(w, http.StatusConflict, "Purchase already listed on DH")
		return
	}
	// When the item has an inventory ID, it must be in_stock to transition
	// to listed. When it doesn't, the listing service will do an inline
	// match + push first (for DHPushStatusPending items), then list.
	if p.DHInventoryID != 0 && p.DHStatus != inventory.DHStatusInStock {
		writeError(w, http.StatusConflict, "Purchase is not in_stock on DH")
		return
	}
	if p.DHInventoryID == 0 && p.DHPushStatus != inventory.DHPushStatusPending {
		// Not yet pushed and not eligible for inline push (held / unmatched /
		// dismissed / empty). Reject with a reason the user can act on.
		switch p.DHPushStatus {
		case inventory.DHPushStatusHeld:
			writeError(w, http.StatusConflict, "DH push is held for review — approve it first")
		case inventory.DHPushStatusUnmatched:
			writeError(w, http.StatusConflict, "Cert could not be matched to a DH card")
		case inventory.DHPushStatusDismissed:
			writeError(w, http.StatusConflict, "DH push was dismissed for this purchase")
		default:
			writeError(w, http.StatusConflict, "Purchase not yet pushed to DH inventory")
		}
		return
	}
	// DH now honors our listing_price_cents as-is, so we require a reviewed
	// price before listing. Stale or missing prices are rejected here rather
	// than silently letting DH fall back to its catalog value.
	if p.ReviewedPriceCents == 0 {
		writeError(w, http.StatusConflict, "Review the price before listing on DH")
		return
	}

	result := h.dhListingSvc.ListPurchases(r.Context(), []string{p.CertNumber})
	if result.Error != nil {
		h.logger.Error(r.Context(), "dh listing failed",
			observability.Err(result.Error), observability.String("purchaseId", purchaseID))
		writeError(w, http.StatusInternalServerError, "DH listing failed")
		return
	}
	if result.Listed == 0 {
		// Re-read the purchase to give a specific reason for the failure.
		updated, readErr := h.service.GetPurchase(r.Context(), purchaseID)
		if readErr != nil {
			writeError(w, http.StatusBadGateway, "DH listing failed — check server logs for details")
			return
		}
		if updated.DHStatus == inventory.DHStatusListed {
			writeJSON(w, http.StatusOK, result)
			return
		}
		if updated.DHInventoryID == 0 {
			// Either the inline push couldn't match this cert, or a stale inventory
			// ID was reset after ERR_PROV_NOT_FOUND. Either way the push scheduler
			// will retry automatically on its next cycle.
			writeError(w, http.StatusBadGateway, "DH push failed — will retry automatically on next sync")
			return
		}
		writeError(w, http.StatusBadGateway, "DH listing failed — check server logs for details")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// triggerDHListing runs in the background so it doesn't delay the HTTP response.
func (h *CampaignsHandler) triggerDHListing(certNumbers []string) {
	if h.dhListingSvc == nil || len(certNumbers) == 0 {
		return
	}

	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error(h.baseCtx, "panic in triggerDHListing",
					observability.String("panic", fmt.Sprintf("%v", r)))
			}
		}()
		ctx, cancel := context.WithTimeout(h.baseCtx, 5*time.Minute)
		defer cancel()

		result := h.dhListingSvc.ListPurchases(ctx, certNumbers)
		h.logger.Info(ctx, "dh listing goroutine completed",
			observability.Int("listed", result.Listed),
			observability.Int("synced", result.Synced),
			observability.Int("total", result.Total))
	}()
}
