package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CardSearchRequest represents a search request for cards
type CardSearchRequest struct {
	Query string `json:"query"` // Free-form search text
	Limit int    `json:"limit"` // Max results to return (default: 10)
}

// CardSearchResponse represents a search result
type CardSearchResponse struct {
	Cards []CardResult `json:"cards"`
	Total int          `json:"total"`
}

// CardResult represents a single card in search results
type CardResult struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Number      string  `json:"number"`
	Set         string  `json:"set"`
	SetName     string  `json:"setName"`
	Rarity      string  `json:"rarity"`
	ImageURL    string  `json:"imageUrl"`
	MarketPrice float64 `json:"marketPrice"`
	Score       float64 `json:"score"` // Relevance score
}

// HandleCardSearch handles free-form card search requests
// Supports search by:
// - Card name (partial or full)
// - Set name (partial or full)
// - Card number
// - Combined queries (e.g., "Charizard Base Set 4")
func (h *Handler) HandleCardSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	var req CardSearchRequest
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.logger.Error(ctx, "failed to decode card search request", observability.Err(err))
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	} else {
		req.Query = r.URL.Query().Get("q")
		req.Limit = 10
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "Query parameter required")
		return
	}

	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 10
	}

	// Call domain search service
	results, total, err := h.searchService.Search(ctx, req.Query, req.Limit)
	if err != nil {
		h.logger.Error(ctx, "card search failed", observability.Err(err), observability.String("query", req.Query))
		writeError(w, http.StatusInternalServerError, "Search failed")
		return
	}

	// Build response
	resp := CardSearchResponse{
		Cards: make([]CardResult, 0, len(results)),
		Total: total,
	}
	for _, sr := range results {
		resp.Cards = append(resp.Cards, CardResult{
			ID:          sr.Card.ID,
			Name:        sr.Card.Name,
			Number:      sr.Card.Number,
			Set:         sr.Card.Set,
			SetName:     sr.Card.SetName,
			Rarity:      sr.Card.Rarity,
			ImageURL:    sr.Card.ImageURL,
			MarketPrice: mathutil.ToDollars(sr.Card.MarketPrice.Cents),
			Score:       sr.Score,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
