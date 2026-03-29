package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleCampaignPNL handles GET /api/campaigns/{id}/pnl.
func (h *CampaignsHandler) HandleCampaignPNL(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	pnl, err := h.service.GetCampaignPNL(r.Context(), id)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get campaign PNL", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, pnl)
}

// HandlePNLByChannel handles GET /api/campaigns/{id}/pnl-by-channel.
func (h *CampaignsHandler) HandlePNLByChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	channels, err := h.service.GetPNLByChannel(r.Context(), id)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get PNL by channel", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if channels == nil {
		channels = []campaigns.ChannelPNL{}
	}
	writeJSON(w, http.StatusOK, channels)
}

// HandleFillRate handles GET /api/campaigns/{id}/fill-rate.
func (h *CampaignsHandler) HandleFillRate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	days := 30
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 && v <= 365 {
		days = v
	}
	daily, err := h.service.GetDailySpend(r.Context(), id, days)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get fill rate", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	// Enrich with campaign's daily cap
	campaign, capErr := h.service.GetCampaign(r.Context(), id)
	if capErr != nil {
		h.logger.Debug(r.Context(), "failed to get campaign for fill rate enrichment",
			observability.String("campaign_id", id),
			observability.Err(capErr))
	}
	if capErr == nil && daily != nil {
		for i := range daily {
			daily[i].CapCents = campaign.DailySpendCapCents
			if daily[i].CapCents > 0 {
				daily[i].FillRatePct = float64(daily[i].SpendCents) / float64(daily[i].CapCents)
			}
		}
	}
	if daily == nil {
		daily = []campaigns.DailySpend{}
	}
	writeJSON(w, http.StatusOK, daily)
}

// HandleDaysToSell handles GET /api/campaigns/{id}/days-to-sell.
func (h *CampaignsHandler) HandleDaysToSell(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	buckets, err := h.service.GetDaysToSellDistribution(r.Context(), id)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get days-to-sell", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if buckets == nil {
		buckets = []campaigns.DaysToSellBucket{}
	}
	writeJSON(w, http.StatusOK, buckets)
}

// HandleInventory handles GET /api/campaigns/{id}/inventory.
func (h *CampaignsHandler) HandleInventory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	items, err := h.service.GetInventoryAging(r.Context(), id)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get inventory", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if items == nil {
		items = []campaigns.AgingItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// HandleSellSheet handles POST /api/campaigns/{id}/sell-sheet.
func (h *CampaignsHandler) HandleSellSheet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}

	var req struct {
		PurchaseIDs []string `json:"purchaseIds"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.PurchaseIDs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one purchase ID is required")
		return
	}

	sheet, err := h.service.GenerateSellSheet(r.Context(), id, req.PurchaseIDs)
	if err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "sell sheet generation failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, sheet)
}

// HandleGlobalInventory handles GET /api/inventory.
func (h *CampaignsHandler) HandleGlobalInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	items, err := h.service.GetGlobalInventoryAging(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get global inventory", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if items == nil {
		items = []campaigns.AgingItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// HandleGlobalSellSheet handles POST /api/sell-sheet.
func (h *CampaignsHandler) HandleGlobalSellSheet(w http.ResponseWriter, r *http.Request) {
	sheet, err := h.service.GenerateGlobalSellSheet(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "global sell sheet generation failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, sheet)
}

// HandleSelectedSellSheet handles POST /api/portfolio/sell-sheet.
func (h *CampaignsHandler) HandleSelectedSellSheet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PurchaseIDs []string `json:"purchaseIds"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.PurchaseIDs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one purchase ID is required")
		return
	}

	sheet, err := h.service.GenerateSelectedSellSheet(r.Context(), req.PurchaseIDs)
	if err != nil {
		h.logger.Error(r.Context(), "selected sell sheet generation failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, sheet)
}

// HandleTuning handles GET /api/campaigns/{id}/tuning.
func (h *CampaignsHandler) HandleTuning(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	tuning, err := h.service.GetCampaignTuning(r.Context(), id)
	if err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get tuning data", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, tuning)
}

// HandleCrackCandidates handles GET /api/campaigns/{id}/crack-candidates.
func (h *CampaignsHandler) HandleCrackCandidates(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	candidates, err := h.service.GetCrackCandidates(r.Context(), id)
	if err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get crack candidates", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if candidates == nil {
		candidates = []campaigns.CrackAnalysis{}
	}
	writeJSON(w, http.StatusOK, candidates)
}

