package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// PSASnapshotInfo reports when the PSA portal rows snapshot was last harvested.
type PSASnapshotInfo interface {
	SnapshotFetchedAt(ctx context.Context) (time.Time, error)
}

// PSASyncRefresher runs a PSA sync cycle on demand and provides last-run stats.
type PSASyncRefresher interface {
	RunOnce(ctx context.Context) error
	GetLastRunStats() *scheduler.PSASyncRunStats
}

// PSASyncPurchaseCreator resolves a pending item into a purchase. It spans two
// of inventory.Service's concerns — purchase CRUD and cert-keyed lookup — so no
// single published sub-interface covers it. Assignment is update-or-create
// rather than create-only: the reconciler enqueues items for purchases that
// already exist, so the cert lookup and reassignment are as load-bearing as the
// create.
type PSASyncPurchaseCreator interface {
	CreatePurchase(ctx context.Context, p *inventory.Purchase) error
	GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
	ReassignPurchase(ctx context.Context, purchaseID string, newCampaignID string) error
}

// PSASyncHandlerConfig holds dependencies for the PSA sync handler.
type PSASyncHandlerConfig struct {
	PendingRepo  inventory.PendingItemRepository
	Refresher    PSASyncRefresher       // optional
	Service      PSASyncPurchaseCreator // optional
	SnapshotInfo PSASnapshotInfo        // optional
	Interval     string
	Logger       observability.Logger
}

// PSASyncHandler serves PSA sync status and pending-item CRUD endpoints.
type PSASyncHandler struct {
	pendingRepo  inventory.PendingItemRepository
	refresher    PSASyncRefresher
	service      PSASyncPurchaseCreator
	snapshotInfo PSASnapshotInfo
	interval     string
	logger       observability.Logger
}

// NewPSASyncHandler creates a new PSASyncHandler from the given config.
func NewPSASyncHandler(cfg PSASyncHandlerConfig) *PSASyncHandler {
	return &PSASyncHandler{
		pendingRepo:  cfg.PendingRepo,
		refresher:    cfg.Refresher,
		service:      cfg.Service,
		snapshotInfo: cfg.SnapshotInfo,
		interval:     cfg.Interval,
		logger:       cfg.Logger,
	}
}

// HandleStatus returns the PSA sync configuration and last-run stats.
// GET /api/admin/psa-sync/status
func (h *PSASyncHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// "configured" means the PSA portal sync is wired (a scheduler/refresher exists);
	// the daily run is separately gated by PSA_SYNC_ENABLED.
	resp := map[string]any{
		"configured": h.refresher != nil,
		"interval":   h.interval,
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

	if h.snapshotInfo != nil {
		at, err := h.snapshotInfo.SnapshotFetchedAt(ctx)
		switch {
		case err != nil:
			h.logger.Warn(ctx, "failed to read snapshot fetched_at", observability.Err(err))
		case !at.IsZero():
			resp["snapshotFetchedAt"] = at.UTC().Format(time.RFC3339)
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

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "purchase creation not available")
		return
	}

	// A pending item exists precisely because the automated paths could not decide:
	// FindMatchingCampaign returned "ambiguous"/"unmatched", or PSA named a campaign
	// that did not resolve. Whichever campaign the operator posts — one of the
	// suggested candidates or something else entirely — a human broke a tie the
	// machine refused to break, so 'manual' is the honest source. The request body
	// carries only campaignId, so this handler cannot distinguish "accepted a
	// suggestion" from "overrode it" — and it does not need to, since both are a
	// human decision. Note for anyone reading attribution_source analytically:
	// 'manual' here means "a person decided", not "a person disagreed with us".
	//
	// Both branches below record that source: CreatePurchase from the struct
	// field, ReassignPurchase because UpdatePurchaseCampaign stamps 'manual'.
	purchase, ok := h.resolvePendingItemToPurchase(ctx, w, item, body.CampaignID)
	if !ok {
		return
	}

	if err := h.pendingRepo.ResolvePendingItem(ctx, id, body.CampaignID); err != nil {
		h.logger.Error(ctx, "failed to resolve pending item after assignment",
			observability.Err(err),
			observability.String("pendingItemID", id),
			observability.String("campaignID", body.CampaignID))
		// The assignment was committed — the purchase was created or reassigned —
		// but the pending item was not resolved. Return 500 with the purchase ID so
		// the caller knows the assignment stuck and can dismiss the pending item
		// manually via DELETE /api/admin/psa-sync/pending/{id}.
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":      "campaign assigned but pending item could not be resolved — dismiss it manually",
			"purchaseId": purchase.ID,
		})
		return
	}

	writeJSON(w, http.StatusOK, purchase)
}

// resolvePendingItemToPurchase books the operator's campaign choice against the
// item's cert: it reassigns the purchase when one already exists, and creates
// one otherwise. Reconciliation enqueues items for certs we already own (a PSA
// campaign name that stopped resolving), and campaign_purchases is UNIQUE on
// (grader, cert_number) — so a create-only path answered those items with a 409
// the operator could not clear from the UI.
//
// It writes its own error response and returns ok=false when the assignment
// could not be booked.
func (h *PSASyncHandler) resolvePendingItemToPurchase(
	ctx context.Context, w http.ResponseWriter, item *inventory.PendingItem, campaignID string,
) (*inventory.Purchase, bool) {
	// A failed lookup is not "no such purchase": falling through to create on
	// error is exactly the 409 this path exists to avoid.
	existing, err := h.service.GetPurchasesByCertNumbers(ctx, []string{item.CertNumber})
	if err != nil {
		h.logger.Error(ctx, "failed to look up purchase for pending item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return nil, false
	}

	if p := existing[item.CertNumber]; p != nil {
		if err := h.service.ReassignPurchase(ctx, p.ID, campaignID); err != nil {
			// A sold purchase's campaign is frozen: sale_fee_cents and
			// net_profit_cents were computed from it. Say so rather than
			// returning a bare 500.
			if errors.Is(err, inventory.ErrPurchaseHasSale) {
				writeError(w, http.StatusConflict,
					fmt.Sprintf("cert %s has been sold and cannot be reassigned", item.CertNumber))
				return nil, false
			}
			h.logger.Error(ctx, "failed to reassign purchase for pending item", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return nil, false
		}
		p.CampaignID = campaignID
		p.AttributionSource = inventory.AttributionSourceManual
		return p, true
	}

	purchase := &inventory.Purchase{
		CampaignID:        campaignID,
		CertNumber:        item.CertNumber,
		CardName:          item.CardName,
		SetName:           item.SetName,
		CardNumber:        item.CardNumber,
		Grader:            "PSA",
		GradeValue:        item.Grade,
		BuyCostCents:      item.BuyCostCents,
		PurchaseDate:      item.PurchaseDate,
		AttributionSource: inventory.AttributionSourceManual,
	}
	if err := h.service.CreatePurchase(ctx, purchase); err != nil {
		if inventory.IsDuplicateCertNumber(err) {
			writeError(w, http.StatusConflict, fmt.Sprintf("cert %s already exists", item.CertNumber))
			return nil, false
		}
		h.logger.Error(ctx, "failed to create purchase from pending item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return nil, false
	}
	return purchase, true
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
