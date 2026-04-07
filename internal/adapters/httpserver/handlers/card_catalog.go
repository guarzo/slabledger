package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CatalogSearcher abstracts the CL card catalog search.
type CatalogSearcher interface {
	FetchCardCatalog(ctx context.Context, query string, filters map[string]string, page, limit int) (*cardladder.SearchResponse[cardladder.CatalogCard], error)
}

// CardCatalogHandler serves CL card catalog search results.
type CardCatalogHandler struct {
	searcher CatalogSearcher
	logger   observability.Logger
}

// NewCardCatalogHandler creates a new card catalog handler.
func NewCardCatalogHandler(searcher CatalogSearcher, logger observability.Logger) *CardCatalogHandler {
	return &CardCatalogHandler{searcher: searcher, logger: logger}
}

// HandleSearch handles GET /api/cards/catalog?q=...&condition=...&set=...&limit=20
func (h *CardCatalogHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "q parameter required")
		return
	}

	limit := 20
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = v
	}

	page := 0
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v >= 0 {
		page = v
	}

	filters := make(map[string]string)
	if v := q.Get("condition"); v != "" {
		filters["condition"] = v
	}
	if v := q.Get("set"); v != "" {
		filters["set"] = v
	}
	if v := q.Get("gradingCompany"); v != "" {
		filters["gradingCompany"] = v
	}
	if v := q.Get("category"); v != "" {
		filters["category"] = v
	}

	resp, err := h.searcher.FetchCardCatalog(r.Context(), query, filters, page, limit)
	if err != nil {
		h.logger.Error(r.Context(), "card catalog search failed",
			observability.Err(err), observability.String("query", query))
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
