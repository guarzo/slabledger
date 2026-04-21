package handlers

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
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

// HandleFixMatch lets users manually resolve a card by pasting a DH URL.
// Usable on unmatched purchases (first-time match) or on already-matched
// purchases whose current mapping is wrong (re-match to a different card).
//
// On a card swap (old dh_card_id != new dh_card_id and the push creates a
// new dh_inventory_id), the previous listing is stranded on DH under the
// wrong card. We best-effort DelistChannels on the old inventory ID after
// the new match is committed locally, so eBay/Shopify aren't left advertising
// the wrong card. DH has no delete-inventory endpoint today — the old row
// stays on DH's side; this only detaches its channels.
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

	// Find the purchase by ID
	purchase, err := h.purchaseLister.GetPurchase(ctx, req.PurchaseID)
	if err != nil {
		if inventory.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "purchase not found")
			return
		}
		h.logger.Error(ctx, "fix match: get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to look up purchase")
		return
	}

	// Capture prior DH state before UpdatePurchaseDHFields overwrites it.
	// Used later to take down channels on the stranded old listing when
	// the user is swapping to a different DH card.
	oldInventoryID := purchase.DHInventoryID
	oldCardID := purchase.DHCardID

	// Save card ID mapping
	externalID := strconv.Itoa(dhCardID)
	if err := h.cardIDSaver.SaveExternalID(ctx, purchase.CardName, purchase.SetName, purchase.CardNumber, pricing.SourceDH, externalID); err != nil {
		h.logger.Error(ctx, "fix match: save external ID", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save card mapping")
		return
	}

	// listing_price_cents is an optional preset on push (DH catalog fallback
	// when omitted). Don't gate the push on price.
	listingPrice := dhlisting.ResolveListingPriceCents(purchase)

	// Push to DH inventory and persist fields
	inventoryID, pushErr := h.pushAndPersistDH(ctx, purchase, dhCardID, listingPrice)
	if pushErr != nil {
		switch {
		case errors.Is(pushErr, errDHPersistFailed):
			h.logger.Error(ctx, "fix match: failed to persist DH fields",
				observability.String("purchaseID", purchase.ID),
				observability.Err(pushErr))
			writeError(w, http.StatusInternalServerError, "DH push succeeded but failed to save local state")
		case errors.Is(pushErr, errDHPushNoInventoryID):
			writeError(w, http.StatusBadGateway, "DH push failed — no inventory ID returned")
		default:
			h.logger.Error(ctx, "fix match: push inventory", observability.Err(pushErr))
			writeError(w, http.StatusBadGateway, "DH API error")
		}
		return
	}

	// Set status to manual
	if h.pushStatusUpdater != nil {
		if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, inventory.DHPushStatusManual); err != nil {
			h.logger.Warn(ctx, "fix match: failed to set manual status",
				observability.String("purchaseID", purchase.ID),
				observability.Err(err))
		}
	}

	// Clear stored candidates (if any) now that a manual match has been applied
	if h.candidatesSaver != nil {
		if err := h.candidatesSaver.UpdatePurchaseDHCandidates(ctx, purchase.ID, ""); err != nil {
			h.logger.Warn(ctx, "fix match: failed to clear candidates",
				observability.String("purchaseID", purchase.ID),
				observability.Err(err))
		}
	}

	// Teach DH the correct match so future lookups resolve automatically.
	if h.matchConfirmer != nil && purchase.CertNumber != "" {
		confirmReq := dh.ConfirmMatchRequest{
			CertNumber: purchase.CertNumber,
			DHCardID:   dhCardID,
			SetName:    purchase.SetName,
			CardName:   purchase.CardName,
		}
		if _, err := h.matchConfirmer.ConfirmMatch(ctx, confirmReq); err != nil {
			h.logger.Warn(ctx, "fix match: failed to confirm match with DH",
				observability.String("purchaseID", purchase.ID),
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
		}
	}

	// If this purchase was previously matched to a different DH card and the
	// push created a new inventory row, take down the channels on the old
	// listing so eBay/Shopify aren't still advertising the wrong card.
	// Best-effort: the local match is already committed.
	if h.channelDelister != nil &&
		oldInventoryID != 0 &&
		oldInventoryID != inventoryID &&
		oldCardID != dhCardID {
		if _, derr := h.channelDelister.DelistChannels(ctx, oldInventoryID, nil); derr != nil {
			h.logger.Warn(ctx, "fix match: delist old channels failed, continuing",
				observability.String("purchaseID", purchase.ID),
				observability.Int("oldDHInventoryID", oldInventoryID),
				observability.Int("oldDHCardID", oldCardID),
				observability.Int("newDHCardID", dhCardID),
				observability.Err(derr))
		}
	}

	writeJSON(w, http.StatusOK, fixMatchResponse{
		Status:        "ok",
		DHCardID:      dhCardID,
		DHInventoryID: inventoryID,
	})
}
