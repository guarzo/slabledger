package handlers

import (
	stderrors "errors"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleCapitalSummary handles GET /api/credit/summary.
func (h *CampaignsHandler) HandleCapitalSummary(w http.ResponseWriter, r *http.Request) {
	if h.financeService == nil {
		writeError(w, http.StatusServiceUnavailable, "Finance service not available")
		return
	}

	summary, ok := serviceCall(w, r.Context(), h.logger, "failed to get capital summary", func() (*inventory.CapitalSummary, error) {
		return h.financeService.GetCapitalSummary(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// HandleGetCashflowConfig handles GET /api/credit/config.
func (h *CampaignsHandler) HandleGetCashflowConfig(w http.ResponseWriter, r *http.Request) {
	if h.financeService == nil {
		writeError(w, http.StatusServiceUnavailable, "Finance service not available")
		return
	}

	cfg, ok := serviceCall(w, r.Context(), h.logger, "failed to get cashflow config", func() (*inventory.CashflowConfig, error) {
		return h.financeService.GetCashflowConfig(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// HandleUpdateCashflowConfig handles PUT /api/credit/config.
func (h *CampaignsHandler) HandleUpdateCashflowConfig(w http.ResponseWriter, r *http.Request) {
	if h.financeService == nil {
		writeError(w, http.StatusServiceUnavailable, "Finance service not available")
		return
	}

	var req struct {
		CapitalBudgetCents int `json:"capitalBudgetCents"`
		CashBufferCents    int `json:"cashBufferCents"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	cfg := &inventory.CashflowConfig{
		CapitalBudgetCents: req.CapitalBudgetCents,
		CashBufferCents:    req.CashBufferCents,
	}
	if err := h.financeService.UpdateCashflowConfig(r.Context(), cfg); err != nil {
		if stderrors.Is(err, inventory.ErrInvalidCashflowConfig) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error(r.Context(), "failed to update cashflow config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// HandleListInvoices handles GET /api/credit/invoices.
func (h *CampaignsHandler) HandleListInvoices(w http.ResponseWriter, r *http.Request) {
	if h.financeService == nil {
		writeError(w, http.StatusServiceUnavailable, "Finance service not available")
		return
	}

	invoices, ok := serviceCall(w, r.Context(), h.logger, "failed to list invoices", func() ([]inventory.Invoice, error) {
		return h.financeService.ListInvoices(r.Context())
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, invoices)
}

// HandleUpdateInvoice handles PUT /api/credit/invoices.
func (h *CampaignsHandler) HandleUpdateInvoice(w http.ResponseWriter, r *http.Request) {
	if h.financeService == nil {
		writeError(w, http.StatusServiceUnavailable, "Finance service not available")
		return
	}

	var inv inventory.Invoice
	if !decodeBody(w, r, &inv) {
		return
	}
	if err := h.financeService.UpdateInvoice(r.Context(), &inv); err != nil {
		if stderrors.Is(err, inventory.ErrInvoiceNotFound) {
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
	health, ok := serviceCall(w, r.Context(), h.logger, "failed to get portfolio health", func() (*inventory.PortfolioHealth, error) {
		return h.portSvc.GetPortfolioHealth(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, health)
}

// HandlePortfolioChannelVelocity handles GET /api/portfolio/channel-velocity.
func (h *CampaignsHandler) HandlePortfolioChannelVelocity(w http.ResponseWriter, r *http.Request) {
	velocity, ok := serviceCall(w, r.Context(), h.logger, "failed to get channel velocity", func() ([]inventory.ChannelVelocity, error) {
		return h.portSvc.GetPortfolioChannelVelocity(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, velocity)
}

// HandlePortfolioInsights handles GET /api/portfolio/insights.
func (h *CampaignsHandler) HandlePortfolioInsights(w http.ResponseWriter, r *http.Request) {
	insights, ok := serviceCall(w, r.Context(), h.logger, "failed to get portfolio insights", func() (*inventory.PortfolioInsights, error) {
		return h.portSvc.GetPortfolioInsights(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, insights)
}

// HandleCampaignSuggestions handles GET /api/portfolio/suggestions.
func (h *CampaignsHandler) HandleCampaignSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, ok := serviceCall(w, r.Context(), h.logger, "failed to get campaign suggestions", func() (*inventory.SuggestionsResponse, error) {
		return h.portSvc.GetCampaignSuggestions(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// HandleCapitalTimeline handles GET /api/portfolio/capital-timeline.
func (h *CampaignsHandler) HandleCapitalTimeline(w http.ResponseWriter, r *http.Request) {
	timeline, ok := serviceCall(w, r.Context(), h.logger, "failed to get capital timeline", func() (*inventory.CapitalTimeline, error) {
		return h.portSvc.GetCapitalTimeline(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

// HandleWeeklyReview handles GET /api/portfolio/weekly-review.
func (h *CampaignsHandler) HandleWeeklyReview(w http.ResponseWriter, r *http.Request) {
	summary, ok := serviceCall(w, r.Context(), h.logger, "failed to get weekly review", func() (*inventory.WeeklyReviewSummary, error) {
		return h.portSvc.GetWeeklyReviewSummary(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// HandleWeeklyHistory handles GET /api/portfolio/weekly-history?weeks=N.
// Returns the N most recent weekly summaries in reverse chronological order.
// Defaults to 8 weeks if not specified. Maximum 52 weeks.
func (h *CampaignsHandler) HandleWeeklyHistory(w http.ResponseWriter, r *http.Request) {
	weeksStr := r.URL.Query().Get("weeks")
	weeks := 8
	if weeksStr != "" {
		n, err := strconv.Atoi(weeksStr)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "weeks must be a positive integer")
			return
		}
		if n > 52 {
			writeError(w, http.StatusBadRequest, "weeks must be at most 52")
			return
		}
		weeks = n
	}

	summaries, ok := serviceCall(w, r.Context(), h.logger, "failed to get weekly history", func() ([]inventory.WeeklyReviewSummary, error) {
		return h.portSvc.GetWeeklyHistory(r.Context(), weeks)
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, summaries)
}

// HandleListRevocationFlags handles GET /api/portfolio/revocations.
func (h *CampaignsHandler) HandleListRevocationFlags(w http.ResponseWriter, r *http.Request) {
	if h.financeService == nil {
		writeError(w, http.StatusServiceUnavailable, "Finance service not available")
		return
	}

	flags, ok := serviceCall(w, r.Context(), h.logger, "failed to list revocation flags", func() ([]inventory.RevocationFlag, error) {
		return h.financeService.ListRevocationFlags(r.Context())
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, flags)
}

// HandleCreateRevocationFlag handles POST /api/portfolio/revocations.
func (h *CampaignsHandler) HandleCreateRevocationFlag(w http.ResponseWriter, r *http.Request) {
	if h.financeService == nil {
		writeError(w, http.StatusServiceUnavailable, "Finance service not available")
		return
	}

	var req struct {
		SegmentLabel     string `json:"segmentLabel"`
		SegmentDimension string `json:"segmentDimension"`
		Reason           string `json:"reason"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	flag, err := h.financeService.FlagForRevocation(r.Context(), req.SegmentLabel, req.SegmentDimension, req.Reason)
	if err != nil {
		// Check if it's the "too soon" error
		var appErr *errors.AppError
		if stderrors.As(err, &appErr) && appErr.Code == inventory.ErrCodeRevocationTooSoon {
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
	if h.financeService == nil {
		writeError(w, http.StatusServiceUnavailable, "Finance service not available")
		return
	}

	flagID, ok := pathID(w, r, "flagId", "Flag ID")
	if !ok {
		return
	}

	email, ok := serviceCall(w, r.Context(), h.logger, "failed to generate revocation email", func() (string, error) {
		return h.financeService.GenerateRevocationEmail(r.Context(), flagID)
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"emailText": email})
}
