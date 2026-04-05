package handlers

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

var dhURLPattern = regexp.MustCompile(`doubleholo\.com/card/(\d+)`)

type fixMatchRequest struct {
	PurchaseID string `json:"purchaseId"`
	DHURL      string `json:"dhUrl"`
}

type fixMatchResponse struct {
	Status        string `json:"status"`
	DHCardID      int    `json:"dhCardId"`
	DHInventoryID int    `json:"dhInventoryId"`
}

// HandleFixMatch lets users manually resolve an unmatched card by pasting a DH URL.
// It parses the DH card ID from the URL, saves the mapping, pushes to DH inventory,
// persists the DH fields, and marks the purchase status as "manual".
func (h *DHHandler) HandleFixMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	var req fixMatchRequest
	if !decodeBody(w, r, &req) {
		return
	}

	if req.PurchaseID == "" {
		writeError(w, http.StatusBadRequest, "purchaseId is required")
		return
	}
	if req.DHURL == "" {
		writeError(w, http.StatusBadRequest, "dhUrl is required")
		return
	}

	// Parse DH card ID from URL
	matches := dhURLPattern.FindStringSubmatch(req.DHURL)
	if len(matches) < 2 {
		writeError(w, http.StatusBadRequest, "invalid DH URL — expected format: doubleholo.com/card/{id}/...")
		return
	}
	dhCardID, err := strconv.Atoi(matches[1])
	if err != nil || dhCardID == 0 {
		writeError(w, http.StatusBadRequest, "invalid card ID in URL")
		return
	}

	// Find the purchase
	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.logger.Error(ctx, "fix match: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to look up purchase")
		return
	}

	var purchase *campaigns.Purchase
	for i := range purchases {
		if purchases[i].ID == req.PurchaseID {
			purchase = &purchases[i]
			break
		}
	}
	if purchase == nil {
		writeError(w, http.StatusNotFound, "purchase not found")
		return
	}

	// Save card ID mapping
	externalID := strconv.Itoa(dhCardID)
	if err := h.cardIDSaver.SaveExternalID(ctx, purchase.CardName, purchase.SetName, purchase.CardNumber, pricing.SourceDH, externalID); err != nil {
		h.logger.Error(ctx, "fix match: save external ID", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save card mapping")
		return
	}

	// Push to DH inventory
	item := dh.InventoryItem{
		DHCardID:       dhCardID,
		CertNumber:     purchase.CertNumber,
		GradingCompany: dh.GraderPSA,
		Grade:          purchase.GradeValue,
		CostBasisCents: purchase.CLValueCents,
		Status:         dh.InventoryStatusInStock,
	}

	pushResp, pushErr := h.inventoryPusher.PushInventory(ctx, []dh.InventoryItem{item})
	if pushErr != nil {
		h.logger.Error(ctx, "fix match: push inventory", observability.Err(pushErr))
		writeError(w, http.StatusBadGateway, "DH API error")
		return
	}

	var inventoryID int
	for _, result := range pushResp.Results {
		if result.Status != "failed" && result.DHInventoryID != 0 {
			inventoryID = result.DHInventoryID
			if h.dhFieldsUpdater != nil {
				_ = h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, purchase.ID, campaigns.DHFieldsUpdate{
					CardID:            dhCardID,
					InventoryID:       result.DHInventoryID,
					CertStatus:        dh.CertStatusMatched,
					ListingPriceCents: result.AssignedPriceCents,
					ChannelsJSON:      marshalChannels(result.Channels),
					DHStatus:          campaigns.DHStatus(result.Status),
				})
			}
			break
		}
	}

	if inventoryID == 0 {
		writeError(w, http.StatusBadGateway, "DH push failed — no inventory ID returned")
		return
	}

	// Set status to manual
	if h.pushStatusUpdater != nil {
		_ = h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, campaigns.DHPushStatusManual)
	}

	writeJSON(w, http.StatusOK, fixMatchResponse{
		Status:        "ok",
		DHCardID:      dhCardID,
		DHInventoryID: inventoryID,
	})
}
