package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleListPurchases handles GET /api/campaigns/{id}/purchases.
func (h *CampaignsHandler) HandleListPurchases(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	list, ok := serviceCall(w, r.Context(), h.logger, "failed to list purchases", func() ([]campaigns.Purchase, error) {
		return h.service.ListPurchasesByCampaign(r.Context(), id, limit, offset)
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, list)
}

// HandleCreatePurchase handles POST /api/campaigns/{id}/purchases.
func (h *CampaignsHandler) HandleCreatePurchase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	var p campaigns.Purchase
	if !decodeBody(w, r, &p) {
		return
	}
	p.CampaignID = id

	if err := h.service.CreatePurchase(r.Context(), &p); err != nil {
		if campaigns.IsDuplicateCertNumber(err) {
			writeError(w, http.StatusConflict, "Certificate number already exists")
			return
		}
		if campaigns.IsValidationError(err) || campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusBadRequest, "invalid purchase data")
			return
		}
		h.logger.Error(r.Context(), "failed to create purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// HandleListSales handles GET /api/campaigns/{id}/sales.
func (h *CampaignsHandler) HandleListSales(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	limit, offset := parsePagination(r)
	list, ok := serviceCall(w, r.Context(), h.logger, "failed to list sales", func() ([]campaigns.Sale, error) {
		return h.service.ListSalesByCampaign(r.Context(), id, limit, offset)
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, list)
}

// HandleCreateSale handles POST /api/campaigns/{id}/sales.
func (h *CampaignsHandler) HandleCreateSale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}

	var sale campaigns.Sale
	if !decodeBody(w, r, &sale) {
		return
	}
	s := &sale

	// Look up the purchase and campaign for profit computation
	purchase, err := h.service.GetPurchase(r.Context(), s.PurchaseID)
	if err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
		} else {
			h.logger.Error(r.Context(), "HandleCreateSale: GetPurchase failed",
				observability.String("purchaseID", s.PurchaseID),
				observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}
	if purchase.CampaignID != id {
		writeError(w, http.StatusBadRequest, "Purchase does not belong to this campaign")
		return
	}

	campaign, err := h.service.GetCampaign(r.Context(), id)
	if err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
		} else {
			h.logger.Error(r.Context(), "HandleCreateSale: GetCampaign failed",
				observability.String("campaignID", id),
				observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	if err := h.service.CreateSale(r.Context(), s, campaign, purchase); err != nil {
		if campaigns.IsDuplicateSale(err) {
			writeError(w, http.StatusConflict, "Sale already exists for this purchase")
			return
		}
		if campaigns.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "invalid sale data")
			return
		}
		h.logger.Error(r.Context(), "failed to create sale", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, s)
}

