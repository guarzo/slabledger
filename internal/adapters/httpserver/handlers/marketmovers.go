package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// MMRefresher runs a Market Movers value refresh cycle on demand.
type MMRefresher interface {
	RunOnce(ctx context.Context) error
}

// MarketMoversHandler manages Market Movers admin endpoints.
type MarketMoversHandler struct {
	mu        sync.Mutex
	store     *sqlite.MarketMoversStore
	client    *marketmovers.Client
	refresher MMRefresher
	logger    observability.Logger
}

// SetRefresher injects the refresh trigger after scheduler construction.
func (h *MarketMoversHandler) SetRefresher(r MMRefresher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refresher = r
}

// NewMarketMoversHandler creates a new Market Movers admin handler.
func NewMarketMoversHandler(store *sqlite.MarketMoversStore, client *marketmovers.Client, logger observability.Logger) *MarketMoversHandler {
	return &MarketMoversHandler{store: store, client: client, logger: logger}
}

type marketMoversConfigRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleSaveConfig authenticates with Market Movers and stores the refresh token.
func (h *MarketMoversHandler) HandleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req marketMoversConfigRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	// Authenticate to get tokens
	tempAuth := marketmovers.NewAuth()
	authResp, err := tempAuth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		h.logger.Error(r.Context(), "Market Movers login failed", observability.Err(err))
		writeError(w, http.StatusUnauthorized, "Market Movers authentication failed")
		return
	}

	if err := h.store.SaveConfig(r.Context(), req.Username, authResp.RefreshToken); err != nil {
		h.logger.Error(r.Context(), "failed to save Market Movers config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	// Parse access token expiry from JWT payload
	expiry := marketmovers.ParseJWTExpiry(authResp.AccessToken)

	// Atomically update or create the live client
	h.mu.Lock()
	if h.client != nil {
		h.client.UpdateCredentials(
			marketmovers.NewAuth(),
			authResp.RefreshToken,
		)
		h.client.SetToken(authResp.AccessToken, expiry)
	} else {
		h.client = marketmovers.NewClient(
			marketmovers.WithTokenManager(
				marketmovers.NewAuth(),
				authResp.RefreshToken,
				time.Time{},
			),
		)
		h.client.SetToken(authResp.AccessToken, expiry)
	}
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

// HandleStatus returns the current Market Movers connection status.
func (h *MarketMoversHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetConfig(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get Market Movers config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	status := map[string]any{
		"configured": cfg != nil,
	}
	if cfg != nil {
		status["username"] = cfg.Username
	}

	mappings, err := h.store.ListMappings(r.Context())
	if err == nil {
		status["cardsMapped"] = len(mappings)
	}

	writeJSON(w, http.StatusOK, status)
}

// HandleRefresh triggers a manual Market Movers value sync.
func (h *MarketMoversHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	refresher := h.refresher
	h.mu.Unlock()
	if refresher == nil {
		writeError(w, http.StatusServiceUnavailable, "Market Movers refresh scheduler not available")
		return
	}
	if err := refresher.RunOnce(r.Context()); err != nil {
		h.logger.Error(r.Context(), "manual MM refresh failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "refresh complete"})
}
