package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleCampaignPNL handles GET /api/campaigns/{id}/pnl.
func (h *CampaignsHandler) HandleCampaignPNL(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	pnl, ok := serviceCall(w, r.Context(), h.logger, "failed to get campaign PNL", func() (*inventory.CampaignPNL, error) {
		return h.service.GetCampaignPNL(r.Context(), id)
	})
	if !ok {
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
	channels, ok := serviceCall(w, r.Context(), h.logger, "failed to get PNL by channel", func() ([]inventory.ChannelPNL, error) {
		return h.service.GetPNLByChannel(r.Context(), id)
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, channels)
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
		if inventory.IsCampaignNotFound(capErr) {
			h.logger.Warn(r.Context(), "campaign not found for fill rate enrichment",
				observability.String("campaign_id", id))
		} else {
			h.logger.Error(r.Context(), "failed to get campaign for fill rate enrichment",
				observability.String("campaign_id", id),
				observability.Err(capErr))
		}
	}

	type fillRateRow struct {
		Date          string  `json:"date"`
		SpendUSD      float64 `json:"spendUSD"`
		CapUSD        float64 `json:"capUSD"`
		FillRatePct   float64 `json:"fillRatePct"`
		PurchaseCount int     `json:"purchaseCount"`
	}
	rows := make([]fillRateRow, len(daily))
	for i, d := range daily {
		capCents := d.CapCents
		if capErr == nil && campaign != nil {
			capCents = campaign.DailySpendCapCents
		}
		fillRate := d.FillRatePct
		if capCents > 0 {
			fillRate = float64(d.SpendCents) / float64(capCents)
		}
		rows[i] = fillRateRow{
			Date:          d.Date,
			SpendUSD:      float64(d.SpendCents) / 100.0,
			CapUSD:        float64(capCents) / 100.0,
			FillRatePct:   fillRate,
			PurchaseCount: d.PurchaseCount,
		}
	}
	writeJSONList(w, http.StatusOK, rows)
}

// HandleDaysToSell handles GET /api/campaigns/{id}/days-to-sell.
func (h *CampaignsHandler) HandleDaysToSell(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	buckets, ok := serviceCall(w, r.Context(), h.logger, "failed to get days-to-sell", func() ([]inventory.DaysToSellBucket, error) {
		return h.service.GetDaysToSellDistribution(r.Context(), id)
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, buckets)
}

// HandleInventory handles GET /api/campaigns/{id}/inventory.
func (h *CampaignsHandler) HandleInventory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	result, ok := serviceCall(w, r.Context(), h.logger, "failed to get inventory", func() (*inventory.InventoryResult, error) {
		return h.service.GetInventoryAging(r.Context(), id)
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// HandleSellSheet handles POST /api/campaigns/{id}/sell-sheet.
func (h *CampaignsHandler) HandleSellSheet(w http.ResponseWriter, r *http.Request) {
	if h.exportService == nil {
		writeError(w, http.StatusServiceUnavailable, "Export service not available")
		return
	}

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

	sheet, err := h.exportService.GenerateSellSheet(r.Context(), id, req.PurchaseIDs)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
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
	result, ok := serviceCall(w, r.Context(), h.logger, "failed to get global inventory", func() (*inventory.InventoryResult, error) {
		return h.service.GetGlobalInventoryAging(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// HandleGlobalSellSheet handles POST /api/sell-sheet.
func (h *CampaignsHandler) HandleGlobalSellSheet(w http.ResponseWriter, r *http.Request) {
	if h.exportService == nil {
		writeError(w, http.StatusServiceUnavailable, "Export service not available")
		return
	}

	sheet, ok := serviceCall(w, r.Context(), h.logger, "global sell sheet generation failed", func() (*inventory.SellSheet, error) {
		return h.exportService.GenerateGlobalSellSheet(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, sheet)
}

// HandleSelectedSellSheet handles POST /api/portfolio/sell-sheet.
func (h *CampaignsHandler) HandleSelectedSellSheet(w http.ResponseWriter, r *http.Request) {
	if h.exportService == nil {
		writeError(w, http.StatusServiceUnavailable, "Export service not available")
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
	const maxSelectedSellSheetItems = 5000
	if len(req.PurchaseIDs) > maxSelectedSellSheetItems {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Too many purchase IDs (max %d)", maxSelectedSellSheetItems))
		return
	}

	sheet, ok := serviceCall(w, r.Context(), h.logger, "selected sell sheet generation failed", func() (*inventory.SellSheet, error) {
		return h.exportService.GenerateSelectedSellSheet(r.Context(), req.PurchaseIDs)
	})
	if !ok {
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
	tuning, err := h.tuningSvc.GetCampaignTuning(r.Context(), id)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
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
	candidates, err := h.arbSvc.GetCrackCandidates(r.Context(), id)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get crack candidates", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSONList(w, http.StatusOK, candidates)
}

// HandleExpectedValues handles GET /api/campaigns/{id}/expected-values.
func (h *CampaignsHandler) HandleExpectedValues(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	portfolio, ok := serviceCall(w, r.Context(), h.logger, "failed to get expected values", func() (*arbitrage.EVPortfolio, error) {
		return h.arbSvc.GetExpectedValues(r.Context(), id)
	})
	if !ok {
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
	ev, err := h.arbSvc.EvaluatePurchase(r.Context(), id, req.CardName, req.Grade, req.BuyCostCents)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
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
	checklist, err := h.arbSvc.GetActivationChecklist(r.Context(), id)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
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
	result, err := h.arbSvc.RunProjection(r.Context(), id)
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to run projection", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Return 422 when there is insufficient data to run a meaningful simulation.
	// Callers (e.g. campaign-analysis skill) can branch on the status code rather
	// than parsing the body.
	if result != nil && result.Confidence == "insufficient" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":       "insufficient_data",
			"minRequired": 10,
			"available":   result.SampleSize,
		})
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
		if inventory.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		if inventory.IsValidationError(err) {
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
		if inventory.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "Purchase not found")
			return
		}
		if inventory.IsValidationError(err) {
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
