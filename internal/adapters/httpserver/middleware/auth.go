package middleware

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SessionCookieName is the cookie name used for session tokens.
const SessionCookieName = "session_id"

// contextKey is an unexported type for context keys to avoid cross-package collisions.
type contextKey string

// UserContextKey is the context key for storing the authenticated user.
var UserContextKey contextKey = "user"

// AuthMiddleware handles authentication
type AuthMiddleware struct {
	authService   auth.Service
	logger        observability.Logger
	localAPIToken string // optional token for CLI/curl access without browser OAuth
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(authService auth.Service, logger observability.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
		logger:      logger,
	}
}

// WithLocalAPIToken enables local API token authentication as an alternative to session cookies.
// When set, requests with "Authorization: Bearer <token>" bypass OAuth session validation.
func (m *AuthMiddleware) WithLocalAPIToken(token string) *AuthMiddleware {
	m.localAPIToken = token
	return m
}

// RequireAuth requires authentication
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.validateSession(r)
		if err != nil {
			m.logger.Warn(r.Context(), "authentication required",
				observability.String("path", r.URL.Path),
				observability.String("remote_addr", r.RemoteAddr),
				observability.Err(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Attach user to context
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuth optionally authenticates but doesn't fail if no auth
func (m *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.validateSession(r)
		if err == nil && user != nil {
			// Attach user to context
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)
	})
}

// validateSession validates the session cookie (or local API token) and returns the user.
func (m *AuthMiddleware) validateSession(r *http.Request) (*auth.User, error) {
	// Check for local API token in Authorization header
	if m.localAPIToken != "" {
		if header := r.Header.Get("Authorization"); header != "" {
			token, ok := strings.CutPrefix(header, "Bearer ")
			if ok && len(token) == len(m.localAPIToken) && subtle.ConstantTimeCompare([]byte(token), []byte(m.localAPIToken)) == 1 {
				return m.resolveLocalAPIUser(r.Context())
			}
		}
	}

	// Fall back to session cookie (requires authService)
	if m.authService == nil {
		return nil, fmt.Errorf("no session service configured")
	}

	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, err
	}

	_, user, err := m.authService.ValidateSession(r.Context(), cookie.Value)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// resolveLocalAPIUser returns a real DB-backed user for local API token auth.
// When authService is available, it ensures a "local-api" user exists in the users
// table so that FK constraints (favorites, price_flags) are satisfied.
// If the DB call fails, the error is propagated — falling back to a synthetic ID=0
// user would violate FK constraints on subsequent writes.
// When authService is nil, it falls back to a synthetic user with ID 0.
func (m *AuthMiddleware) resolveLocalAPIUser(ctx context.Context) (*auth.User, error) {
	if m.authService != nil {
		user, err := m.authService.GetOrCreateUser(ctx, "local-api-token", "local-api", "local@localhost", "")
		if err != nil {
			return nil, fmt.Errorf("resolve local API user: %w", err)
		}
		// Ensure admin privileges for local API token
		user.IsAdmin = true
		return user, nil
	}
	return &auth.User{
		ID:       0,
		Username: "local-api",
		Email:    "local@localhost",
		IsAdmin:  true,
	}, nil
}

// RequireAdmin requires the authenticated user to be an admin
func (m *AuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if !ok || user == nil {
			m.logger.Warn(r.Context(), "admin access denied: user not found",
				observability.String("path", r.URL.Path))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if !user.IsAdmin {
			m.logger.Warn(r.Context(), "admin access denied",
				observability.String("user_id", fmt.Sprintf("%d", user.ID)),
				observability.String("path", r.URL.Path))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// GetUserFromContext retrieves the user from the request context
func GetUserFromContext(ctx context.Context) (*auth.User, bool) {
	user, ok := ctx.Value(UserContextKey).(*auth.User)
	return user, ok
}
