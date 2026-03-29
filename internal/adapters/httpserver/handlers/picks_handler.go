package handlers

import (
	"errors"
	"net/http"
	"strconv"

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

func (h *PicksHandler) HandleGetPicks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	result, err := h.service.GetLatestPicks(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to get picks", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if result == nil {
		result = []picks.Pick{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"picks": result})
}

func (h *PicksHandler) HandleGetPickHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}
	result, err := h.service.GetPickHistory(ctx, days)
	if err != nil {
		h.logger.Error(ctx, "failed to get pick history", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if result == nil {
		result = []picks.Pick{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"picks": result})
}

func (h *PicksHandler) HandleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := h.service.GetWatchlist(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to get watchlist", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if items == nil {
		items = []picks.WatchlistItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
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
