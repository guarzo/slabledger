package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/instagram"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// InstagramOAuthClient abstracts the Instagram OAuth operations.
type InstagramOAuthClient interface {
	GetLoginURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*instagram.TokenInfo, error)
}

// InstagramConfigStore abstracts Instagram configuration persistence.
type InstagramConfigStore interface {
	Get(ctx context.Context) (*sqlite.InstagramConfig, error)
	Save(ctx context.Context, token, igUserID, username string, expiresAt time.Time) error
	Delete(ctx context.Context) error
}

// OAuthStateManager abstracts the OAuth state storage used by the auth service.
type OAuthStateManager interface {
	StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error
	ConsumeOAuthState(ctx context.Context, state string) (bool, error)
}

// InstagramHandler handles Instagram OAuth and publishing endpoints.
type InstagramHandler struct {
	client  InstagramOAuthClient
	store   InstagramConfigStore
	social  social.Service
	logger  observability.Logger
	authSvc OAuthStateManager
}

// NewInstagramHandler creates a new Instagram handler.
func NewInstagramHandler(
	client InstagramOAuthClient,
	store InstagramConfigStore,
	socialSvc social.Service,
	authSvc OAuthStateManager,
	logger observability.Logger,
) *InstagramHandler {
	return &InstagramHandler{
		client:  client,
		store:   store,
		social:  socialSvc,
		logger:  logger,
		authSvc: authSvc,
	}
}

// HandleStatus returns the current Instagram connection status.
func (h *InstagramHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	cfg, err := h.store.Get(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "get instagram config failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if cfg == nil || !cfg.IsConnected {
		writeJSON(w, http.StatusOK, map[string]any{
			"connected": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connected":   true,
		"username":    cfg.Username,
		"expiresAt":   cfg.ExpiresAt.Format(time.RFC3339),
		"connectedAt": cfg.ConnectedAt.Format(time.RFC3339),
	})
}

// HandleConnect initiates the Instagram OAuth flow.
func (h *InstagramHandler) HandleConnect(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	// Generate CSRF state
	state, err := auth.GenerateState()
	if err != nil {
		h.logger.Error(r.Context(), "generate instagram oauth state failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if err := h.authSvc.StoreOAuthState(r.Context(), state, time.Now().Add(5*time.Minute)); err != nil {
		h.logger.Error(r.Context(), "store instagram oauth state failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	loginURL := h.client.GetLoginURL(state)
	writeJSON(w, http.StatusOK, map[string]string{"url": loginURL})
}

// HandleCallback handles the Instagram OAuth callback.
func (h *InstagramHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")

	if errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		h.logger.Warn(r.Context(), "instagram OAuth denied",
			observability.String("error", errParam),
			observability.String("description", errDesc))
		http.Redirect(w, r, "/?instagram=denied", http.StatusFound)
		return
	}

	if code == "" || state == "" {
		http.Redirect(w, r, "/?instagram=invalid", http.StatusFound)
		return
	}

	// Validate CSRF state
	valid, err := h.authSvc.ConsumeOAuthState(r.Context(), state)
	if err != nil || !valid {
		h.logger.Warn(r.Context(), "instagram OAuth state invalid")
		http.Redirect(w, r, "/?instagram=invalid_state", http.StatusFound)
		return
	}

	// Exchange code for long-lived token
	tokenInfo, err := h.client.ExchangeCode(r.Context(), code)
	if err != nil {
		h.logger.Error(r.Context(), "instagram token exchange failed", observability.Err(err))
		http.Redirect(w, r, "/?instagram=exchange_failed", http.StatusFound)
		return
	}

	// Store the token
	if err := h.store.Save(r.Context(), tokenInfo.AccessToken, tokenInfo.UserID, tokenInfo.Username, tokenInfo.ExpiresAt); err != nil {
		h.logger.Error(r.Context(), "store instagram token failed", observability.Err(err))
		http.Redirect(w, r, "/?instagram=save_failed", http.StatusFound)
		return
	}

	h.logger.Info(r.Context(), "instagram connected",
		observability.String("username", tokenInfo.Username),
		observability.String("igUserID", tokenInfo.UserID))

	http.Redirect(w, r, "/?instagram=connected", http.StatusFound)
}

// HandleDisconnect removes the Instagram connection.
func (h *InstagramHandler) HandleDisconnect(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	if err := h.store.Delete(r.Context()); err != nil {
		h.logger.Error(r.Context(), "disconnect instagram failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	h.logger.Info(r.Context(), "instagram disconnected")
	w.WriteHeader(http.StatusNoContent)
}

// HandlePublish starts async publishing of a social post to Instagram.
// Returns 202 Accepted immediately; the actual publish happens in background.
func (h *InstagramHandler) HandlePublish(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	id, ok := pathID(w, r, "id", "id")
	if !ok {
		return
	}

	if err := h.social.Publish(r.Context(), id); err != nil {
		h.logger.Error(r.Context(), "publish to instagram failed",
			observability.String("postId", id),
			observability.Err(err))
		switch {
		case errors.Is(err, social.ErrNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "Instagram publishing is not configured")
		case errors.Is(err, social.ErrPostNotFound), errors.Is(err, social.ErrNotPublishable):
			writeError(w, http.StatusBadRequest, "Post not found or not in a publishable state")
		default:
			writeError(w, http.StatusInternalServerError, "Failed to publish post")
		}
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "publishing"})
}
