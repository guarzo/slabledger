package handlers

import (
	"context"
	"net/http"
	"sync"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHInventoryLister transitions DH inventory items to listed and syncs channels.
type DHInventoryLister interface {
	UpdateInventory(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
	SyncChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
}

// CampaignsHandler handles campaign-related HTTP requests.
type CampaignsHandler struct {
	service           campaigns.Service
	logger            observability.Logger
	dhLister          DHInventoryLister   // optional: lists cards on DH after cert import
	dhCertResolver    DHCertResolver      // optional: resolves certs against DH
	dhPusher          DHInventoryPusher   // optional: pushes inventory to DH
	dhFieldsUpdater   DHFieldsUpdater     // optional: persists DH fields after push
	pushStatusUpdater DHPushStatusUpdater // optional: sets dh_push_status
	dhCardIDSaver     DHCardIDSaver       // optional: persists DH card ID mappings
	dhCandidatesSaver DHCandidatesSaver   // optional: stores ambiguous candidates
	baseCtx           context.Context
	bgWG              sync.WaitGroup // tracks background goroutines (e.g. DH listing)
}

// CampaignsHandlerOption configures optional dependencies on CampaignsHandler.
type CampaignsHandlerOption func(*CampaignsHandler)

// WithDHLister enables DH listing after cert import.
func WithDHLister(l DHInventoryLister) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhLister = l }
}

// WithDHCertResolver enables DH cert resolution for inline push.
func WithDHCertResolver(c DHCertResolver) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhCertResolver = c }
}

// WithDHPusher enables inventory push to DH.
func WithDHPusher(p DHInventoryPusher) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhPusher = p }
}

// WithDHFieldsUpdater enables persisting DH fields after push.
func WithDHFieldsUpdater(u DHFieldsUpdater) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhFieldsUpdater = u }
}

// WithDHPushStatusUpdater enables setting dh_push_status.
func WithDHPushStatusUpdater(u DHPushStatusUpdater) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.pushStatusUpdater = u }
}

// WithDHCardIDSaver enables persisting DH card ID mappings.
func WithDHCardIDSaver(s DHCardIDSaver) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhCardIDSaver = s }
}

// WithDHCandidatesSaver enables storing ambiguous DH candidates on purchases.
func WithDHCandidatesSaver(s DHCandidatesSaver) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhCandidatesSaver = s }
}

// NewCampaignsHandler creates a new campaigns handler.
// baseCtx is a server-lifecycle context; background goroutines derive from it
// so they are cancelled on shutdown. If nil, context.Background() is used.
func NewCampaignsHandler(
	service campaigns.Service,
	logger observability.Logger,
	baseCtx context.Context,
	opts ...CampaignsHandlerOption,
) *CampaignsHandler {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	h := &CampaignsHandler{
		service: service,
		logger:  logger,
		baseCtx: baseCtx,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// WaitBackground blocks until all background goroutines (e.g. card discovery) complete.
// Call this before closing the database to avoid use-after-close.
func (h *CampaignsHandler) WaitBackground() {
	h.bgWG.Wait()
}

// Compile-time checks.
var _ DHInventoryLister = (*dh.Client)(nil)

// HandleListCampaigns handles GET /api/campaigns.
func (h *CampaignsHandler) HandleListCampaigns(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("activeOnly") == "true"
	list, ok := serviceCall(w, r.Context(), h.logger, "failed to list campaigns", func() ([]campaigns.Campaign, error) {
		return h.service.ListCampaigns(r.Context(), activeOnly)
	})
	if !ok {
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
