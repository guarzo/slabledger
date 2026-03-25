package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/favorites"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Note: Query parameter validation constants are defined in validation.go:
// MaxCardNameLength, MaxSetNameLength, MaxCardNumberLength

// FavoritesHandlers handles favorites-related HTTP requests
type FavoritesHandlers struct {
	service favorites.Service
	logger  observability.Logger
}

// NewFavoritesHandlers creates a new favorites handlers instance
func NewFavoritesHandlers(service favorites.Service, logger observability.Logger) *FavoritesHandlers {
	return &FavoritesHandlers{
		service: service,
		logger:  logger,
	}
}

// HandleListFavorites handles GET /api/favorites
func (h *FavoritesHandlers) HandleListFavorites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user := requireUser(w, r)
	if user == nil {
		return
	}

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 0 {
		page = 0
	}
	pageSize, err := strconv.Atoi(r.URL.Query().Get("page_size"))
	if err != nil || pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	list, err := h.service.GetFavorites(ctx, user.ID, page, pageSize)
	if err != nil {
		h.logger.Error(ctx, "failed to get favorites",
			observability.Err(err),
			observability.Int64("user_id", user.ID))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, list)
}

// HandleAddFavorite handles POST /api/favorites
func (h *FavoritesHandlers) HandleAddFavorite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user := requireUser(w, r)
	if user == nil {
		return
	}

	var input favorites.FavoriteInput
	if !decodeBody(w, r, &input) {
		return
	}

	fav, err := h.service.AddFavorite(ctx, user.ID, input)
	if err != nil {
		if favorites.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "invalid favorite data")
			return
		}
		if errors.Is(err, favorites.ErrFavoriteAlreadyExists) {
			writeError(w, http.StatusConflict, "Already favorited")
			return
		}
		h.logger.Error(ctx, "failed to add favorite",
			observability.Err(err),
			observability.Int64("user_id", user.ID),
			observability.String("card_name", input.CardName))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	h.logger.Info(ctx, "favorite added",
		observability.Int64("user_id", user.ID),
		observability.String("card_name", input.CardName),
		observability.String("set_name", input.SetName))

	writeJSON(w, http.StatusCreated, fav)
}

// HandleRemoveFavorite handles DELETE /api/favorites
func (h *FavoritesHandlers) HandleRemoveFavorite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user := requireUser(w, r)
	if user == nil {
		return
	}

	cardName := r.URL.Query().Get("card_name")
	setName := r.URL.Query().Get("set_name")
	cardNumber := r.URL.Query().Get("card_number")

	// Validate query parameter lengths to prevent abuse
	if len(cardName) > favorites.MaxCardNameLength {
		writeError(w, http.StatusBadRequest, "card_name too long")
		return
	}
	if len(setName) > favorites.MaxSetNameLength {
		writeError(w, http.StatusBadRequest, "set_name too long")
		return
	}
	if len(cardNumber) > favorites.MaxCardNumberLength {
		writeError(w, http.StatusBadRequest, "card_number too long")
		return
	}

	// Validate required fields
	if cardName == "" || setName == "" {
		writeError(w, http.StatusBadRequest, "card_name and set_name are required")
		return
	}

	err := h.service.RemoveFavorite(ctx, user.ID, cardName, setName, cardNumber)
	if err != nil {
		if errors.Is(err, favorites.ErrFavoriteNotFound) {
			writeError(w, http.StatusNotFound, "Favorite not found")
			return
		}
		h.logger.Error(ctx, "failed to remove favorite",
			observability.Err(err),
			observability.Int64("user_id", user.ID),
			observability.String("card_name", cardName))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	h.logger.Info(ctx, "favorite removed",
		observability.Int64("user_id", user.ID),
		observability.String("card_name", cardName),
		observability.String("set_name", setName))

	w.WriteHeader(http.StatusNoContent)
}

// HandleToggleFavorite handles POST /api/favorites/toggle
func (h *FavoritesHandlers) HandleToggleFavorite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user := requireUser(w, r)
	if user == nil {
		return
	}

	var input favorites.FavoriteInput
	if !decodeBody(w, r, &input) {
		return
	}

	isFavorite, err := h.service.ToggleFavorite(ctx, user.ID, input)
	if err != nil {
		if favorites.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, "invalid favorite data")
			return
		}
		h.logger.Error(ctx, "failed to toggle favorite",
			observability.Err(err),
			observability.Int64("user_id", user.ID),
			observability.String("card_name", input.CardName))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	action := "removed"
	if isFavorite {
		action = "added"
	}
	h.logger.Debug(ctx, "favorite toggled",
		observability.Int64("user_id", user.ID),
		observability.String("card_name", input.CardName),
		observability.String("action", action))

	writeJSON(w, http.StatusOK, map[string]bool{"is_favorite": isFavorite})
}

// HandleCheckFavorites handles POST /api/favorites/check
func (h *FavoritesHandlers) HandleCheckFavorites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user := requireUser(w, r)
	if user == nil {
		return
	}

	var cards []favorites.FavoriteInput
	if !decodeBody(w, r, &cards) {
		return
	}

	// Limit batch size to prevent abuse
	if len(cards) > 100 {
		writeError(w, http.StatusBadRequest, "Too many cards (max 100)")
		return
	}

	checks, err := h.service.CheckFavorites(ctx, user.ID, cards)
	if err != nil {
		h.logger.Error(ctx, "failed to check favorites",
			observability.Err(err),
			observability.Int64("user_id", user.ID),
			observability.Int("count", len(cards)))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, checks)
}
