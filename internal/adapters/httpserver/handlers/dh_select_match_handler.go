package handlers

import (
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

type selectMatchRequest struct {
	PurchaseID string `json:"purchaseId"`
	DHCardID   int    `json:"dhCardId"`
}

func (h *DHHandler) HandleSelectMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	var req selectMatchRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.PurchaseID == "" {
		writeError(w, http.StatusBadRequest, "purchaseId is required")
		return
	}
	if req.DHCardID == 0 {
		writeError(w, http.StatusBadRequest, "dhCardId is required")
		return
	}

	purchase, err := h.purchaseLister.GetPurchase(ctx, req.PurchaseID)
	if err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "purchase not found")
			return
		}
		h.logger.Error(ctx, "select match: get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to look up purchase")
		return
	}

	externalID := strconv.Itoa(req.DHCardID)
	if err := h.cardIDSaver.SaveExternalID(ctx, purchase.CardName, purchase.SetName, purchase.CardNumber, pricing.SourceDH, externalID); err != nil {
		h.logger.Error(ctx, "select match: save external ID", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save card mapping")
		return
	}

	item := dh.InventoryItem{
		DHCardID:       req.DHCardID,
		CertNumber:     purchase.CertNumber,
		GradingCompany: dh.GraderPSA,
		Grade:          purchase.GradeValue,
		CostBasisCents: purchase.CLValueCents,
		Status:         dh.InventoryStatusInStock,
	}

	pushResp, pushErr := h.inventoryPusher.PushInventory(ctx, []dh.InventoryItem{item})
	if pushErr != nil {
		h.logger.Error(ctx, "select match: push inventory", observability.Err(pushErr))
		writeError(w, http.StatusBadGateway, "DH API error")
		return
	}

	var inventoryID int
	for _, result := range pushResp.Results {
		if result.Status != "failed" && result.DHInventoryID != 0 {
			inventoryID = result.DHInventoryID
			if h.dhFieldsUpdater != nil {
				if err := h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, purchase.ID, campaigns.DHFieldsUpdate{
					CardID:            req.DHCardID,
					InventoryID:       result.DHInventoryID,
					CertStatus:        dh.CertStatusMatched,
					ListingPriceCents: result.AssignedPriceCents,
					ChannelsJSON:      dh.MarshalChannels(result.Channels),
					DHStatus:          campaigns.DHStatus(result.Status),
				}); err != nil {
					h.logger.Warn(ctx, "select match: failed to persist DH fields",
						observability.String("purchaseID", purchase.ID), observability.Err(err))
				}
			}
			break
		}
	}

	if inventoryID == 0 {
		writeError(w, http.StatusBadGateway, "DH push failed — no inventory ID returned")
		return
	}

	if h.pushStatusUpdater != nil {
		if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, campaigns.DHPushStatusManual); err != nil {
			h.logger.Warn(ctx, "select match: failed to set manual status",
				observability.String("purchaseID", purchase.ID), observability.Err(err))
		}
	}

	// Clear stored candidates
	if h.candidatesSaver != nil {
		if err := h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, purchase.ID, ""); err != nil {
			h.logger.Warn(ctx, "select match: failed to clear candidates",
				observability.String("purchaseID", purchase.ID), observability.Err(err))
		}
	}

	writeJSON(w, http.StatusOK, fixMatchResponse{
		Status:        "ok",
		DHCardID:      req.DHCardID,
		DHInventoryID: inventoryID,
	})
}
