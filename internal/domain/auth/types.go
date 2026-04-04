package auth

import (
	"time"
)

// User represents an authenticated user
type User struct {
	ID          int64
	GoogleID    string // Google OAuth user ID
	Username    string
	Email       string
	AvatarURL   string // Google profile picture URL
	IsAdmin     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastLoginAt *time.Time
}

// AllowedEmail represents an email in the login allowlist
type AllowedEmail struct {
	Email     string
	AddedBy   *int64 // nil if seeded from env var
	CreatedAt time.Time
	Notes     string
}

// UserTokens represents OAuth access and refresh tokens
type UserTokens struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresAt    time.Time
	Scope        string
}

// UserInfo represents user profile information from an OAuth provider
type UserInfo struct {
	ProviderID string
	Name       string
	Email      string
	AvatarURL  string
}

// Session represents an active user session
type Session struct {
	ID             string
	UserID         int64
	ExpiresAt      time.Time
	CreatedAt      time.Time
	LastAccessedAt time.Time
	UserAgent      string
	IPAddress      string
}
