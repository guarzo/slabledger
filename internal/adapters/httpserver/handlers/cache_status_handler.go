package handlers

import (
	"context"
	"net/http"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CacheStatsProvider exposes persistent cache statistics.
// Implemented by tcgdex.TCGdex.
type CacheStatsProvider interface {
	GetCacheStats(ctx context.Context) (domainCards.CacheStats, error)
}

// CacheStatusHandler serves card data cache statistics.
type CacheStatusHandler struct {
	provider CacheStatsProvider
	logger   observability.Logger
}

// NewCacheStatusHandler creates a new handler for the cache status endpoint.
func NewCacheStatusHandler(provider CacheStatsProvider, logger observability.Logger) *CacheStatusHandler {
	return &CacheStatusHandler{provider: provider, logger: logger}
}

// HandleCacheStats returns current card data cache statistics.
func (h *CacheStatusHandler) HandleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if h.provider == nil {
		writeJSON(w, http.StatusOK, domainCards.CacheStats{Enabled: false})
		return
	}

	stats, err := h.provider.GetCacheStats(r.Context())
	if err != nil {
		h.logger.Warn(r.Context(), "failed to get cache stats", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Failed to retrieve cache stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