// HandleBulkSales creates multiple sales in one request.
// POST /api/campaigns/{id}/sales/bulk
func (h *CampaignsHandler) HandleBulkSales(w http.ResponseWriter, r *http.Request) {
	campaignID, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}

	var req struct {
		SaleChannel campaigns.SaleChannel     `json:"saleChannel"`
		SaleDate    string                    `json:"saleDate"`
		Items       []campaigns.BulkSaleInput `json:"items"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "No items provided")
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, "failed to create bulk sales", func() (*campaigns.BulkSaleResult, error) {
		return h.service.CreateBulkSales(r.Context(), campaignID, req.SaleChannel, req.SaleDate, req.Items)
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, result)
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
	purchase, err := h.service.GetPurchase(r.Context(), purchaseID)
	if err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if purchase.CampaignID != campaignID {
		writeError(w, http.StatusForbidden, "Purchase does not belong to this campaign")
		return
	}

	if err := h.service.DeletePurchase(r.Context(), purchaseID); err != nil {
		if campaigns.IsPurchaseNotFound(err) {
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
	purchase, err := h.service.GetPurchase(r.Context(), purchaseID)
	if err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if purchase.CampaignID != campaignID {
		writeError(w, http.StatusForbidden, "Purchase does not belong to this campaign")
		return
	}

	if err := h.service.DeleteSaleByPurchaseID(r.Context(), purchaseID); err != nil {
		if campaigns.IsSaleNotFound(err) {
			writeError(w, http.StatusNotFound, "No sale found for this purchase")
			return
		}
		h.logger.Error(r.Context(), "failed to delete sale", observability.Err(err), observability.String("purchase_id", purchaseID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleCertLookup handles GET /api/certs/{certNumber}.
func (h *CampaignsHandler) HandleCertLookup(w http.ResponseWriter, r *http.Request) {
	certNumber, ok := pathID(w, r, "certNumber", "Cert number")
	if !ok {
		return
	}
	info, snapshot, err := h.service.LookupCert(r.Context(), certNumber)
	if err != nil {
		h.logger.Error(r.Context(), "cert lookup failed", observability.Err(err), observability.String("cert", certNumber))
		writeError(w, http.StatusNotFound, "cert lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cert":   info,
		"market": snapshot,
	})
}

// HandleQuickAdd handles POST /api/campaigns/{id}/purchases/quick-add.
func (h *CampaignsHandler) HandleQuickAdd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}

	var req campaigns.QuickAddRequest
	if !decodeBody(w, r, &req) {
		return
	}

	purchase, err := h.service.QuickAddPurchase(r.Context(), id, req)
	if err != nil {
		if campaigns.IsDuplicateCertNumber(err) {
			writeError(w, http.StatusConflict, "Certificate number already exists")
			return
		}
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "quick-add failed", observability.Err(err))
		writeError(w, http.StatusBadRequest, "quick-add failed")
		return
	}
	writeJSON(w, http.StatusCreated, purchase)
}

// HandlePriceOverrideStats handles GET /api/admin/price-override-stats.
func (h *CampaignsHandler) HandlePriceOverrideStats(w http.ResponseWriter, r *http.Request) {
	stats, ok := serviceCall(w, r.Context(), h.logger, "failed to get price override stats", func() (*campaigns.PriceOverrideStats, error) {
		return h.service.GetPriceOverrideStats(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// HandleSetPriceOverride handles PATCH /api/purchases/{purchaseId}/price-override.
func (h *CampaignsHandler) HandleSetPriceOverride(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	var req struct {
		PriceCents int    `json:"priceCents"`
		Source     string `json:"source"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	if err := h.service.SetPriceOverride(r.Context(), purchaseID, req.PriceCents, req.Source); err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		if campaigns.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error(r.Context(), "failed to set price override", observability.Err(err), observability.String("purchase_id", purchaseID), observability.String("source", req.Source))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleClearPriceOverride handles DELETE /api/purchases/{purchaseId}/price-override.
func (h *CampaignsHandler) HandleClearPriceOverride(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	if err := h.service.SetPriceOverride(r.Context(), purchaseID, 0, ""); err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		h.logger.Error(r.Context(), "failed to clear price override", observability.Err(err), observability.String("purchase_id", purchaseID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleAcceptAISuggestion handles POST /api/purchases/{purchaseId}/accept-ai-suggestion.
func (h *CampaignsHandler) HandleAcceptAISuggestion(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	if err := h.service.AcceptAISuggestion(r.Context(), purchaseID); err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		if campaigns.IsNoAISuggestion(err) {
			writeError(w, http.StatusConflict, "AI suggestion is no longer available")
			return
		}
		if campaigns.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error(r.Context(), "failed to accept AI suggestion", observability.Err(err), observability.String("purchase_id", purchaseID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleDismissAISuggestion handles DELETE /api/purchases/{purchaseId}/ai-suggestion.
func (h *CampaignsHandler) HandleDismissAISuggestion(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	if err := h.service.DismissAISuggestion(r.Context(), purchaseID); err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		h.logger.Error(r.Context(), "failed to dismiss AI suggestion", observability.Err(err), observability.String("purchase_id", purchaseID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleUpdateBuyCost handles PATCH /api/purchases/{purchaseId}/buy-cost.
func (h *CampaignsHandler) HandleUpdateBuyCost(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	var req struct {
		BuyCostCents int `json:"buyCostCents"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	if err := h.service.UpdateBuyCost(r.Context(), purchaseID, req.BuyCostCents); err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		if campaigns.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error(r.Context(), "failed to update buy cost", observability.Err(err), observability.String("purchase_id", purchaseID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleReassignPurchase handles PATCH /api/purchases/{purchaseId}/campaign.
func (h *CampaignsHandler) HandleReassignPurchase(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}

	var req struct {
		CampaignID string `json:"campaignId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CampaignID == "" {
		writeError(w, http.StatusBadRequest, "campaignId is required")
		return
	}

	if err := h.service.ReassignPurchase(r.Context(), purchaseID, req.CampaignID); err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "reassign purchase failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
