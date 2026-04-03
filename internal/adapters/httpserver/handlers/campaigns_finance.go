package handlers

import (
	stderrors "errors"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleCreditSummary handles GET /api/credit/summary.
func (h *CampaignsHandler) HandleCreditSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.GetCreditSummary(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get credit summary", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// HandleGetCashflowConfig handles GET /api/credit/config.
func (h *CampaignsHandler) HandleGetCashflowConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.service.GetCashflowConfig(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get cashflow config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// HandleUpdateCashflowConfig handles PUT /api/credit/config.
func (h *CampaignsHandler) HandleUpdateCashflowConfig(w http.ResponseWriter, r *http.Request) {
	var cfg campaigns.CashflowConfig
	if !decodeBody(w, r, &cfg) {
		return
	}
	if err := h.service.UpdateCashflowConfig(r.Context(), &cfg); err != nil {
		h.logger.Error(r.Context(), "failed to update cashflow config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// HandleListInvoices handles GET /api/credit/invoices.
func (h *CampaignsHandler) HandleListInvoices(w http.ResponseWriter, r *http.Request) {
	invoices, err := h.service.ListInvoices(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to list invoices", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSONList(w, http.StatusOK, invoices)
}

// HandleUpdateInvoice handles PUT /api/credit/invoices.
func (h *CampaignsHandler) HandleUpdateInvoice(w http.ResponseWriter, r *http.Request) {
	var inv campaigns.Invoice
	if !decodeBody(w, r, &inv) {
		return
	}
	if err := h.service.UpdateInvoice(r.Context(), &inv); err != nil {
		if stderrors.Is(err, campaigns.ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "Invoice not found")
			return
		}
		h.logger.Error(r.Context(), "failed to update invoice", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// HandlePortfolioHealth handles GET /api/portfolio/health.
func (h *CampaignsHandler) HandlePortfolioHealth(w http.ResponseWriter, r *http.Request) {
	health, err := h.service.GetPortfolioHealth(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get portfolio health", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, health)
}

// HandlePortfolioChannelVelocity handles GET /api/portfolio/channel-velocity.
func (h *CampaignsHandler) HandlePortfolioChannelVelocity(w http.ResponseWriter, r *http.Request) {
	velocity, err := h.service.GetPortfolioChannelVelocity(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get channel velocity", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, velocity)
}

// HandlePortfolioInsights handles GET /api/portfolio/insights.
func (h *CampaignsHandler) HandlePortfolioInsights(w http.ResponseWriter, r *http.Request) {
	insights, err := h.service.GetPortfolioInsights(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get portfolio insights", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, insights)
}

// HandleCampaignSuggestions handles GET /api/portfolio/suggestions.
func (h *CampaignsHandler) HandleCampaignSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := h.service.GetCampaignSuggestions(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get campaign suggestions", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// HandleCapitalTimeline handles GET /api/portfolio/capital-timeline.
func (h *CampaignsHandler) HandleCapitalTimeline(w http.ResponseWriter, r *http.Request) {
	timeline, err := h.service.GetCapitalTimeline(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get capital timeline", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

// HandleWeeklyReview handles GET /api/portfolio/weekly-review.
func (h *CampaignsHandler) HandleWeeklyReview(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.GetWeeklyReviewSummary(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get weekly review", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// HandleListRevocationFlags handles GET /api/portfolio/revocations.
func (h *CampaignsHandler) HandleListRevocationFlags(w http.ResponseWriter, r *http.Request) {
	flags, err := h.service.ListRevocationFlags(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to list revocation flags", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSONList(w, http.StatusOK, flags)
}

// HandleCreateRevocationFlag handles POST /api/portfolio/revocations.
func (h *CampaignsHandler) HandleCreateRevocationFlag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SegmentLabel     string `json:"segmentLabel"`
		SegmentDimension string `json:"segmentDimension"`
		Reason           string `json:"reason"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	flag, err := h.service.FlagForRevocation(r.Context(), req.SegmentLabel, req.SegmentDimension, req.Reason)
	if err != nil {
		// Check if it's the "too soon" error
		var appErr *errors.AppError
		if stderrors.As(err, &appErr) && appErr.Code == campaigns.ErrCodeRevocationTooSoon {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		h.logger.Error(r.Context(), "failed to create revocation flag", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, flag)
}

// HandleRevocationEmail handles GET /api/portfolio/revocations/{flagId}/email.
func (h *CampaignsHandler) HandleRevocationEmail(w http.ResponseWriter, r *http.Request) {
	flagID, ok := pathID(w, r, "flagId", "Flag ID")
	if !ok {
		return
	}

	email, err := h.service.GenerateRevocationEmail(r.Context(), flagID)
	if err != nil {
		h.logger.Error(r.Context(), "failed to generate revocation email", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"emailText": email})
}
