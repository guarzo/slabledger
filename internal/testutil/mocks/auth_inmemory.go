package mocks

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// InMemoryAuthRepository is a fully-stateful in-memory implementation of auth.Repository
// for use in service-level unit tests. All operations are safe for concurrent use.
//
// Example:
//
//	repo := mocks.NewInMemoryAuthRepository()
//	svc := auth.New(repo, func(state string) string { return "https://example.com/auth?state=" + state })
type InMemoryAuthRepository struct {
	mu sync.Mutex

	users    map[int64]*auth.User
	byGoogle map[string]int64 // googleID → userID

	sessions map[string]*auth.Session

	states  map[string]time.Time // state token → expiresAt
	allowed map[string]*auth.AllowedEmail
	tokens  map[tokenKey]*auth.UserTokens

	nextID atomic.Int64
}

type tokenKey struct {
	userID    int64
	sessionID string
}

// NewInMemoryAuthRepository returns a zeroed, ready-to-use repository.
func NewInMemoryAuthRepository() *InMemoryAuthRepository {
	r := &InMemoryAuthRepository{
		users:    make(map[int64]*auth.User),
		byGoogle: make(map[string]int64),
		sessions: make(map[string]*auth.Session),
		states:   make(map[string]time.Time),
		allowed:  make(map[string]*auth.AllowedEmail),
		tokens:   make(map[tokenKey]*auth.UserTokens),
	}
	r.nextID.Store(0)
	return r
}

var _ auth.Repository = (*InMemoryAuthRepository)(nil)

// ─── Users ────────────────────────────────────────────────────────────────────

func (r *InMemoryAuthRepository) CreateUser(_ context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextID.Add(1)
	now := time.Now()
	u := &auth.User{
		ID:        id,
		GoogleID:  googleID,
		Username:  username,
		Email:     email,
		AvatarURL: avatarURL,
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.users[id] = u
	r.byGoogle[googleID] = id
	cp := *u
	return &cp, nil
}

func (r *InMemoryAuthRepository) GetUserByGoogleID(_ context.Context, googleID string) (*auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, ok := r.byGoogle[googleID]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	u := *r.users[id] // return a copy
	return &u, nil
}

func (r *InMemoryAuthRepository) GetUserByID(_ context.Context, userID int64) (*auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.users[userID]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *InMemoryAuthRepository) UpdateUser(_ context.Context, user *auth.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prev, ok := r.users[user.ID]
	if !ok {
		return auth.ErrUserNotFound
	}
	// Remove the old Google ID mapping if it changed.
	if prev.GoogleID != user.GoogleID {
		delete(r.byGoogle, prev.GoogleID)
	}
	cp := *user
	cp.UpdatedAt = time.Now()
	r.users[user.ID] = &cp
	r.byGoogle[user.GoogleID] = user.ID
	return nil
}

// ─── Tokens ───────────────────────────────────────────────────────────────────

func (r *InMemoryAuthRepository) StoreTokens(_ context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *tokens
	r.tokens[tokenKey{userID, sessionID}] = &cp
	return nil
}

func (r *InMemoryAuthRepository) GetTokens(_ context.Context, userID int64, sessionID string) (*auth.UserTokens, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tokens[tokenKey{userID, sessionID}]
	if !ok {
		return nil, apperrors.NewAppError("ERR_TOKENS_NOT_FOUND", "tokens not found")
	}
	cp := *t
	return &cp, nil
}

func (r *InMemoryAuthRepository) GetTokensByUserID(_ context.Context, userID int64) (*auth.UserTokens, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var latest *auth.UserTokens
	for k, t := range r.tokens {
		if k.userID != userID {
			continue
		}
		if latest == nil || t.ExpiresAt.After(latest.ExpiresAt) {
			latest = t
		}
	}
	if latest == nil {
		return nil, apperrors.NewAppError("ERR_TOKENS_NOT_FOUND", "tokens not found")
	}
	cp := *latest
	return &cp, nil
}

func (r *InMemoryAuthRepository) UpdateTokens(_ context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *tokens
	r.tokens[tokenKey{userID, sessionID}] = &cp
	return nil
}

func (r *InMemoryAuthRepository) DeleteTokens(_ context.Context, userID int64, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tokens, tokenKey{userID, sessionID})
	return nil
}

func (r *InMemoryAuthRepository) DeleteAllUserTokens(_ context.Context, userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.tokens {
		if k.userID == userID {
			delete(r.tokens, k)
		}
	}
	return nil
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

func (r *InMemoryAuthRepository) CreateSession(_ context.Context, session *auth.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *session
	r.sessions[session.ID] = &cp
	return nil
}

func (r *InMemoryAuthRepository) GetSession(_ context.Context, sessionID string) (*auth.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[sessionID]
	if !ok {
		return nil, apperrors.NewAppError("ERR_SESSION_NOT_FOUND", "session not found")
	}
	cp := *s
	return &cp, nil
}

func (r *InMemoryAuthRepository) UpdateSessionAccess(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[sessionID]
	if !ok {
		return nil
	}
	s.LastAccessedAt = time.Now()
	return nil
}

func (r *InMemoryAuthRepository) DeleteSession(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, sessionID)
	return nil
}

func (r *InMemoryAuthRepository) DeleteExpiredSessions(_ context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	count := 0
	for id, s := range r.sessions {
		if s.ExpiresAt.Before(now) {
			delete(r.sessions, id)
			count++
		}
	}
	return count, nil
}

// ─── OAuth State ──────────────────────────────────────────────────────────────

func (r *InMemoryAuthRepository) StoreOAuthState(_ context.Context, state string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[state] = expiresAt
	return nil
}

func (r *InMemoryAuthRepository) ConsumeOAuthState(_ context.Context, state string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	exp, ok := r.states[state]
	if !ok {
		return false, nil
	}
	delete(r.states, state)
	if exp.Before(time.Now()) {
		return false, nil
	}
	return true, nil
}

func (r *InMemoryAuthRepository) CleanupExpiredOAuthStates(_ context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	count := 0
	for s, exp := range r.states {
		if exp.Before(now) {
			delete(r.states, s)
			count++
		}
	}
	return count, nil
}

// ─── Email Allowlist ──────────────────────────────────────────────────────────

func (r *InMemoryAuthRepository) IsEmailAllowed(_ context.Context, email string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.allowed[email]
	return ok, nil
}

func (r *InMemoryAuthRepository) ListAllowedEmails(_ context.Context) ([]auth.AllowedEmail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]auth.AllowedEmail, 0, len(r.allowed))
	for _, e := range r.allowed {
		out = append(out, *e)
	}
	return out, nil
}

func (r *InMemoryAuthRepository) AddAllowedEmail(_ context.Context, email string, addedBy int64, notes string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := addedBy
	e := &auth.AllowedEmail{
		Email:     email,
		AddedBy:   &id,
		CreatedAt: time.Now(),
		Notes:     notes,
	}
	r.allowed[email] = e
	return nil
}

func (r *InMemoryAuthRepository) RemoveAllowedEmail(_ context.Context, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.allowed, email)
	return nil
}

// ─── Admin / User Listing ─────────────────────────────────────────────────────

func (r *InMemoryAuthRepository) ListUsers(_ context.Context) ([]auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]auth.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, *u)
	}
	return out, nil
}

func (r *InMemoryAuthRepository) SetUserAdmin(_ context.Context, userID int64, isAdmin bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.users[userID]
	if !ok {
		return auth.ErrUserNotFound
	}
	u.IsAdmin = isAdmin
	return nil
}
