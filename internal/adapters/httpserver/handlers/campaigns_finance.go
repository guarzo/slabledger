package handlers

import (
	stderrors "errors"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleCapitalSummary handles GET /api/credit/summary.
func (h *CampaignsHandler) HandleCapitalSummary(w http.ResponseWriter, r *http.Request) {
	summary, ok := serviceCall(w, r.Context(), h.logger, "failed to get capital summary", func() (*campaigns.CapitalSummary, error) {
		return h.service.GetCapitalSummary(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// HandleGetCashflowConfig handles GET /api/credit/config.
func (h *CampaignsHandler) HandleGetCashflowConfig(w http.ResponseWriter, r *http.Request) {
	cfg, ok := serviceCall(w, r.Context(), h.logger, "failed to get cashflow config", func() (*campaigns.CashflowConfig, error) {
		return h.service.GetCashflowConfig(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// HandleUpdateCashflowConfig handles PUT /api/credit/config.
func (h *CampaignsHandler) HandleUpdateCashflowConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CapitalBudgetCents int `json:"capitalBudgetCents"`
		CashBufferCents    int `json:"cashBufferCents"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	cfg := &campaigns.CashflowConfig{
		CapitalBudgetCents: req.CapitalBudgetCents,
		CashBufferCents:    req.CashBufferCents,
	}
	if err := h.service.UpdateCashflowConfig(r.Context(), cfg); err != nil {
		if stderrors.Is(err, campaigns.ErrInvalidCashflowConfig) {
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
	invoices, ok := serviceCall(w, r.Context(), h.logger, "failed to list invoices", func() ([]campaigns.Invoice, error) {
		return h.service.ListInvoices(r.Context())
	})
	if !ok {
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
	health, ok := serviceCall(w, r.Context(), h.logger, "failed to get portfolio health", func() (*campaigns.PortfolioHealth, error) {
		return h.service.GetPortfolioHealth(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, health)
}

// HandlePortfolioChannelVelocity handles GET /api/portfolio/channel-velocity.
func (h *CampaignsHandler) HandlePortfolioChannelVelocity(w http.ResponseWriter, r *http.Request) {
	velocity, ok := serviceCall(w, r.Context(), h.logger, "failed to get channel velocity", func() ([]campaigns.ChannelVelocity, error) {
		return h.service.GetPortfolioChannelVelocity(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, velocity)
}

// HandlePortfolioInsights handles GET /api/portfolio/insights.
func (h *CampaignsHandler) HandlePortfolioInsights(w http.ResponseWriter, r *http.Request) {
	insights, ok := serviceCall(w, r.Context(), h.logger, "failed to get portfolio insights", func() (*campaigns.PortfolioInsights, error) {
		return h.service.GetPortfolioInsights(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, insights)
}

// HandleCampaignSuggestions handles GET /api/portfolio/suggestions.
func (h *CampaignsHandler) HandleCampaignSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, ok := serviceCall(w, r.Context(), h.logger, "failed to get campaign suggestions", func() (*campaigns.SuggestionsResponse, error) {
		return h.service.GetCampaignSuggestions(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// HandleCapitalTimeline handles GET /api/portfolio/capital-timeline.
func (h *CampaignsHandler) HandleCapitalTimeline(w http.ResponseWriter, r *http.Request) {
	timeline, ok := serviceCall(w, r.Context(), h.logger, "failed to get capital timeline", func() (*campaigns.CapitalTimeline, error) {
		return h.service.GetCapitalTimeline(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

// HandleWeeklyReview handles GET /api/portfolio/weekly-review.
func (h *CampaignsHandler) HandleWeeklyReview(w http.ResponseWriter, r *http.Request) {
	summary, ok := serviceCall(w, r.Context(), h.logger, "failed to get weekly review", func() (*campaigns.WeeklyReviewSummary, error) {
		return h.service.GetWeeklyReviewSummary(r.Context())
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// HandleListRevocationFlags handles GET /api/portfolio/revocations.
func (h *CampaignsHandler) HandleListRevocationFlags(w http.ResponseWriter, r *http.Request) {
	flags, ok := serviceCall(w, r.Context(), h.logger, "failed to list revocation flags", func() ([]campaigns.RevocationFlag, error) {
		return h.service.ListRevocationFlags(r.Context())
	})
	if !ok {
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

	email, ok := serviceCall(w, r.Context(), h.logger, "failed to generate revocation email", func() (string, error) {
		return h.service.GenerateRevocationEmail(r.Context(), flagID)
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"emailText": email})
}
