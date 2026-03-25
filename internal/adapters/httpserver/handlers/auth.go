package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	stateCookieName = "oauth_state"
)

// AuthHandlers handles authentication-related HTTP requests
type AuthHandlers struct {
	authService   auth.Service
	logger        observability.Logger
	secureCookies bool     // Use secure cookies in production
	adminEmails   []string // Emails that are always allowed and always admin
}

// NewAuthHandlers creates a new auth handlers instance
func NewAuthHandlers(
	authService auth.Service,
	logger observability.Logger,
	secureCookies bool,
	adminEmails []string,
) *AuthHandlers {
	return &AuthHandlers{
		authService:   authService,
		logger:        logger,
		secureCookies: secureCookies,
		adminEmails:   adminEmails,
	}
}

// HandleGoogleLogin initiates the Google OAuth flow
func (h *AuthHandlers) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Generate CSRF state token
	state, err := auth.GenerateState()
	if err != nil {
		h.logger.Error(ctx, "failed to generate state", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Store state in database for atomic validation (prevents race conditions)
	// State expires in 5 minutes
	expiresAt := time.Now().Add(5 * time.Minute)
	if err := h.authService.StoreOAuthState(ctx, state, expiresAt); err != nil {
		h.logger.Error(ctx, "failed to store OAuth state", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Also store state in cookie as a secondary check (defense in depth)
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	loginURL := h.authService.GetLoginURL(state)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

// HandleGoogleCallback handles the OAuth callback from Google
func (h *AuthHandlers) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stateParam := r.URL.Query().Get("state")
	if stateParam == "" {
		h.logger.Warn(ctx, "missing state parameter")
		writeError(w, http.StatusBadRequest, "Invalid state")
		return
	}

	// Validate state cookie as secondary check (defense in depth)
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil {
		h.logger.Warn(ctx, "missing state cookie")
		writeError(w, http.StatusBadRequest, "Invalid state")
		return
	}

	if stateParam != stateCookie.Value {
		h.logger.Warn(ctx, "state mismatch between cookie and parameter",
			observability.String("cookie", truncateID(stateCookie.Value)),
			observability.String("param", truncateID(stateParam)))
		writeError(w, http.StatusBadRequest, "Invalid state")
		return
	}

	// Atomically consume state token from database (prevents race conditions)
	// This is the primary validation - ensures one-time use
	valid, err := h.authService.ConsumeOAuthState(ctx, stateParam)
	if err != nil {
		h.logger.Error(ctx, "failed to validate OAuth state", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !valid {
		h.logger.Warn(ctx, "invalid or expired OAuth state token",
			observability.String("state", stateParam[:min(8, len(stateParam))]+"..."))
		writeError(w, http.StatusBadRequest, "Invalid or expired state")
		return
	}

	// Clear state cookie (mirror original security attributes for consistent clearing)
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	// Check for error from Google
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		h.logger.Warn(ctx, "oauth error from google",
			observability.String("error", errParam),
			observability.String("description", r.URL.Query().Get("error_description")))
		http.Redirect(w, r, "/?error=oauth_failed", http.StatusFound)
		return
	}

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		h.logger.Warn(ctx, "missing authorization code")
		writeError(w, http.StatusBadRequest, "Missing code")
		return
	}

	// Exchange code for tokens
	tokens, err := h.authService.ExchangeCodeForTokens(ctx, code)
	if err != nil {
		h.logger.Error(ctx, "failed to exchange code for tokens", observability.Err(err))
		http.Redirect(w, r, "/?error=token_exchange_failed", http.StatusFound)
		return
	}

	// Get user info from Google
	userInfo, err := h.authService.GetUserInfo(ctx, tokens.AccessToken)
	if err != nil {
		h.logger.Error(ctx, "failed to get user info from google", observability.Err(err))
		http.Redirect(w, r, "/?error=user_info_failed", http.StatusFound)
		return
	}

	googleID, username, email, avatarURL := userInfo.ProviderID, userInfo.Name, userInfo.Email, userInfo.AvatarURL

	// Check email allowlist
	isAdminEmail := h.isAdminEmail(email)
	if !isAdminEmail {
		allowed, err := h.authService.IsEmailAllowed(ctx, email)
		if err != nil {
			h.logger.Error(ctx, "failed to check email allowlist", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if !allowed {
			h.logger.Warn(ctx, "login denied: email not in allowlist",
				observability.String("email_domain", extractDomain(email)))
			http.Redirect(w, r, "/login?error=not_authorized", http.StatusFound)
			return
		}
	}

	// Get or create user
	user, err := h.authService.GetOrCreateUser(ctx, googleID, username, email, avatarURL)
	if err != nil {
		h.logger.Error(ctx, "failed to get or create user", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Ensure admin email users have admin flag set
	if isAdminEmail && !user.IsAdmin {
		if err := h.authService.SetUserAdmin(ctx, user.ID, true); err != nil {
			h.logger.Error(ctx, "failed to set admin flag for admin email", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		user.IsAdmin = true
	}

	// Create session first (needed for token storage)
	session, err := h.authService.CreateSession(ctx, user.ID, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		h.logger.Error(ctx, "failed to create session", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Store tokens (encrypted in database, linked to session for multi-device support)
	if err := h.authService.StoreTokens(ctx, user.ID, session.ID, tokens); err != nil {
		h.logger.Warn(ctx, "failed to store tokens", observability.Err(err))
		// Continue - user can still use the app, just won't have token refresh
	}

	// Set session cookie with lifetime derived from session expiry
	cookieMaxAge := computeMaxAgeFromExpiry(session.ExpiresAt)
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		MaxAge:   cookieMaxAge,
		Expires:  session.ExpiresAt, // Set Expires for browser compatibility
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})

	h.logger.Info(ctx, "user logged in successfully",
		observability.String("google_id", truncateID(googleID)),
		observability.String("email_domain", extractDomain(email)))

	// Redirect to app
	http.Redirect(w, r, "/", http.StatusFound)
}

// computeMaxAgeFromExpiry calculates cookie MaxAge in seconds from an expiry timestamp.
// The result is clamped to >= 0 to handle already-expired sessions gracefully.
// If expiresAt is the zero value, falls back to 30 days as a default.
func computeMaxAgeFromExpiry(expiresAt time.Time) int {
	// Fallback: if session does not expose expiry (zero value), use 30-day default
	if expiresAt.IsZero() {
		return 30 * 24 * 60 * 60 // 30 days in seconds
	}

	remaining := time.Until(expiresAt)
	maxAge := int(remaining.Seconds())

	// Clamp to >= 0; negative MaxAge would delete the cookie
	if maxAge < 0 {
		maxAge = 0
	}

	return maxAge
}

// HandleLogout logs out the user
func (h *AuthHandlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get session cookie
	cookie, err := r.Cookie(middleware.SessionCookieName)
	if err == nil {
		// Delete session from database
		if err := h.authService.DeleteSession(ctx, cookie.Value); err != nil {
			h.logger.Warn(ctx, "failed to delete session", observability.Err(err))
		}
	}

	// Clear session cookie (mirror original security attributes for consistent clearing)
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	// Respond with success
	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

// userResponse is a DTO that excludes sensitive fields (e.g. google_id) from the API response.
type userResponse struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	AvatarURL   string     `json:"avatar_url"`
	IsAdmin     bool       `json:"is_admin"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// HandleGetCurrentUser returns the currently authenticated user
func (h *AuthHandlers) HandleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}

	writeJSON(w, http.StatusOK, userResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		AvatarURL:   user.AvatarURL,
		IsAdmin:     user.IsAdmin,
		LastLoginAt: user.LastLoginAt,
	})
}

// isAdminEmail checks if an email is in the admin emails list from env config
func (h *AuthHandlers) isAdminEmail(email string) bool {
	for _, e := range h.adminEmails {
		if strings.EqualFold(e, email) {
			return true
		}
	}
	return false
}
