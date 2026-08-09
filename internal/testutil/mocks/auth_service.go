package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	appErrs "github.com/guarzo/slabledger/internal/domain/errors"
)

// AuthServiceMock is a test double for auth.Service.
// Override any Fn field to customize behavior per-test; unset methods return a
// benign zero value or a "not implemented" internal error.
//
// It also satisfies the narrow consumer-side seams carved out of auth.Service
// (scheduler.SessionJanitor, middleware.SessionValidator,
// handlers.AdminDirectory), so tests for those consumers can use it directly.
//
// Example:
//
//	svc := &AuthServiceMock{}
//	svc.ValidateSessionFn = func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
//	    return &auth.Session{ID: sessionID}, &auth.User{ID: 1}, nil
//	}
type AuthServiceMock struct {
	// OAuth flow
	GetLoginURLFn               func(state string) string
	ExchangeCodeForTokensFn     func(ctx context.Context, code string) (*auth.UserTokens, error)
	GetUserInfoFn               func(ctx context.Context, accessToken string) (*auth.UserInfo, error)
	StoreOAuthStateFn           func(ctx context.Context, state string, expiresAt time.Time) error
	ConsumeOAuthStateFn         func(ctx context.Context, state string) (bool, error)
	CleanupExpiredOAuthStatesFn func(ctx context.Context) (int, error)

	// Session management
	CreateSessionFn          func(ctx context.Context, userID int64, userAgent, ipAddress string) (*auth.Session, error)
	ValidateSessionFn        func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error)
	DeleteSessionFn          func(ctx context.Context, sessionID string) error
	CleanupExpiredSessionsFn func(ctx context.Context) (int, error)

	// User management
	GetOrCreateUserFn func(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error)
	GetUserByIDFn     func(ctx context.Context, userID int64) (*auth.User, error)
	UpdateLastLoginFn func(ctx context.Context, userID int64) error

	// Token storage
	StoreTokensFn func(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error

	// Email allowlist
	IsEmailAllowedFn     func(ctx context.Context, email string) (bool, error)
	ListAllowedEmailsFn  func(ctx context.Context) ([]auth.AllowedEmail, error)
	AddAllowedEmailFn    func(ctx context.Context, email string, addedBy int64, notes string) error
	RemoveAllowedEmailFn func(ctx context.Context, email string) error

	// Admin
	ListUsersFn    func(ctx context.Context) ([]auth.User, error)
	SetUserAdminFn func(ctx context.Context, userID int64, isAdmin bool) error
}

var _ auth.Service = (*AuthServiceMock)(nil)

// errAuthNotImplemented is what an unset Fn field returns when there is no
// sensible zero value — the project's internal error type, not stdlib.
func errAuthNotImplemented() error {
	return appErrs.NewAppError(appErrs.ErrCodeInternal, "not implemented")
}

func (m *AuthServiceMock) GetLoginURL(state string) string {
	if m.GetLoginURLFn != nil {
		return m.GetLoginURLFn(state)
	}
	return "https://accounts.google.com/o/oauth2/v2/auth?state=" + state
}

func (m *AuthServiceMock) ExchangeCodeForTokens(ctx context.Context, code string) (*auth.UserTokens, error) {
	if m.ExchangeCodeForTokensFn != nil {
		return m.ExchangeCodeForTokensFn(ctx, code)
	}
	return nil, errAuthNotImplemented()
}

func (m *AuthServiceMock) GetUserInfo(ctx context.Context, accessToken string) (*auth.UserInfo, error) {
	if m.GetUserInfoFn != nil {
		return m.GetUserInfoFn(ctx, accessToken)
	}
	return nil, errAuthNotImplemented()
}

func (m *AuthServiceMock) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	if m.StoreOAuthStateFn != nil {
		return m.StoreOAuthStateFn(ctx, state, expiresAt)
	}
	return nil
}

func (m *AuthServiceMock) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	if m.ConsumeOAuthStateFn != nil {
		return m.ConsumeOAuthStateFn(ctx, state)
	}
	return true, nil
}

func (m *AuthServiceMock) CleanupExpiredOAuthStates(ctx context.Context) (int, error) {
	if m.CleanupExpiredOAuthStatesFn != nil {
		return m.CleanupExpiredOAuthStatesFn(ctx)
	}
	return 0, nil
}

func (m *AuthServiceMock) CreateSession(ctx context.Context, userID int64, userAgent, ipAddress string) (*auth.Session, error) {
	if m.CreateSessionFn != nil {
		return m.CreateSessionFn(ctx, userID, userAgent, ipAddress)
	}
	return nil, errAuthNotImplemented()
}

func (m *AuthServiceMock) ValidateSession(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
	if m.ValidateSessionFn != nil {
		return m.ValidateSessionFn(ctx, sessionID)
	}
	return nil, nil, errAuthNotImplemented()
}

func (m *AuthServiceMock) DeleteSession(ctx context.Context, sessionID string) error {
	if m.DeleteSessionFn != nil {
		return m.DeleteSessionFn(ctx, sessionID)
	}
	return nil
}

func (m *AuthServiceMock) CleanupExpiredSessions(ctx context.Context) (int, error) {
	if m.CleanupExpiredSessionsFn != nil {
		return m.CleanupExpiredSessionsFn(ctx)
	}
	return 0, nil
}

func (m *AuthServiceMock) GetOrCreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	if m.GetOrCreateUserFn != nil {
		return m.GetOrCreateUserFn(ctx, googleID, username, email, avatarURL)
	}
	return nil, errAuthNotImplemented()
}

func (m *AuthServiceMock) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	if m.GetUserByIDFn != nil {
		return m.GetUserByIDFn(ctx, userID)
	}
	return nil, errAuthNotImplemented()
}

func (m *AuthServiceMock) UpdateLastLogin(ctx context.Context, userID int64) error {
	if m.UpdateLastLoginFn != nil {
		return m.UpdateLastLoginFn(ctx, userID)
	}
	return nil
}

func (m *AuthServiceMock) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	if m.StoreTokensFn != nil {
		return m.StoreTokensFn(ctx, userID, sessionID, tokens)
	}
	return nil
}

func (m *AuthServiceMock) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	if m.IsEmailAllowedFn != nil {
		return m.IsEmailAllowedFn(ctx, email)
	}
	return false, nil
}

func (m *AuthServiceMock) ListAllowedEmails(ctx context.Context) ([]auth.AllowedEmail, error) {
	if m.ListAllowedEmailsFn != nil {
		return m.ListAllowedEmailsFn(ctx)
	}
	return nil, nil
}

func (m *AuthServiceMock) AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error {
	if m.AddAllowedEmailFn != nil {
		return m.AddAllowedEmailFn(ctx, email, addedBy, notes)
	}
	return nil
}

func (m *AuthServiceMock) RemoveAllowedEmail(ctx context.Context, email string) error {
	if m.RemoveAllowedEmailFn != nil {
		return m.RemoveAllowedEmailFn(ctx, email)
	}
	return nil
}

func (m *AuthServiceMock) ListUsers(ctx context.Context) ([]auth.User, error) {
	if m.ListUsersFn != nil {
		return m.ListUsersFn(ctx)
	}
	return nil, nil
}

func (m *AuthServiceMock) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	if m.SetUserAdminFn != nil {
		return m.SetUserAdminFn(ctx, userID, isAdmin)
	}
	return nil
}
