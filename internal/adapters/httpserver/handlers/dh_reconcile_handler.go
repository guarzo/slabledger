package handlers

import (
	"net/http"

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
