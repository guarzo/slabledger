package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/picks"
)

type PicksHandler struct {
	service picks.Service
	logger  observability.Logger
}

func NewPicksHandler(service picks.Service, logger observability.Logger) *PicksHandler {
	return &PicksHandler{service: service, logger: logger}
}

// pickResponse mirrors picks.Pick but with dollar-denominated prices for the API.
type pickResponse struct {
	ID                int              `json:"id"`
	Date              string           `json:"date"`
	CardName          string           `json:"card_name"`
	SetName           string           `json:"set_name"`
	Grade             string           `json:"grade"`
	Direction         picks.Direction  `json:"direction"`
	Confidence        picks.Confidence `json:"confidence"`
	BuyThesis         string           `json:"buy_thesis"`
	TargetBuyPrice    float64          `json:"target_buy_price"`
	ExpectedSellPrice float64          `json:"expected_sell_price"`
	Signals           []picks.Signal   `json:"signals"`
	Rank              int              `json:"rank"`
	Source            picks.PickSource `json:"source"`
	CreatedAt         string           `json:"created_at"`
}

// watchlistItemResponse mirrors picks.WatchlistItem but with dollar-denominated prices for the API.
type watchlistItemResponse struct {
	ID               int                   `json:"id"`
	CardName         string                `json:"card_name"`
	SetName          string                `json:"set_name"`
	Grade            string                `json:"grade"`
	Source           picks.WatchlistSource `json:"source"`
	Active           bool                  `json:"active"`
	LatestAssessment *pickResponse         `json:"latest_assessment,omitempty"`
	AddedAt          string                `json:"added_at"`
	UpdatedAt        string                `json:"updated_at"`
}

func toWatchlistItemResponses(items []picks.WatchlistItem) []watchlistItemResponse {
	out := make([]watchlistItemResponse, len(items))
	for i, item := range items {
		out[i] = watchlistItemResponse{
			ID:        item.ID,
			CardName:  item.CardName,
			SetName:   item.SetName,
			Grade:     item.Grade,
			Source:    item.Source,
			Active:    item.Active,
			AddedAt:   item.AddedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if item.LatestAssessment != nil {
			resp := toPickResponses([]picks.Pick{*item.LatestAssessment})
			out[i].LatestAssessment = &resp[0]
		}
	}
	return out
}

func toPickResponses(pp []picks.Pick) []pickResponse {
	out := make([]pickResponse, len(pp))
	for i, p := range pp {
		out[i] = pickResponse{
			ID:                p.ID,
			Date:              p.Date.Format("2006-01-02"),
			CardName:          p.CardName,
			SetName:           p.SetName,
			Grade:             p.Grade,
			Direction:         p.Direction,
			Confidence:        p.Confidence,
			BuyThesis:         p.BuyThesis,
			TargetBuyPrice:    mathutil.ToDollars(int64(p.TargetBuyPrice)),
			ExpectedSellPrice: mathutil.ToDollars(int64(p.ExpectedSellPrice)),
			Signals:           p.Signals,
			Rank:              p.Rank,
			Source:            p.Source,
			CreatedAt:         p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return out
}

func (h *PicksHandler) HandleGetPicks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	result, ok := serviceCall(w, ctx, h.logger, "failed to get picks", func() ([]picks.Pick, error) {
		return h.service.GetLatestPicks(ctx)
	})
	if !ok {
		return
	}
	if result == nil {
		result = []picks.Pick{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"picks": toPickResponses(result)})
}

func (h *PicksHandler) HandleGetPickHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}
	result, ok := serviceCall(w, ctx, h.logger, "failed to get pick history", func() ([]picks.Pick, error) {
		return h.service.GetPickHistory(ctx, days)
	})
	if !ok {
		return
	}
	if result == nil {
		result = []picks.Pick{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"picks": toPickResponses(result)})
}

func (h *PicksHandler) HandleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, ok := serviceCall(w, ctx, h.logger, "failed to get watchlist", func() ([]picks.WatchlistItem, error) {
		return h.service.GetWatchlist(ctx)
	})
	if !ok {
		return
	}
	if items == nil {
		items = []picks.WatchlistItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toWatchlistItemResponses(items)})
}

func (h *PicksHandler) HandleAddWatchlistItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var input struct {
		CardName string `json:"card_name"`
		SetName  string `json:"set_name"`
		Grade    string `json:"grade"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if input.CardName == "" || input.SetName == "" || input.Grade == "" {
		writeError(w, http.StatusBadRequest, "card_name, set_name, and grade are required")
		return
	}
	item := picks.WatchlistItem{CardName: input.CardName, SetName: input.SetName, Grade: input.Grade}
	err := h.service.AddToWatchlist(ctx, item)
	if err != nil {
		if errors.Is(err, picks.ErrWatchlistDuplicate) {
			writeError(w, http.StatusConflict, "Card already on watchlist")
			return
		}
		h.logger.Error(ctx, "failed to add watchlist item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *PicksHandler) HandleDeleteWatchlistItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	err = h.service.RemoveFromWatchlist(ctx, id)
	if err != nil {
		if errors.Is(err, picks.ErrWatchlistItemNotFound) {
			writeError(w, http.StatusNotFound, "Watchlist item not found")
			return
		}
		h.logger.Error(ctx, "failed to remove watchlist item", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
