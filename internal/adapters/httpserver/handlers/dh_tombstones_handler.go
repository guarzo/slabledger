package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// DHTombstonesHandler exposes admin endpoints for inspecting and clearing the
// DH card tombstone table.
type DHTombstonesHandler struct {
	repo   pricing.DHCardTombstoneRepo
	logger observability.Logger
}

// NewDHTombstonesHandler constructs the handler. A nil repo is allowed and
// causes endpoints to return 503 so deployments without DH wired in still boot.
func NewDHTombstonesHandler(repo pricing.DHCardTombstoneRepo, logger observability.Logger) *DHTombstonesHandler {
	return &DHTombstonesHandler{repo: repo, logger: logger}
}

// HandleCount handles GET /api/admin/dh-tombstones/count.
func (h *DHTombstonesHandler) HandleCount(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "DH tombstones not configured")
		return
	}
	n, err := h.repo.Count(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "dh tombstones count failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "count failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

// HandleClear handles POST /api/admin/dh-tombstones/clear.
func (h *DHTombstonesHandler) HandleClear(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "DH tombstones not configured")
		return
	}
	cleared, err := h.repo.ClearAll(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "dh tombstones clear failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "clear failed")
		return
	}
	h.logger.Info(r.Context(), "dh tombstones cleared", observability.Int("cleared", cleared))
	writeJSON(w, http.StatusOK, map[string]int{"cleared": cleared})
}
