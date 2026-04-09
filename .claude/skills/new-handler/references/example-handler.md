# Example Handler: Favorites

This shows the real Favorites handler as a worked example of the handler pattern.

## Handler Struct

**File:** `internal/adapters/httpserver/handlers/favorites.go`

```go
type FavoritesHandlers struct {
    service favorites.Service
    logger  observability.Logger
}

func NewFavoritesHandlers(service favorites.Service, logger observability.Logger) *FavoritesHandlers {
    return &FavoritesHandlers{
        service: service,
        logger:  logger,
    }
}
```

Dependencies are the domain service interface and a logger. The constructor is simple — no options pattern needed for handlers.

## GET Handler (List with Pagination)

```go
func (h *FavoritesHandlers) HandleListFavorites(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    user := requireUser(w, r)   // Returns nil + writes 401 if not authenticated
    if user == nil {
        return
    }

    // Parse query params with safe defaults
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
```

**Patterns shown:** `requireUser()` for auth, safe query param parsing with defaults, domain service call, structured error logging, `writeJSON()` helper.

## POST Handler (Create with Validation)

```go
func (h *FavoritesHandlers) HandleAddFavorite(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    user := requireUser(w, r)
    if user == nil {
        return
    }

    var input favorites.FavoriteInput
    if !decodeBody(w, r, &input) {   // Returns false + writes 400 on decode error
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
            observability.Int64("user_id", user.ID))
        writeError(w, http.StatusInternalServerError, "Internal server error")
        return
    }

    writeJSON(w, http.StatusCreated, fav)
}
```

**Patterns shown:** `decodeBody()` for JSON parsing, domain validation errors → 400, sentinel errors → specific HTTP codes (409 Conflict), success → 201 Created.

## Route Registration

In `internal/adapters/httpserver/routes.go`:

```go
authRoute(mux, "GET /api/favorites", favoritesHandler.HandleListFavorites, authMid)
authRoute(mux, "POST /api/favorites", favoritesHandler.HandleAddFavorite, authMid)
authRoute(mux, "DELETE /api/favorites", favoritesHandler.HandleRemoveFavorite, authMid)
authRoute(mux, "POST /api/favorites/toggle", favoritesHandler.HandleToggleFavorite, authMid)
authRoute(mux, "POST /api/favorites/check", favoritesHandler.HandleCheckFavorites, authMid)
```

All use `authRoute()` since favorites are user-scoped.
