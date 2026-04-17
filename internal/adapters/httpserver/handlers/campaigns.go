package handlers

import (
	"context"
	"net/http"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/tuning"
)

// SheetFetcher fetches sheet data for PSA sync.
type SheetFetcher interface {
	ReadSheet(ctx context.Context, spreadsheetID, sheetName string) ([][]string, error)
}

// DHPriceSyncer queues a DH price-sync for one purchase. Fire-and-forget
// from the handler's perspective — errors are logged by the service.
type DHPriceSyncer interface {
	SyncPurchasePrice(ctx context.Context, purchaseID string)
}

// CampaignsHandler handles campaign-related HTTP requests.
type CampaignsHandler struct {
	service           inventory.Service
	arbSvc            arbitrage.Service
	portSvc           portfolio.Service
	tuningSvc         tuning.Service
	logger            observability.Logger
	dhListingSvc      dhlisting.Service // optional: lists cards on DH after cert import
	dhPriceSyncer     DHPriceSyncer     // optional: async DH price re-sync on SetReviewedPrice
	financeService    finance.Service   // optional: finance operations
	exportService     export.Service    // optional: sell sheet and eBay export
	baseCtx           context.Context
	bgWG              sync.WaitGroup // tracks background goroutines (e.g. DH listing)
	sheetFetcher      SheetFetcher   // optional: fetches PSA data from Google Sheets
	sheetsSpreadsheet string         // spreadsheet ID for PSA sync
	sheetsTab         string         // tab name for PSA sync
}

// CampaignsHandlerOption configures optional dependencies on CampaignsHandler.
type CampaignsHandlerOption func(*CampaignsHandler)

// WithDHListingService enables DH listing after cert import.
func WithDHListingService(svc dhlisting.Service) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhListingSvc = svc }
}

// WithDHPriceSyncer enables async DH price re-sync on reviewed-price changes.
func WithDHPriceSyncer(syncer DHPriceSyncer) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhPriceSyncer = syncer }
}

// WithFinanceService enables finance operations on campaigns.
func WithFinanceService(svc finance.Service) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.financeService = svc }
}

// WithExportService enables sell sheet and eBay export operations.
func WithExportService(svc export.Service) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.exportService = svc }
}

// WithSheetFetcher enables Google Sheets PSA sync.
func WithSheetFetcher(f SheetFetcher, spreadsheetID, tabName string) CampaignsHandlerOption {
	return func(h *CampaignsHandler) {
		h.sheetFetcher = f
		h.sheetsSpreadsheet = spreadsheetID
		h.sheetsTab = tabName
	}
}

// NewCampaignsHandler creates a new campaigns handler.
// baseCtx is a server-lifecycle context; background goroutines derive from it
// so they are cancelled on shutdown. If nil, context.Background() is used.
func NewCampaignsHandler(
	service inventory.Service,
	arbSvc arbitrage.Service,
	portSvc portfolio.Service,
	tuningSvc tuning.Service,
	logger observability.Logger,
	baseCtx context.Context,
	opts ...CampaignsHandlerOption,
) *CampaignsHandler {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	h := &CampaignsHandler{
		service:   service,
		arbSvc:    arbSvc,
		portSvc:   portSvc,
		tuningSvc: tuningSvc,
		logger:    logger,
		baseCtx:   baseCtx,
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

// HandleListCampaigns handles GET /api/inventory.
func (h *CampaignsHandler) HandleListCampaigns(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("activeOnly") == "true"
	list, ok := serviceCall(w, r.Context(), h.logger, "failed to list campaigns", func() ([]inventory.Campaign, error) {
		return h.service.ListCampaigns(r.Context(), activeOnly)
	})
	if !ok {
		return
	}
	for i := range list {
		list[i].SetKind()
	}
	writeJSONList(w, http.StatusOK, list)
}

// HandleCreateCampaign handles POST /api/inventory.
func (h *CampaignsHandler) HandleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var c inventory.Campaign
	if !decodeBody(w, r, &c) {
		return
	}

	if err := h.service.CreateCampaign(r.Context(), &c); err != nil {
		if inventory.IsCampaignNotFound(err) || inventory.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "invalid campaign data")
			return
		}
		h.logger.Error(r.Context(), "failed to create campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	c.SetKind()
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
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	c.SetKind()
	writeJSON(w, http.StatusOK, c)
}

// HandleUpdateCampaign handles PUT /api/campaigns/{id}.
func (h *CampaignsHandler) HandleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	var c inventory.Campaign
	if !decodeBody(w, r, &c) {
		return
	}
	c.ID = id

	if err := h.service.UpdateCampaign(r.Context(), &c); err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		if inventory.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "invalid campaign data")
			return
		}
		h.logger.Error(r.Context(), "failed to update campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	c.SetKind()
	writeJSON(w, http.StatusOK, c)
}

// HandleDelete handles DELETE /api/campaigns/{id}.
func (h *CampaignsHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	if err := h.service.DeleteCampaign(r.Context(), id); err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		h.logger.Error(r.Context(), "failed to delete campaign", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
