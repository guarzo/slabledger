package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
)

// ShowPrepWorkerControl intentionally has no RunOnce or source capability.
type ShowPrepWorkerControl interface {
	Status(context.Context) (scheduler.ShowPrepWorkerStatus, error)
	RequestRun(context.Context, bool) error
}

type ShowPrepWorkerHandler struct{ worker ShowPrepWorkerControl }

func NewShowPrepWorkerHandler(worker ShowPrepWorkerControl) *ShowPrepWorkerHandler {
	return &ShowPrepWorkerHandler{worker: worker}
}

func (h *ShowPrepWorkerHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.worker.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Evidence worker status unavailable")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// Coverage contains only the same safe counts/state, never provider diagnostics,
// credentials, identity keys or purchase IDs. Status cannot resolve a source.
func (h *ShowPrepWorkerHandler) HandleCoverage(w http.ResponseWriter, r *http.Request) {
	h.HandleStatus(w, r)
}
func (h *ShowPrepWorkerHandler) HandleRun(w http.ResponseWriter, r *http.Request) {
	h.request(w, r, false)
}
func (h *ShowPrepWorkerHandler) HandleRetry(w http.ResponseWriter, r *http.Request) {
	h.request(w, r, true)
}
func (h *ShowPrepWorkerHandler) request(w http.ResponseWriter, r *http.Request, retry bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.worker.RequestRun(ctx, retry); err != nil {
		writeError(w, http.StatusServiceUnavailable, "Evidence worker request was not accepted")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}
