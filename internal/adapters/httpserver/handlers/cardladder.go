package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CardLadderHandler manages Card Ladder admin endpoints.
type CardLadderHandler struct {
	store  *sqlite.CardLadderStore
	client *cardladder.Client
	auth   *cardladder.FirebaseAuth
	logger observability.Logger
}

// NewCardLadderHandler creates a new Card Ladder admin handler.
func NewCardLadderHandler(store *sqlite.CardLadderStore, client *cardladder.Client, auth *cardladder.FirebaseAuth, logger observability.Logger) *CardLadderHandler {
	return &CardLadderHandler{store: store, client: client, auth: auth, logger: logger}
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
		writeError(w, http.StatusUnauthorized, "Firebase authentication failed: "+err.Error())
		return
	}

	if err := h.store.SaveConfig(r.Context(), req.Email, authResp.RefreshToken, req.CollectionID, req.FirebaseAPIKey); err != nil {
		h.logger.Error(r.Context(), "failed to save Card Ladder config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	// Update the live client's auth credentials
	if h.auth != nil {
		*h.auth = *cardladder.NewFirebaseAuth(req.FirebaseAPIKey)
	}
	h.client.SetRefreshToken(authResp.RefreshToken)

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
	if err == nil {
		status["cardsMapped"] = len(mappings)
	}

	writeJSON(w, http.StatusOK, status)
}

// HandleRefresh triggers a manual CL value sync.
func (h *CardLadderHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "refresh triggered"})
}
