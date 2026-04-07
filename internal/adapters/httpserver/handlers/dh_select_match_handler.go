package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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
	req.PurchaseID = strings.TrimSpace(req.PurchaseID)
	if req.PurchaseID == "" {
		writeError(w, http.StatusBadRequest, "purchaseId is required")
		return
	}
	if req.DHCardID <= 0 {
		writeError(w, http.StatusBadRequest, "dhCardId must be a positive integer")
		return
	}

	// Fail early if required dependencies are not configured.
	if h.inventoryPusher == nil {
		writeError(w, http.StatusInternalServerError, "inventory pusher not configured")
		return
	}
	if h.dhFieldsUpdater == nil {
		writeError(w, http.StatusInternalServerError, "DH fields updater not configured")
		return
	}

	// Serialize per purchase to prevent duplicate DH pushes from concurrent requests.
	mu := h.selectMatchLock(req.PurchaseID)
	mu.Lock()
	defer mu.Unlock()

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

	// Short-circuit if this purchase is already linked to DH.
	if purchase.DHInventoryID != 0 {
		writeJSON(w, http.StatusOK, fixMatchResponse{
			Status:        "ok",
			DHCardID:      purchase.DHCardID,
			DHInventoryID: purchase.DHInventoryID,
		})
		return
	}

	// Validate that the chosen card ID is among the purchase's candidates.
	if purchase.DHCandidatesJSON != "" {
		var candidates []dh.CertResolutionCandidate
		if err := json.Unmarshal([]byte(purchase.DHCandidatesJSON), &candidates); err != nil {
			writeError(w, http.StatusBadRequest, "malformed candidates data")
			return
		}
		found := false
		for _, c := range candidates {
			if c.DHCardID == req.DHCardID {
				found = true
				break
			}
		}
		if !found {
			writeError(w, http.StatusBadRequest, "dhCardId is not among the purchase's candidates")
			return
		}
	}

	externalID := strconv.Itoa(req.DHCardID)
	if err := h.cardIDSaver.SaveExternalID(ctx, purchase.CardName, purchase.SetName, purchase.CardNumber, pricing.SourceDH, externalID); err != nil {
		h.logger.Error(ctx, "select match: save external ID", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save card mapping")
		return
	}

	item := dh.InventoryItem{
		DHCardID:         req.DHCardID,
		CertNumber:       purchase.CertNumber,
		GradingCompany:   dh.GraderPSA,
		Grade:            purchase.GradeValue,
		CostBasisCents:   purchase.CLValueCents,
		MarketValueCents: dh.IntPtr(purchase.CLValueCents),
		Status:           dh.InventoryStatusInStock,
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
			if err := h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, purchase.ID, campaigns.DHFieldsUpdate{
				CardID:            req.DHCardID,
				InventoryID:       result.DHInventoryID,
				CertStatus:        dh.CertStatusMatched,
				ListingPriceCents: result.AssignedPriceCents,
				ChannelsJSON:      dh.MarshalChannels(result.Channels),
				DHStatus:          campaigns.DHStatus(result.Status),
			}); err != nil {
				h.logger.Error(ctx, "select match: failed to persist DH fields",
					observability.String("purchaseID", purchase.ID), observability.Err(err))
				writeError(w, http.StatusInternalServerError, "DH push succeeded but failed to save local state")
				return
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
		} else if h.candidatesSaver != nil {
			if err := h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, purchase.ID, ""); err != nil {
				h.logger.Warn(ctx, "select match: failed to clear candidates",
					observability.String("purchaseID", purchase.ID), observability.Err(err))
			}
		}
	}

	writeJSON(w, http.StatusOK, fixMatchResponse{
		Status:        "ok",
		DHCardID:      req.DHCardID,
		DHInventoryID: inventoryID,
	})
}