// HandleExpectedValues handles GET /api/campaigns/{id}/expected-values.
func (h *CampaignsHandler) HandleExpectedValues(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	portfolio, err := h.service.GetExpectedValues(r.Context(), id)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get expected values", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, portfolio)
}

// HandleEvaluatePurchase handles POST /api/campaigns/{id}/evaluate-purchase.
func (h *CampaignsHandler) HandleEvaluatePurchase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	var req struct {
		CardName     string  `json:"cardName"`
		Grade        float64 `json:"grade"`
		BuyCostCents int     `json:"buyCostCents"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	req.CardName = strings.TrimSpace(req.CardName)
	if req.CardName == "" {
		writeError(w, http.StatusBadRequest, "cardName is required")
		return
	}
	if req.Grade < 1 || req.Grade > 10 {
		writeError(w, http.StatusBadRequest, "grade is invalid")
		return
	}
	if req.BuyCostCents < 0 {
		writeError(w, http.StatusBadRequest, "buyCostCents is invalid")
		return
	}
	ev, err := h.service.EvaluatePurchase(r.Context(), id, req.CardName, req.Grade, req.BuyCostCents)
	if err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to evaluate purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// HandleActivationChecklist handles GET /api/campaigns/{id}/activation-checklist.
func (h *CampaignsHandler) HandleActivationChecklist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	checklist, err := h.service.GetActivationChecklist(r.Context(), id)
	if err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get activation checklist", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, checklist)
}

// HandleProjections handles GET /api/campaigns/{id}/projections.
func (h *CampaignsHandler) HandleProjections(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	result, err := h.service.RunProjection(r.Context(), id)
	if err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to run projection", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// HandleSetReviewedPrice handles PATCH /api/purchases/{purchaseId}/review-price.
func (h *CampaignsHandler) HandleSetReviewedPrice(w http.ResponseWriter, r *http.Request) {
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
	if err := h.service.SetReviewedPrice(r.Context(), purchaseID, req.PriceCents, req.Source); err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		if campaigns.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error(r.Context(), "failed to set reviewed price", observability.Err(err),
			observability.String("purchase_id", purchaseID), observability.String("source", req.Source))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"reviewedAt": time.Now().Format(time.RFC3339),
	})
}

// HandleCreatePriceFlag handles POST /api/purchases/{purchaseId}/flag.
func (h *CampaignsHandler) HandleCreatePriceFlag(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}
	user := requireUser(w, r)
	if user == nil {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	flagID, err := h.service.CreatePriceFlag(r.Context(), purchaseID, user.ID, req.Reason)
	if err != nil {
		if campaigns.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		if campaigns.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error(r.Context(), "failed to create price flag", observability.Err(err),
			observability.String("purchase_id", purchaseID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        flagID,
		"flaggedAt": time.Now().Format(time.RFC3339),
	})
}

// HandleShopifyPriceSync handles POST /api/shopify/price-sync.
func (h *CampaignsHandler) HandleShopifyPriceSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []campaigns.ShopifyPriceSyncItem `json:"items"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "At least one item is required")
		return
	}
	const maxShopifyPriceSyncItems = 5000
	if len(req.Items) > maxShopifyPriceSyncItems {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Too many items (max %d)", maxShopifyPriceSyncItems))
		return
	}
	resp, err := h.service.MatchShopifyPrices(r.Context(), req.Items)
	if err != nil {
		h.logger.Error(r.Context(), "shopify price sync failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
