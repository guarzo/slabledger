package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// PSASyncRefresher provides last-run stats from the PSA sync scheduler.
type PSASyncRefresher interface {
	GetLastRunStats() *scheduler.PSASyncRunStats
}

// PSASyncPurchaseCreator creates purchases (subset of campaigns.Service).
type PSASyncPurchaseCreator interface {
	CreatePurchase(ctx context.Context, p *campaigns.Purchase) error
}

// PSASyncHandlerConfig holds dependencies for the PSA sync handler.
type PSASyncHandlerConfig struct {
	PendingRepo   campaigns.PendingItemRepository
	Refresher     PSASyncRefresher       // optional
	Service       PSASyncPurchaseCreator // optional
	SpreadsheetID string
	Interval      string
	Logger        observability.Logger
}

// PSASyncHandler serves PSA sync status and pending-item CRUD endpoints.
type PSASyncHandler struct {
	pendingRepo   campaigns.PendingItemRepository
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

// HandleListPendingItems returns all pending items awaiting user resolution.
// GET /api/purchases/psa-pending
func (h *PSASyncHandler) HandleListPendingItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items, ok := serviceCall(w, ctx, h.logger, "failed to list pending items", func() ([]campaigns.PendingItem, error) {
		return h.pendingRepo.ListPendingItems(ctx)
	})
	if !ok {
		return
	}
	if items == nil {
		items = []campaigns.PendingItem{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleAssignPendingItem assigns a pending item to a campaign by creating
// a purchase and resolving the pending item.
// POST /api/purchases/psa-pending/{id}/assign
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
		if campaigns.IsPendingItemNotFound(err) {
			writeError(w, http.StatusNotFound, "pending item not found")
			return
		}
		h.logger.Error(ctx, "failed to get pending item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	purchase := &campaigns.Purchase{
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

	if err := h.service.CreatePurchase(ctx, purchase); err != nil {
		if campaigns.IsDuplicateCertNumber(err) {
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
		writeJSON(w, http.StatusOK, map[string]any{
			"purchase": purchase,
			"warning":  "purchase created but pending item could not be resolved",
		})
		return
	}

	writeJSON(w, http.StatusOK, purchase)
}

// HandleDismissPendingItem removes a pending item without creating a purchase.
// DELETE /api/purchases/psa-pending/{id}
func (h *PSASyncHandler) HandleDismissPendingItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := pathID(w, r, "id", "pending item ID")
	if !ok {
		return
	}

	if err := h.pendingRepo.DismissPendingItem(ctx, id); err != nil {
		if campaigns.IsPendingItemNotFound(err) {
			writeError(w, http.StatusNotFound, "pending item not found")
			return
		}
		h.logger.Error(ctx, "failed to dismiss pending item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
