package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"
)

// GenerateState generates a random state string for OAuth CSRF protection
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Service handles authentication and session management operations
type Service interface {
	// OAuth flow
	GetLoginURL(state string) string
	ExchangeCodeForTokens(ctx context.Context, code string) (*UserTokens, error)
	GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error)
	StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error
	ConsumeOAuthState(ctx context.Context, state string) (bool, error)

	// Session management
	CreateSession(ctx context.Context, userID int64, userAgent, ipAddress string) (*Session, error)
	ValidateSession(ctx context.Context, sessionID string) (*Session, *User, error)
	DeleteSession(ctx context.Context, sessionID string) error
	CleanupExpiredSessions(ctx context.Context) (int, error)

	// User management
	GetOrCreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*User, error)
	GetUserByID(ctx context.Context, userID int64) (*User, error)
	UpdateLastLogin(ctx context.Context, userID int64) error

	// Token storage (session-based for multi-device support)
	StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *UserTokens) error

	// Email allowlist
	IsEmailAllowed(ctx context.Context, email string) (bool, error)
	ListAllowedEmails(ctx context.Context) ([]AllowedEmail, error)
	AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error
	RemoveAllowedEmail(ctx context.Context, email string) error

	// Admin
	ListUsers(ctx context.Context) ([]User, error)
	SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error
}
