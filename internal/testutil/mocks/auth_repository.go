package mocks

import (
	"context"
	"errors"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
)

// MockAuthRepository is a test double for auth.Repository.
// Override any Fn field to customize behavior per-test.
//
// Example:
//
//	repo := NewMockAuthRepository()
//	repo.CreateUserFn = func(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
//	    return &auth.User{ID: 1, GoogleID: googleID, Email: email}, nil
//	}
type MockAuthRepository struct {
	Users    map[string]*auth.User
	Tokens   map[int64]*auth.UserTokens
	Sessions map[string]*auth.Session

	CreateUserFn                func(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error)
	GetUserByGoogleIDFn         func(ctx context.Context, googleID string) (*auth.User, error)
	GetUserByIDFn               func(ctx context.Context, userID int64) (*auth.User, error)
	UpdateUserFn                func(ctx context.Context, user *auth.User) error
	StoreTokensFn               func(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error
	GetTokensFn                 func(ctx context.Context, userID int64, sessionID string) (*auth.UserTokens, error)
	GetTokensByUserIDFn         func(ctx context.Context, userID int64) (*auth.UserTokens, error)
	UpdateTokensFn              func(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error
	DeleteTokensFn              func(ctx context.Context, userID int64, sessionID string) error
	DeleteAllUserTokensFn       func(ctx context.Context, userID int64) error
	CreateSessionFn             func(ctx context.Context, session *auth.Session) error
	GetSessionFn                func(ctx context.Context, sessionID string) (*auth.Session, error)
	UpdateSessionAccessFn       func(ctx context.Context, sessionID string) error
	DeleteSessionFn             func(ctx context.Context, sessionID string) error
	DeleteExpiredSessionsFn     func(ctx context.Context) (int, error)
	StoreOAuthStateFn           func(ctx context.Context, state string, expiresAt time.Time) error
	ConsumeOAuthStateFn         func(ctx context.Context, state string) (bool, error)
	CleanupExpiredOAuthStatesFn func(ctx context.Context) (int, error)
	IsEmailAllowedFn            func(ctx context.Context, email string) (bool, error)
	ListAllowedEmailsFn         func(ctx context.Context) ([]auth.AllowedEmail, error)
	AddAllowedEmailFn           func(ctx context.Context, email string, addedBy int64, notes string) error
	RemoveAllowedEmailFn        func(ctx context.Context, email string) error
	ListUsersFn                 func(ctx context.Context) ([]auth.User, error)
	SetUserAdminFn              func(ctx context.Context, userID int64, isAdmin bool) error
}

var _ auth.Repository = (*MockAuthRepository)(nil)

func NewMockAuthRepository() *MockAuthRepository {
	return &MockAuthRepository{
		Users:    make(map[string]*auth.User),
		Tokens:   make(map[int64]*auth.UserTokens),
		Sessions: make(map[string]*auth.Session),
	}
}

func (m *MockAuthRepository) CreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	if m.CreateUserFn != nil {
		return m.CreateUserFn(ctx, googleID, username, email, avatarURL)
	}
	user := &auth.User{
		ID:        int64(len(m.Users) + 1),
		GoogleID:  googleID,
		Username:  username,
		Email:     email,
		AvatarURL: avatarURL,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.Users[googleID] = user
	return user, nil
}

func (m *MockAuthRepository) GetUserByGoogleID(ctx context.Context, googleID string) (*auth.User, error) {
	if m.GetUserByGoogleIDFn != nil {
		return m.GetUserByGoogleIDFn(ctx, googleID)
	}
	user, ok := m.Users[googleID]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return user, nil
}

func (m *MockAuthRepository) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	if m.GetUserByIDFn != nil {
		return m.GetUserByIDFn(ctx, userID)
	}
	for _, user := range m.Users {
		if user.ID == userID {
			return user, nil
		}
	}
	return nil, auth.ErrUserNotFound
}

func (m *MockAuthRepository) UpdateUser(ctx context.Context, user *auth.User) error {
	if m.UpdateUserFn != nil {
		return m.UpdateUserFn(ctx, user)
	}
	m.Users[user.GoogleID] = user
	return nil
}

func (m *MockAuthRepository) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	if m.StoreTokensFn != nil {
		return m.StoreTokensFn(ctx, userID, sessionID, tokens)
	}
	m.Tokens[userID] = tokens
	return nil
}

func (m *MockAuthRepository) GetTokens(ctx context.Context, userID int64, sessionID string) (*auth.UserTokens, error) {
	if m.GetTokensFn != nil {
		return m.GetTokensFn(ctx, userID, sessionID)
	}
	tokens, ok := m.Tokens[userID]
	if !ok {
		return nil, errors.New("tokens not found")
	}
	return tokens, nil
}

