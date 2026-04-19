package handlers

import (
	"context"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	domainobs "github.com/guarzo/slabledger/internal/domain/observability"
)

// reconcileResponse is the JSON shape for POST /api/dh/reconcile.
// Field names match the frontend's expected camelCase.
type reconcileResponse struct {
	Scanned     int      `json:"scanned"`
	MissingOnDH int      `json:"missingOnDH"`
	Reset       int      `json:"reset"`
	Errors      []string `json:"errors,omitempty"`
	ResetIDs    []string `json:"resetIds,omitempty"`
}

// HandleReconcile handles POST /api/dh/reconcile. It runs the DH
// reconciliation synchronously and returns counts. Runs under a mutex so
// concurrent clicks don't double-reset the same rows.
func (h *DHHandler) HandleReconcile(w http.ResponseWriter, r *http.Request) {
	if h.reconciler == nil {
		writeError(w, http.StatusServiceUnavailable, "DH reconciliation not configured")
		return
	}

	if !h.reconcileMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "already_running"})
		return
	}
	defer h.reconcileMu.Unlock()

	result, err := h.reconciler.Reconcile(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "dh reconcile failed", domainobs.Err(err))
		writeError(w, http.StatusBadGateway, "reconcile failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, reconcileResponse{
		Scanned:     result.Scanned,
		MissingOnDH: result.MissingOnDH,
		Reset:       result.Reset,
		Errors:      result.Errors,
		ResetIDs:    result.ResetIDs,
	})
}

// DHReconcileRunner runs a reconcile cycle on demand and exposes the last
// result. The scheduler.DHReconcileScheduler already satisfies this interface.
type DHReconcileRunner interface {
	RunOnce(ctx context.Context) error
	GetLastRunResult() *dhlisting.ReconcileResult
}

// DHReconcileHandler exposes the DH reconciler over HTTP for admin-triggered
// runs. Mirrors the shape of PSASyncHandler's manual refresh path.
type DHReconcileHandler struct {
	runner DHReconcileRunner
	logger domainobs.Logger
}

// NewDHReconcileHandler constructs a DHReconcileHandler. A nil runner is
// allowed and causes HandleTrigger to return 503 so deployments without a
// configured scheduler still boot.
func NewDHReconcileHandler(runner DHReconcileRunner, logger domainobs.Logger) *DHReconcileHandler {
	return &DHReconcileHandler{runner: runner, logger: logger}
}

// HandleTrigger runs the reconciler synchronously and returns its result.
// POST /api/admin/dh-reconcile/trigger
func (h *DHReconcileHandler) HandleTrigger(w http.ResponseWriter, r *http.Request) {
	if h.runner == nil {
		writeError(w, http.StatusServiceUnavailable, "DH reconcile not configured")
		return
	}
	if err := h.runner.RunOnce(r.Context()); err != nil {
		h.logger.Error(r.Context(), "dh reconcile trigger failed", domainobs.Err(err))
		writeError(w, http.StatusBadGateway, "DH reconcile failed")
		return
	}
	result := h.runner.GetLastRunResult()
	if result == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"scanned":     0,
			"missingOnDH": 0,
			"reset":       0,
			"errors":      []string{},
			"resetIDs":    []string{},
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
