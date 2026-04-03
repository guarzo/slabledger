package handlers

import (
	"context"
	"net/http"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CampaignsHandler handles campaign-related HTTP requests.
type CampaignsHandler struct {
	service    campaigns.Service
	logger     observability.Logger
	discoverer CardDiscoverer // optional: triggers CardHedger discovery after imports
	baseCtx    context.Context
	bgWG       sync.WaitGroup // tracks background goroutines (e.g. card discovery)
}

// NewCampaignsHandler creates a new campaigns handler.
// baseCtx is a server-lifecycle context; background goroutines derive from it
// so they are cancelled on shutdown. If nil, context.Background() is used.
func NewCampaignsHandler(service campaigns.Service, logger observability.Logger, discoverer CardDiscoverer, baseCtx context.Context) *CampaignsHandler {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	return &CampaignsHandler{service: service, logger: logger, discoverer: discoverer, baseCtx: baseCtx}
}

// WaitBackground blocks until all background goroutines (e.g. card discovery) complete.
// Call this before closing the database to avoid use-after-close.
func (h *CampaignsHandler) WaitBackground() {
	h.bgWG.Wait()
}

// HandleListCampaigns handles GET /api/campaigns.
func (h *CampaignsHandler) HandleListCampaigns(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("activeOnly") == "true"
	list, err := h.service.ListCampaigns(r.Context(), activeOnly)
	if err != nil {
		h.logger.Error(r.Context(), "failed to list campaigns", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSONList(w, http.StatusOK, list)
}

// HandleCreateCampaign handles POST /api/campaigns.
func (h *CampaignsHandler) HandleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var c campaigns.Campaign
	if !decodeBody(w, r, &c) {
		return
	}

	if err := h.service.CreateCampaign(r.Context(), &c); err != nil {
		if campaigns.IsCampaignNotFound(err) || campaigns.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "invalid campaign data")
			return
		}
		h.logger.Error(r.Context(), "failed to create campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, c)
}

// HandleGetCampaign handles GET /api/campaigns/{id}.
func (h *CampaignsHandler) HandleGetCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	c, err := h.service.GetCampaign(r.Context(), id)
	if err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// HandleUpdateCampaign handles PUT /api/campaigns/{id}.
func (h *CampaignsHandler) HandleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	var c campaigns.Campaign
	if !decodeBody(w, r, &c) {
		return
	}
	c.ID = id

	if err := h.service.UpdateCampaign(r.Context(), &c); err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		if campaigns.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "invalid campaign data")
			return
		}
		h.logger.Error(r.Context(), "failed to update campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// HandleDelete handles DELETE /api/campaigns/{id}.
func (h *CampaignsHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	if err := h.service.DeleteCampaign(r.Context(), id); err != nil {
		if campaigns.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to delete campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
