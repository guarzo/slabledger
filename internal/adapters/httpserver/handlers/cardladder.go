package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CLRefresher runs a Card Ladder value refresh cycle on demand.
type CLRefresher interface {
	RunOnce(ctx context.Context) error
	// GetLastRunStats returns stats from the most recent refresh run, or nil if
	// no run has completed yet.
	GetLastRunStats() *scheduler.CLRunStats
	// SetClient replaces the API client used by the scheduler (e.g. after
	// credentials are saved at runtime).
	SetClient(client *cardladder.Client)
}

// CardLadderHandler manages Card Ladder admin endpoints.
type CardLadderHandler struct {
	mu             sync.Mutex
	store          *sqlite.CardLadderStore
	client         *cardladder.Client
	refresher      CLRefresher
	purchaseLister CLPurchaseLister
	syncUpdater    CLSyncUpdater
	logger         observability.Logger
}

// SetRefresher injects the refresh trigger after scheduler construction.
func (h *CardLadderHandler) SetRefresher(r CLRefresher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refresher = r
}

// NewCardLadderHandler creates a new Card Ladder admin handler.
func NewCardLadderHandler(store *sqlite.CardLadderStore, client *cardladder.Client, logger observability.Logger) *CardLadderHandler {
	return &CardLadderHandler{store: store, client: client, logger: logger}
}

type cardLadderConfigRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	CollectionID   string `json:"collectionId"`
	FirebaseAPIKey string `json:"firebaseApiKey"`
}

// HandleSaveConfig authenticates with Firebase and stores the refresh token.
func (h *CardLadderHandler) HandleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req cardLadderConfigRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Email == "" || req.Password == "" || req.CollectionID == "" || req.FirebaseAPIKey == "" {
		writeError(w, http.StatusBadRequest, "email, password, collectionId, and firebaseApiKey are required")
		return
	}

	// Create a temporary auth client with the provided API key
	tempAuth := cardladder.NewFirebaseAuth(req.FirebaseAPIKey)
	authResp, err := tempAuth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		h.logger.Error(r.Context(), "Card Ladder Firebase login failed", observability.Err(err))
		writeError(w, http.StatusUnauthorized, "Firebase authentication failed")
		return
	}

	if err := h.store.SaveConfig(r.Context(), req.Email, authResp.RefreshToken, req.CollectionID, req.FirebaseAPIKey, authResp.LocalID); err != nil {
		h.logger.Error(r.Context(), "failed to save Card Ladder config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	// Atomically update or create the live client
	h.mu.Lock()
	if h.client != nil {
		h.client.UpdateCredentials(
			cardladder.NewFirebaseAuth(req.FirebaseAPIKey),
			authResp.RefreshToken,
		)
	} else {
		h.client = cardladder.NewClient(
			cardladder.WithTokenManager(
				cardladder.NewFirebaseAuth(req.FirebaseAPIKey),
				authResp.RefreshToken,
				time.Time{},
			),
		)
	}
	// Capture client ref under lock before pushing to scheduler.
	client := h.client
	h.mu.Unlock()

	// Push the new/updated client to the scheduler so it picks up new credentials.
	if h.refresher != nil {
		h.refresher.SetClient(client)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

// HandleStatus returns the current Card Ladder connection status.
func (h *CardLadderHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetConfig(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get Card Ladder config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	status := map[string]any{
		"configured": cfg != nil,
	}
	if cfg != nil {
		status["email"] = cfg.Email
		status["collectionId"] = cfg.CollectionID
	}

	mappings, err := h.store.ListMappings(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to list CL mappings", observability.Err(err))
	} else {
		status["cardsMapped"] = len(mappings)
	}

	priceStats, err := h.store.GetCLPriceStats(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get CL price stats", observability.Err(err))
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

// HandleRefresh triggers a manual CL value sync.
func (h *CardLadderHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	refresher := h.refresher
	h.mu.Unlock()
	if refresher == nil {
		writeError(w, http.StatusServiceUnavailable, "Card Ladder refresh scheduler not available")
		return
	}
	if err := refresher.RunOnce(r.Context()); err != nil {
		h.logger.Error(r.Context(), "manual CL refresh failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "refresh complete"})
}

// HandleFailures returns a breakdown of per-purchase CL mapping/pricing failures
// grouped by reason, with a bounded sample of recent failed purchases for the
// admin UI to display.
func (h *CardLadderHandler) HandleFailures(w http.ResponseWriter, r *http.Request) {
	limit, _ := parsePagination(r)
	report, err := h.store.GetCLFailures(r.Context(), limit)
	if err != nil {
		h.logger.Error(r.Context(), "failed to load CL failures", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, report)
}