func (m *MockAuthRepository) GetTokensByUserID(ctx context.Context, userID int64) (*auth.UserTokens, error) {
	if m.GetTokensByUserIDFn != nil {
		return m.GetTokensByUserIDFn(ctx, userID)
	}
	tokens, ok := m.Tokens[userID]
	if !ok {
		return nil, errors.New("tokens not found")
	}
	return tokens, nil
}

func (m *MockAuthRepository) UpdateTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	if m.UpdateTokensFn != nil {
		return m.UpdateTokensFn(ctx, userID, sessionID, tokens)
	}
	m.Tokens[userID] = tokens
	return nil
}

func (m *MockAuthRepository) DeleteTokens(ctx context.Context, userID int64, sessionID string) error {
	if m.DeleteTokensFn != nil {
		return m.DeleteTokensFn(ctx, userID, sessionID)
	}
	delete(m.Tokens, userID)
	return nil
}

func (m *MockAuthRepository) DeleteAllUserTokens(ctx context.Context, userID int64) error {
	if m.DeleteAllUserTokensFn != nil {
		return m.DeleteAllUserTokensFn(ctx, userID)
	}
	delete(m.Tokens, userID)
	return nil
}

func (m *MockAuthRepository) CreateSession(ctx context.Context, session *auth.Session) error {
	if m.CreateSessionFn != nil {
		return m.CreateSessionFn(ctx, session)
	}
	m.Sessions[session.ID] = session
	return nil
}

func (m *MockAuthRepository) GetSession(ctx context.Context, sessionID string) (*auth.Session, error) {
	if m.GetSessionFn != nil {
		return m.GetSessionFn(ctx, sessionID)
	}
	session, ok := m.Sessions[sessionID]
	if !ok {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (m *MockAuthRepository) UpdateSessionAccess(ctx context.Context, sessionID string) error {
	if m.UpdateSessionAccessFn != nil {
		return m.UpdateSessionAccessFn(ctx, sessionID)
	}
	if session, ok := m.Sessions[sessionID]; ok {
		session.LastAccessedAt = time.Now()
	}
	return nil
}

func (m *MockAuthRepository) DeleteSession(ctx context.Context, sessionID string) error {
	if m.DeleteSessionFn != nil {
		return m.DeleteSessionFn(ctx, sessionID)
	}
	delete(m.Sessions, sessionID)
	return nil
}

func (m *MockAuthRepository) DeleteExpiredSessions(ctx context.Context) (int, error) {
	if m.DeleteExpiredSessionsFn != nil {
		return m.DeleteExpiredSessionsFn(ctx)
	}
	count := 0
	now := time.Now()
	for id, session := range m.Sessions {
		if session.ExpiresAt.Before(now) {
			delete(m.Sessions, id)
			count++
		}
	}
	return count, nil
}

func (m *MockAuthRepository) StoreOAuthState(_ context.Context, _ string, _ time.Time) error {
	if m.StoreOAuthStateFn != nil {
		return m.StoreOAuthStateFn(context.Background(), "", time.Time{})
	}
	return nil
}

func (m *MockAuthRepository) ConsumeOAuthState(_ context.Context, state string) (bool, error) {
	if m.ConsumeOAuthStateFn != nil {
		return m.ConsumeOAuthStateFn(context.Background(), state)
	}
	return true, nil
}

func (m *MockAuthRepository) CleanupExpiredOAuthStates(_ context.Context) (int, error) {
	if m.CleanupExpiredOAuthStatesFn != nil {
		return m.CleanupExpiredOAuthStatesFn(context.Background())
	}
	return 0, nil
}

func (m *MockAuthRepository) IsEmailAllowed(_ context.Context, _ string) (bool, error) {
	if m.IsEmailAllowedFn != nil {
		return m.IsEmailAllowedFn(context.Background(), "")
	}
	return false, nil
}

func (m *MockAuthRepository) ListAllowedEmails(_ context.Context) ([]auth.AllowedEmail, error) {
	if m.ListAllowedEmailsFn != nil {
		return m.ListAllowedEmailsFn(context.Background())
	}
	return nil, nil
}

func (m *MockAuthRepository) AddAllowedEmail(_ context.Context, _ string, _ int64, _ string) error {
	if m.AddAllowedEmailFn != nil {
		return m.AddAllowedEmailFn(context.Background(), "", 0, "")
	}
	return nil
}

func (m *MockAuthRepository) RemoveAllowedEmail(_ context.Context, _ string) error {
	if m.RemoveAllowedEmailFn != nil {
		return m.RemoveAllowedEmailFn(context.Background(), "")
	}
	return nil
}

func (m *MockAuthRepository) ListUsers(_ context.Context) ([]auth.User, error) {
	if m.ListUsersFn != nil {
		return m.ListUsersFn(context.Background())
	}
	return nil, nil
}

func (m *MockAuthRepository) SetUserAdmin(_ context.Context, _ int64, _ bool) error {
	if m.SetUserAdminFn != nil {
		return m.SetUserAdminFn(context.Background(), 0, false)
	}
	return nil
}
