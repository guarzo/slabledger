package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

const defaultSessionExpiry = 30 * 24 * time.Hour

// authService is a lightweight, repository-backed implementation of Service.
// It is intentionally free of external dependencies (no HTTP client, no logger,
// no OAuth credentials) so that it can be instantiated in unit tests without
// any infrastructure setup.
//
// Methods that require live OAuth (ExchangeCodeForTokens, GetUserInfo) return
// ErrNotImplemented; they are only reachable via the production OAuthService in
// the adapters layer.
type authService struct {
	repo       Repository
	loginURLFn func(state string) string
}

// Option is a functional option for configuring an authService.
type Option func(*authService)

// WithLoginURLFn injects a custom login URL builder into the service.
// When not provided, GetLoginURL returns an empty string.
func WithLoginURLFn(fn func(state string) string) Option {
	return func(s *authService) {
		if fn != nil {
			s.loginURLFn = fn
		}
	}
}

// New creates a testable, repository-backed auth.Service.
// Returns an error if repo is nil. Optional behaviour is configured via Option
// helpers (e.g. WithLoginURLFn).
func New(repo Repository, opts ...Option) (Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("auth.New: repo must not be nil")
	}
	s := &authService{
		repo:       repo,
		loginURLFn: func(_ string) string { return "" },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

var _ Service = (*authService)(nil)

// ─── OAuth flow ───────────────────────────────────────────────────────────────

func (s *authService) GetLoginURL(state string) string {
	return s.loginURLFn(state)
}

// ExchangeCodeForTokens requires a live HTTP connection to Google and is not
// supported by this lightweight implementation.
func (s *authService) ExchangeCodeForTokens(_ context.Context, _ string) (*UserTokens, error) {
	return nil, apperrors.NewAppError("ERR_NOT_IMPLEMENTED", "ExchangeCodeForTokens requires OAuthService")
}

// GetUserInfo requires a live HTTP connection to Google and is not supported by
// this lightweight implementation.
func (s *authService) GetUserInfo(_ context.Context, _ string) (*UserInfo, error) {
	return nil, apperrors.NewAppError("ERR_NOT_IMPLEMENTED", "GetUserInfo requires OAuthService")
}

func (s *authService) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	return s.repo.StoreOAuthState(ctx, state, expiresAt)
}

func (s *authService) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	return s.repo.ConsumeOAuthState(ctx, state)
}

// ─── Session management ───────────────────────────────────────────────────────

func (s *authService) CreateSession(ctx context.Context, userID int64, userAgent, ipAddress string) (*Session, error) {
	now := time.Now()
	session := &Session{
		ID:             uuid.New().String(),
		UserID:         userID,
		ExpiresAt:      now.Add(defaultSessionExpiry),
		CreatedAt:      now,
		LastAccessedAt: now,
		UserAgent:      userAgent,
		IPAddress:      ipAddress,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *authService) ValidateSession(ctx context.Context, sessionID string) (*Session, *User, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	if session.ExpiresAt.Before(time.Now()) {
		return nil, nil, apperrors.SessionExpired()
	}
	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, nil, err
	}
	return session, user, nil
}

func (s *authService) DeleteSession(ctx context.Context, sessionID string) error {
	return s.repo.DeleteSession(ctx, sessionID)
}

func (s *authService) CleanupExpiredSessions(ctx context.Context) (int, error) {
	return s.repo.DeleteExpiredSessions(ctx)
}

// ─── User management ──────────────────────────────────────────────────────────

func (s *authService) GetOrCreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*User, error) {
	user, err := s.repo.GetUserByGoogleID(ctx, googleID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return nil, apperrors.StorageError("get user by google id", err)
	}
	return s.repo.CreateUser(ctx, googleID, username, email, avatarURL)
}

func (s *authService) GetUserByID(ctx context.Context, userID int64) (*User, error) {
	return s.repo.GetUserByID(ctx, userID)
}

func (s *authService) UpdateLastLogin(ctx context.Context, userID int64) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	now := time.Now()
	user.LastLoginAt = &now
	return s.repo.UpdateUser(ctx, user)
}

// ─── Token storage ────────────────────────────────────────────────────────────

func (s *authService) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *UserTokens) error {
	return s.repo.StoreTokens(ctx, userID, sessionID, tokens)
}

// ─── Email allowlist ──────────────────────────────────────────────────────────

func (s *authService) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	return s.repo.IsEmailAllowed(ctx, email)
}

func (s *authService) ListAllowedEmails(ctx context.Context) ([]AllowedEmail, error) {
	return s.repo.ListAllowedEmails(ctx)
}

func (s *authService) AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error {
	return s.repo.AddAllowedEmail(ctx, email, addedBy, notes)
}

func (s *authService) RemoveAllowedEmail(ctx context.Context, email string) error {
	return s.repo.RemoveAllowedEmail(ctx, email)
}

// ─── Admin ────────────────────────────────────────────────────────────────────

func (s *authService) ListUsers(ctx context.Context) ([]User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *authService) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	return s.repo.SetUserAdmin(ctx, userID, isAdmin)
}
