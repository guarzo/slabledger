package mocks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/domain/auth"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// InMemoryAuthService is a lightweight, repository-backed test double for
// auth.Service. It is intentionally free of external dependencies (no HTTP
// client, no logger, no OAuth credentials) so that it can be instantiated in
// unit tests without any infrastructure setup.
//
// It is a double, not a second production implementation: the only production
// implementation of auth.Service is google.OAuthService, which is what the
// binary wires up. Methods that require live OAuth (ExchangeCodeForTokens,
// GetUserInfo) return ErrNotImplemented. This type used to live in
// internal/domain/auth as `authService`, where its test-only shape read as a
// rival production implementation (SLA-88).
//
// Example:
//
//	repo := mocks.NewInMemoryAuthRepository()
//	svc, err := mocks.NewInMemoryAuthService(repo)
type InMemoryAuthService struct {
	repo       auth.Repository
	loginURLFn func(state string) string
}

// InMemoryAuthServiceOption configures an InMemoryAuthService.
type InMemoryAuthServiceOption func(*InMemoryAuthService)

// WithAuthLoginURLFn injects a custom login URL builder into the double.
// When not provided, GetLoginURL returns an empty string.
func WithAuthLoginURLFn(fn func(state string) string) InMemoryAuthServiceOption {
	return func(s *InMemoryAuthService) {
		if fn != nil {
			s.loginURLFn = fn
		}
	}
}

// NewInMemoryAuthService creates a repository-backed auth.Service double.
// Returns an error if repo is nil.
func NewInMemoryAuthService(repo auth.Repository, opts ...InMemoryAuthServiceOption) (auth.Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("mocks.NewInMemoryAuthService: repo must not be nil")
	}
	s := &InMemoryAuthService{
		repo:       repo,
		loginURLFn: func(_ string) string { return "" },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

var _ auth.Service = (*InMemoryAuthService)(nil)

// ─── OAuth flow ───────────────────────────────────────────────────────────────

func (s *InMemoryAuthService) GetLoginURL(state string) string {
	return s.loginURLFn(state)
}

// ExchangeCodeForTokens requires a live HTTP connection to Google and is not
// supported by this double.
func (s *InMemoryAuthService) ExchangeCodeForTokens(_ context.Context, _ string) (*auth.UserTokens, error) {
	return nil, apperrors.NewAppError("ERR_NOT_IMPLEMENTED", "ExchangeCodeForTokens requires OAuthService")
}

// GetUserInfo requires a live HTTP connection to Google and is not supported by
// this double.
func (s *InMemoryAuthService) GetUserInfo(_ context.Context, _ string) (*auth.UserInfo, error) {
	return nil, apperrors.NewAppError("ERR_NOT_IMPLEMENTED", "GetUserInfo requires OAuthService")
}

func (s *InMemoryAuthService) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	return s.repo.StoreOAuthState(ctx, state, expiresAt)
}

func (s *InMemoryAuthService) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	return s.repo.ConsumeOAuthState(ctx, state)
}

func (s *InMemoryAuthService) CleanupExpiredOAuthStates(ctx context.Context) (int, error) {
	return s.repo.CleanupExpiredOAuthStates(ctx)
}

// ─── Session management ───────────────────────────────────────────────────────

func (s *InMemoryAuthService) CreateSession(ctx context.Context, userID int64, userAgent, ipAddress string) (*auth.Session, error) {
	now := time.Now()
	session := &auth.Session{
		ID:             uuid.New().String(),
		UserID:         userID,
		ExpiresAt:      now.Add(auth.DefaultSessionExpiry),
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

func (s *InMemoryAuthService) ValidateSession(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
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

func (s *InMemoryAuthService) DeleteSession(ctx context.Context, sessionID string) error {
	return s.repo.DeleteSession(ctx, sessionID)
}

func (s *InMemoryAuthService) CleanupExpiredSessions(ctx context.Context) (int, error) {
	return s.repo.DeleteExpiredSessions(ctx)
}

// ─── User management ──────────────────────────────────────────────────────────

func (s *InMemoryAuthService) GetOrCreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	user, err := s.repo.GetUserByGoogleID(ctx, googleID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, auth.ErrUserNotFound) {
		return nil, apperrors.StorageError("get user by google id", err)
	}
	return s.repo.CreateUser(ctx, googleID, username, email, avatarURL)
}

func (s *InMemoryAuthService) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}

func (s *InMemoryAuthService) UpdateLastLogin(ctx context.Context, userID int64) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	now := time.Now()
	user.LastLoginAt = &now
	return s.repo.UpdateUser(ctx, user)
}

// ─── Token storage ────────────────────────────────────────────────────────────

func (s *InMemoryAuthService) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	return s.repo.StoreTokens(ctx, userID, sessionID, tokens)
}

// ─── Email allowlist ──────────────────────────────────────────────────────────

func (s *InMemoryAuthService) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	return s.repo.IsEmailAllowed(ctx, email)
}

func (s *InMemoryAuthService) ListAllowedEmails(ctx context.Context) ([]auth.AllowedEmail, error) {
	return s.repo.ListAllowedEmails(ctx)
}

func (s *InMemoryAuthService) AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error {
	return s.repo.AddAllowedEmail(ctx, email, addedBy, notes)
}

func (s *InMemoryAuthService) RemoveAllowedEmail(ctx context.Context, email string) error {
	return s.repo.RemoveAllowedEmail(ctx, email)
}

// ─── Admin ────────────────────────────────────────────────────────────────────

func (s *InMemoryAuthService) ListUsers(ctx context.Context) ([]auth.User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *InMemoryAuthService) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	return s.repo.SetUserAdmin(ctx, userID, isAdmin)
}
