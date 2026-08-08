package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/domain/tuning"
)

// RowProvider fetches PSA export rows from the portal for manual import.
type RowProvider interface {
	FetchRows(ctx context.Context) ([]csvimport.PSAExportRow, error)
}

// DHPriceSyncer queues a DH price-sync for one purchase. Fire-and-forget
// from the handler's perspective — errors are logged by the service.
type DHPriceSyncer interface {
	SyncPurchasePrice(ctx context.Context, purchaseID string)
}

// CampaignsHandler handles campaign-related HTTP requests.
type CampaignsHandler struct {
	service        inventory.Service
	importSvc      csvimport.Service // optional: CSV/portal intake
	arbSvc         arbitrage.Service
	portSvc        portfolio.Service
	tuningSvc      tuning.Service
	logger         observability.Logger
	dhListingSvc   dhlisting.Service // optional: lists cards on DH after cert import
	dhPriceSyncer  DHPriceSyncer     // optional: async DH price re-sync on SetReviewedPrice
	financeService finance.Service   // optional: finance operations
	exportService  export.Service    // optional: sell sheet and eBay export
	baseCtx        context.Context
	bgWG           sync.WaitGroup // tracks background goroutines (e.g. DH listing)
	rowProvider    RowProvider    // optional: fetches PSA data from the portal

	psaSnapshots psacampaign.SnapshotStore  // optional: PSA portal campaign snapshot reader
	psaQueue     psacampaign.PushQueueStore // optional: PSA propose/publish push queue
	psaCatalog   psacampaign.CatalogStore   // optional: PSA spec-list/subject catalog reader
}

// CampaignsHandlerOption configures optional dependencies on CampaignsHandler.
type CampaignsHandlerOption func(*CampaignsHandler)

// WithImportService enables the CSV and PSA-portal intake endpoints. Without it
// those routes answer 503; every other campaign route is unaffected.
func WithImportService(svc csvimport.Service) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.importSvc = svc }
}

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

// WithPSARowProvider enables PSA portal sync for manual import.
func WithPSARowProvider(p RowProvider) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.rowProvider = p }
}

// WithPSASnapshotStore enables the PSA portal campaign snapshot reader.
func WithPSASnapshotStore(s psacampaign.SnapshotStore) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.psaSnapshots = s }
}

// WithPSAPushQueue enables the PSA propose/publish push queue.
func WithPSAPushQueue(q psacampaign.PushQueueStore) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.psaQueue = q }
}

// WithPSACatalogStore enables the PSA spec-list/subject catalog reader, which
// the translators need (via a Resolver) to push list-valued targeting.
func WithPSACatalogStore(s psacampaign.CatalogStore) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.psaCatalog = s }
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
//
// An optional ifUnmodifiedSince query parameter (RFC3339, as served in the
// campaign's own updatedAt) turns the write into a conditional one: the row is
// written only if it has not changed since the caller read it, and a mismatch
// answers 409 rather than silently overwriting. Callers that do a
// read-modify-write — the edit form and the bulk-paste importer — send it.
// Omitting it keeps the historic unconditional behaviour.
func (h *CampaignsHandler) HandleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "Campaign ID")
	if !ok {
		return
	}
	var expectedUpdatedAt *time.Time
	if raw := r.URL.Query().Get("ifUnmodifiedSince"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ifUnmodifiedSince timestamp")
			return
		}
		expectedUpdatedAt = &parsed
	}
	var c inventory.Campaign
	if !decodeBody(w, r, &c) {
		return
	}
	c.ID = id

	var err error
	if expectedUpdatedAt != nil {
		err = h.service.UpdateCampaignIfUnchanged(r.Context(), &c, *expectedUpdatedAt)
	} else {
		err = h.service.UpdateCampaign(r.Context(), &c)
	}
	if err != nil {
		if inventory.IsCampaignNotFound(err) {
			writeError(w, http.StatusNotFound, "Campaign not found")
			return
		}
		if inventory.IsCampaignConflict(err) {
			writeError(w, http.StatusConflict, "Campaign changed since it was loaded")
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
