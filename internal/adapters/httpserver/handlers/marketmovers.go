package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// MMRefresher runs a Market Movers value refresh cycle on demand.
type MMRefresher interface {
	RunOnce(ctx context.Context) error
	// SetClient replaces the underlying API client. Called when credentials are
	// saved for the first time after startup (i.e. no client existed at boot).
	SetClient(client *marketmovers.Client)
	// GetLastRunStats returns stats from the most recent refresh run, or nil if
	// no run has completed yet.
	GetLastRunStats() *scheduler.MMRunStats
}

// MarketMoversHandler manages Market Movers admin endpoints.
type MarketMoversHandler struct {
	mu             sync.Mutex
	store          *postgres.MarketMoversStore
	client         *marketmovers.Client
	refresher      MMRefresher
	purchaseLister MMPurchaseLister // optional: provides unsold purchases for sync
	logger         observability.Logger
}

// SetRefresher injects the refresh trigger after scheduler construction.
func (h *MarketMoversHandler) SetRefresher(r MMRefresher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refresher = r
}

// NewMarketMoversHandler creates a new Market Movers admin handler.
func NewMarketMoversHandler(store *postgres.MarketMoversStore, client *marketmovers.Client, logger observability.Logger) *MarketMoversHandler {
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
	h.updateOrCreateClient(tempAuth, authResp.RefreshToken, authResp.AccessToken, expiry)
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

// updateOrCreateClient updates the existing client or creates a new one with the given credentials.
// Must be called with h.mu held.
func (h *MarketMoversHandler) updateOrCreateClient(auth *marketmovers.Auth, refreshToken, accessToken string, expiry time.Time) {
	if h.client != nil {
		h.client.UpdateCredentials(auth, refreshToken)
		h.client.SetToken(accessToken, expiry)
	} else {
		h.client = marketmovers.NewClient(
			marketmovers.WithTokenManager(auth, refreshToken, time.Time{}),
		)
		h.client.SetToken(accessToken, expiry)
	}
	// Always push the client to the scheduler on credential save — whether new or updated.
	// This ensures the scheduler gets a fresh client reference when credentials change,
	// which is important for Market Movers because credentials can be updated at runtime
	// (unlike Card Ladder which only pushes on initial client creation).
	// The unconditional push pattern handles both first-time setup and credential refresh.
	if h.refresher != nil {
		h.refresher.SetClient(h.client)
	}
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
	if err != nil {
		h.logger.Error(r.Context(), "failed to list Market Movers mappings", observability.Err(err))
	} else {
		status["cardsMapped"] = len(mappings)
	}

	priceStats, err := h.store.GetMMPriceStats(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get MM price stats", observability.Err(err))
	} else {
		status["priceStats"] = priceStats
	}

	h.mu.Lock()
	refresher := h.refresher
	h.mu.Unlock()
	if refresher != nil {
		if stats := refresher.GetLastRunStats(); stats != nil {
			status["lastRun"] = stats
		}
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

// HandleFailures returns a breakdown of per-purchase MM mapping/pricing failures
// grouped by reason, with a bounded sample of recent failed purchases for the
// admin UI to display.
func (h *MarketMoversHandler) HandleFailures(w http.ResponseWriter, r *http.Request) {
	limit, _ := parsePagination(r)
	report, err := h.store.GetMMFailures(r.Context(), limit)
	if err != nil {
		h.logger.Error(r.Context(), "failed to load MM failures", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, report)
}
