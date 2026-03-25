package auth

import (
	"context"
	"time"
)

// Repository defines the interface for auth-related database operations
type Repository interface {
	// Users
	CreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*User, error)
	GetUserByGoogleID(ctx context.Context, googleID string) (*User, error)
	GetUserByID(ctx context.Context, userID int64) (*User, error)
	UpdateUser(ctx context.Context, user *User) error

	// Tokens (session-based for multi-device support)
	StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *UserTokens) error
	GetTokens(ctx context.Context, userID int64, sessionID string) (*UserTokens, error)
	GetTokensByUserID(ctx context.Context, userID int64) (*UserTokens, error) // Gets most recent token for user
	UpdateTokens(ctx context.Context, userID int64, sessionID string, tokens *UserTokens) error
	DeleteTokens(ctx context.Context, userID int64, sessionID string) error
	DeleteAllUserTokens(ctx context.Context, userID int64) error

	// Sessions
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, sessionID string) (*Session, error)
	UpdateSessionAccess(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	DeleteExpiredSessions(ctx context.Context) (int, error)

	// OAuth State (one-time CSRF tokens for OAuth flow)
	// StoreOAuthState stores a one-time state token with expiration.
	StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error
	// ConsumeOAuthState atomically consumes and deletes a state token.
	// Returns true if the state was valid and not expired, false otherwise.
	ConsumeOAuthState(ctx context.Context, state string) (bool, error)
	// CleanupExpiredOAuthStates removes expired state tokens.
	CleanupExpiredOAuthStates(ctx context.Context) (int, error)

	// Email Allowlist
	IsEmailAllowed(ctx context.Context, email string) (bool, error)
	ListAllowedEmails(ctx context.Context) ([]AllowedEmail, error)
	AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error
	RemoveAllowedEmail(ctx context.Context, email string) error

	// User listing (admin)
	ListUsers(ctx context.Context) ([]User, error)
	SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error
}
