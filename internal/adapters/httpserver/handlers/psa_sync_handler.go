package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// PSASyncRefresher runs a PSA sync cycle on demand and provides last-run stats.
type PSASyncRefresher interface {
	RunOnce(ctx context.Context) error
	GetLastRunStats() *scheduler.PSASyncRunStats
}

// PSASyncPurchaseCreator creates purchases (subset of inventory.Service).
type PSASyncPurchaseCreator interface {
	CreatePurchase(ctx context.Context, p *inventory.Purchase) error
}

// PSASyncHandlerConfig holds dependencies for the PSA sync handler.
type PSASyncHandlerConfig struct {
	PendingRepo   inventory.PendingItemRepository
	Refresher     PSASyncRefresher       // optional
	Service       PSASyncPurchaseCreator // optional
	SpreadsheetID string
	Interval      string
	Logger        observability.Logger
}

// PSASyncHandler serves PSA sync status and pending-item CRUD endpoints.
type PSASyncHandler struct {
	pendingRepo   inventory.PendingItemRepository
	refresher     PSASyncRefresher
	service       PSASyncPurchaseCreator
	spreadsheetID string
	interval      string
	logger        observability.Logger
}

// NewPSASyncHandler creates a new PSASyncHandler from the given config.
func NewPSASyncHandler(cfg PSASyncHandlerConfig) *PSASyncHandler {
	return &PSASyncHandler{
		pendingRepo:   cfg.PendingRepo,
		refresher:     cfg.Refresher,
		service:       cfg.Service,
		spreadsheetID: cfg.SpreadsheetID,
		interval:      cfg.Interval,
		logger:        cfg.Logger,
	}
}

// HandleStatus returns the PSA sync configuration and last-run stats.
// GET /api/admin/psa-sync/status
func (h *PSASyncHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp := map[string]any{
		"configured":    h.spreadsheetID != "",
		"spreadsheetId": h.spreadsheetID,
		"interval":      h.interval,
	}

	if h.refresher != nil {
		if stats := h.refresher.GetLastRunStats(); stats != nil {
			resp["lastRun"] = stats
		}
	}

	if h.pendingRepo != nil {
		count, err := h.pendingRepo.CountPendingItems(ctx)
		if err != nil {
			h.logger.Warn(ctx, "failed to count pending items", observability.Err(err))
		} else {
			resp["pendingCount"] = count
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleRefresh triggers a manual PSA sync cycle.
// POST /api/admin/psa-sync/refresh
func (h *PSASyncHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if h.refresher == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA sync scheduler not available")
		return
	}
	if err := h.refresher.RunOnce(r.Context()); err != nil {
		if errors.Is(err, scheduler.ErrSyncInProgress) {
			writeError(w, http.StatusConflict, "PSA sync already in progress")
			return
		}
		h.logger.Error(r.Context(), "manual PSA sync failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "sync failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sync complete"})
}

// HandleListPendingItems returns all pending items awaiting user resolution.
// GET /api/admin/psa-sync/pending
func (h *PSASyncHandler) HandleListPendingItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items, ok := serviceCall(w, ctx, h.logger, "failed to list pending items", func() ([]inventory.PendingItem, error) {
		return h.pendingRepo.ListPendingItems(ctx)
	})
	if !ok {
		return
	}
	if items == nil {
		items = []inventory.PendingItem{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleAssignPendingItem assigns a pending item to a campaign by creating
// a purchase and resolving the pending item.
// POST /api/admin/psa-sync/pending/{id}/assign
func (h *PSASyncHandler) HandleAssignPendingItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := pathID(w, r, "id", "pending item ID")
	if !ok {
		return
	}

	var body struct {
		CampaignID string `json:"campaignId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.CampaignID == "" {
		writeError(w, http.StatusBadRequest, "campaignId required")
		return
	}

	// Find the pending item by ID.
	item, err := h.pendingRepo.GetPendingItemByID(ctx, id)
	if err != nil {
		if inventory.IsPendingItemNotFound(err) {
			writeError(w, http.StatusNotFound, "pending item not found")
			return
		}
		h.logger.Error(ctx, "failed to get pending item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	purchase := &inventory.Purchase{
		CampaignID:   body.CampaignID,
		CertNumber:   item.CertNumber,
		CardName:     item.CardName,
		SetName:      item.SetName,
		CardNumber:   item.CardNumber,
		Grader:       "PSA",
		GradeValue:   item.Grade,
		BuyCostCents: item.BuyCostCents,
		PurchaseDate: item.PurchaseDate,
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "purchase creation not available")
		return
	}
	if err := h.service.CreatePurchase(ctx, purchase); err != nil {
		if inventory.IsDuplicateCertNumber(err) {
			writeError(w, http.StatusConflict, fmt.Sprintf("cert %s already exists", item.CertNumber))
			return
		}
		h.logger.Error(ctx, "failed to create purchase from pending item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if err := h.pendingRepo.ResolvePendingItem(ctx, id, body.CampaignID); err != nil {
		h.logger.Error(ctx, "failed to resolve pending item after purchase created",
			observability.Err(err),
			observability.String("pendingItemID", id),
			observability.String("campaignID", body.CampaignID))
		// The purchase was committed but the pending item was not resolved.
		// Return 500 with the purchase ID so the caller knows the purchase exists
		// and can dismiss the pending item manually via DELETE /api/admin/psa-sync/pending/{id}.
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":      "purchase created but pending item could not be resolved — dismiss it manually",
			"purchaseId": purchase.ID,
		})
		return
	}

	writeJSON(w, http.StatusOK, purchase)
}

// HandleDismissPendingItem removes a pending item without creating a purchase.
// DELETE /api/admin/psa-sync/pending/{id}
func (h *PSASyncHandler) HandleDismissPendingItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := pathID(w, r, "id", "pending item ID")
	if !ok {
		return
	}

	if err := h.pendingRepo.DismissPendingItem(ctx, id); err != nil {
		if inventory.IsPendingItemNotFound(err) {
			writeError(w, http.StatusNotFound, "pending item not found")
			return
		}
		h.logger.Error(ctx, "failed to dismiss pending item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
