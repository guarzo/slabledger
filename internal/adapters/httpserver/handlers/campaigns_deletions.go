package handlers

import (
	"errors"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func writeReturnConflictIfPresent(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, inventory.ErrReturnConflict) {
		return false
	}
	writeConfirmedReturnError(w, err)
	return true
}

// HandleDeletePurchase handles DELETE /api/campaigns/{id}/purchases/{purchaseId}.
func (h *CampaignsHandler) HandleDeletePurchase(w http.ResponseWriter, r *http.Request) {
	campaignID, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	// Verify the purchase belongs to this campaign
	_, ok = h.requirePurchaseInCampaign(w, r, campaignID, purchaseID)
	if !ok {
		return
	}

	if err := h.service.DeletePurchase(r.Context(), purchaseID); err != nil {
		if writeReturnConflictIfPresent(w, err) {
			return
		}
		if inventory.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		h.logger.Error(r.Context(), "failed to delete purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleDeleteSale handles DELETE /api/campaigns/{id}/purchases/{purchaseId}/sale.
func (h *CampaignsHandler) HandleDeleteSale(w http.ResponseWriter, r *http.Request) {
	campaignID, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	// Verify the purchase belongs to this campaign
	_, ok = h.requirePurchaseInCampaign(w, r, campaignID, purchaseID)
	if !ok {
		return
	}

	if err := h.service.DeleteSaleByPurchaseID(r.Context(), purchaseID); err != nil {
		if writeReturnConflictIfPresent(w, err) {
			return
		}
		if inventory.IsSaleNotFound(err) {
			writeError(w, http.StatusNotFound, "No sale found for this purchase")
			return
		}
		h.logger.Error(r.Context(), "failed to delete sale", observability.Err(err), observability.String("purchase_id", purchaseID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
