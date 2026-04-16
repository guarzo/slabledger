package handlers

import (
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleIngestOrders serves POST /api/dh/ingest-orders?since=<rfc3339>.
// Runs a one-shot DH orders ingest with the caller-supplied since timestamp
// and returns the summary as JSON. Used for manual backfills after a deploy
// or when catching up after the scheduler has been offline.
//
// The since query parameter is REQUIRED and must be RFC3339. This endpoint
// is gated on the orders ingester being wired — if the DH orders scheduler
// is not configured, returns 503.
func (h *DHHandler) HandleIngestOrders(w http.ResponseWriter, r *http.Request) {
	if h.ordersIngester == nil {
		writeError(w, http.StatusServiceUnavailable, "dh orders ingest not configured")
		return
	}

	since := r.URL.Query().Get("since")
	if since == "" {
		writeError(w, http.StatusBadRequest, "since query parameter is required (RFC3339)")
		return
	}
	if _, err := time.Parse(time.RFC3339, since); err != nil {
		writeError(w, http.StatusBadRequest, "since must be RFC3339: "+err.Error())
		return
	}

	summary, err := h.ordersIngester.RunOnce(r.Context(), since)
	if err != nil {
		h.logger.Error(r.Context(), "dh ingest orders handler: RunOnce failed",
			observability.String("since", since),
			observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to ingest orders")
		return
	}

	writeJSON(w, http.StatusOK, summary)
}
