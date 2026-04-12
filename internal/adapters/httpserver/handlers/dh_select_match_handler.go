package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
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
		if inventory.IsPurchaseNotFound(err) {
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

	marketValue := dhlisting.ResolveMarketValueCents(purchase)
	if marketValue == 0 {
		writeError(w, http.StatusBadRequest, "purchase has no market value yet")
		return
	}
	if purchase.BuyCostCents <= 0 {
		writeError(w, http.StatusBadRequest, "purchase has no buy cost")
		return
	}

	// Push to DH inventory and persist fields
	inventoryID, pushErr := h.pushAndPersistDH(ctx, purchase, req.DHCardID, marketValue)
	if pushErr != nil {
		switch {
		case errors.Is(pushErr, errDHPersistFailed):
			h.logger.Error(ctx, "select match: failed to persist DH fields",
				observability.String("purchaseID", purchase.ID),
				observability.Err(pushErr))
			writeError(w, http.StatusInternalServerError, "DH push succeeded but failed to save local state")
		case errors.Is(pushErr, errDHPushNoInventoryID):
			writeError(w, http.StatusBadGateway, "DH push failed — no inventory ID returned")
		default:
			h.logger.Error(ctx, "select match: push inventory", observability.Err(pushErr))
			writeError(w, http.StatusBadGateway, "DH API error")
		}
		return
	}

	if h.pushStatusUpdater != nil {
		if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, inventory.DHPushStatusManual); err != nil {
			h.logger.Warn(ctx, "select match: failed to set manual status",
				observability.String("purchaseID", purchase.ID), observability.Err(err))
		} else if h.candidatesSaver != nil {
			if err := h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, purchase.ID, ""); err != nil {
				h.logger.Warn(ctx, "select match: failed to clear candidates",
					observability.String("purchaseID", purchase.ID), observability.Err(err))
			}
		}
	}

	// Teach DH the correct match so future lookups resolve automatically.
	if h.matchConfirmer != nil && purchase.CertNumber != "" {
		confirmReq := dh.ConfirmMatchRequest{
			CertNumber: purchase.CertNumber,
			DHCardID:   req.DHCardID,
			SetName:    purchase.SetName,
			CardName:   purchase.CardName,
		}
		if _, err := h.matchConfirmer.ConfirmMatch(ctx, confirmReq); err != nil {
			h.logger.Warn(ctx, "select match: failed to confirm match with DH",
				observability.String("purchaseID", purchase.ID),
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
		}
	}

	writeJSON(w, http.StatusOK, fixMatchResponse{
		Status:        "ok",
		DHCardID:      req.DHCardID,
		DHInventoryID: inventoryID,
	})
}
