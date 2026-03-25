package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	appErrs "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// requireCookie finds a cookie by name and fails the test if not found.
func requireCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("expected cookie %q to be set", name)
	return nil // unreachable
}

// errNotImplemented returns a standardized internal error for mock methods
// that are not implemented. This uses the project's internal error type
// instead of the stdlib error.
func errNotImplemented() error {
	return appErrs.NewAppError(appErrs.ErrCodeInternal, "not implemented")
}

// Compile-time interface check
var _ auth.Service = (*mockAuthService)(nil)

// mockAuthService implements auth.Service for testing.
type mockAuthService struct {
	getLoginURLFunc            func(state string) string
	exchangeCodeFunc           func(ctx context.Context, code string) (*auth.UserTokens, error)
	getUserInfoFunc            func(ctx context.Context, accessToken string) (*auth.UserInfo, error)
	storeOAuthStateFunc        func(ctx context.Context, state string, expiresAt time.Time) error
	consumeOAuthStateFunc      func(ctx context.Context, state string) (bool, error)
	createSessionFunc          func(ctx context.Context, userID int64, userAgent, ipAddress string) (*auth.Session, error)
	validateSessionFunc        func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error)
	deleteSessionFunc          func(ctx context.Context, sessionID string) error
	cleanupExpiredSessionsFunc func(ctx context.Context) (int, error)
	getOrCreateUserFunc        func(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error)
	getUserByIDFunc            func(ctx context.Context, userID int64) (*auth.User, error)
	updateLastLoginFunc        func(ctx context.Context, userID int64) error
	storeTokensFunc            func(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error
	isEmailAllowedFunc         func(ctx context.Context, email string) (bool, error)
	listAllowedEmailsFunc      func(ctx context.Context) ([]auth.AllowedEmail, error)
	addAllowedEmailFunc        func(ctx context.Context, email string, addedBy int64, notes string) error
	removeAllowedEmailFunc     func(ctx context.Context, email string) error
	listUsersFunc              func(ctx context.Context) ([]auth.User, error)
	setUserAdminFunc           func(ctx context.Context, userID int64, isAdmin bool) error
}

func (m *mockAuthService) GetLoginURL(state string) string {
	if m.getLoginURLFunc != nil {
		return m.getLoginURLFunc(state)
	}
	return "https://accounts.google.com/o/oauth2/v2/auth?state=" + state
}

func (m *mockAuthService) ExchangeCodeForTokens(ctx context.Context, code string) (*auth.UserTokens, error) {
	if m.exchangeCodeFunc != nil {
		return m.exchangeCodeFunc(ctx, code)
	}
	return nil, errNotImplemented()
}

func (m *mockAuthService) GetUserInfo(ctx context.Context, accessToken string) (*auth.UserInfo, error) {
	if m.getUserInfoFunc != nil {
		return m.getUserInfoFunc(ctx, accessToken)
	}
	return nil, errNotImplemented()
}

func (m *mockAuthService) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	if m.storeOAuthStateFunc != nil {
		return m.storeOAuthStateFunc(ctx, state, expiresAt)
	}
	return nil
}

func (m *mockAuthService) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	if m.consumeOAuthStateFunc != nil {
		return m.consumeOAuthStateFunc(ctx, state)
	}
	return true, nil
}

func (m *mockAuthService) CreateSession(ctx context.Context, userID int64, userAgent, ipAddress string) (*auth.Session, error) {
	if m.createSessionFunc != nil {
		return m.createSessionFunc(ctx, userID, userAgent, ipAddress)
	}
	return nil, errNotImplemented()
}

func (m *mockAuthService) ValidateSession(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
	if m.validateSessionFunc != nil {
		return m.validateSessionFunc(ctx, sessionID)
	}
	return nil, nil, errNotImplemented()
}

func (m *mockAuthService) DeleteSession(ctx context.Context, sessionID string) error {
	if m.deleteSessionFunc != nil {
		return m.deleteSessionFunc(ctx, sessionID)
	}
	return nil
}

func (m *mockAuthService) CleanupExpiredSessions(ctx context.Context) (int, error) {
	if m.cleanupExpiredSessionsFunc != nil {
		return m.cleanupExpiredSessionsFunc(ctx)
	}
	return 0, nil
}

func (m *mockAuthService) GetOrCreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	if m.getOrCreateUserFunc != nil {
		return m.getOrCreateUserFunc(ctx, googleID, username, email, avatarURL)
	}
	return nil, errNotImplemented()
}

func (m *mockAuthService) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, userID)
	}
	return nil, errNotImplemented()
}

func (m *mockAuthService) UpdateLastLogin(ctx context.Context, userID int64) error {
	if m.updateLastLoginFunc != nil {
		return m.updateLastLoginFunc(ctx, userID)
	}
	return nil
}

func (m *mockAuthService) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	if m.storeTokensFunc != nil {
		return m.storeTokensFunc(ctx, userID, sessionID, tokens)
	}
	return nil
}

func (m *mockAuthService) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	if m.isEmailAllowedFunc != nil {
		return m.isEmailAllowedFunc(ctx, email)
	}
	return false, nil
}

func (m *mockAuthService) ListAllowedEmails(ctx context.Context) ([]auth.AllowedEmail, error) {
	if m.listAllowedEmailsFunc != nil {
		return m.listAllowedEmailsFunc(ctx)
	}
	return nil, nil
}

func (m *mockAuthService) AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error {
	if m.addAllowedEmailFunc != nil {
		return m.addAllowedEmailFunc(ctx, email, addedBy, notes)
	}
	return nil
}

func (m *mockAuthService) RemoveAllowedEmail(ctx context.Context, email string) error {
	if m.removeAllowedEmailFunc != nil {
		return m.removeAllowedEmailFunc(ctx, email)
	}
	return nil
}

func (m *mockAuthService) ListUsers(ctx context.Context) ([]auth.User, error) {
	if m.listUsersFunc != nil {
		return m.listUsersFunc(ctx)
	}
	return nil, nil
}

func (m *mockAuthService) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	if m.setUserAdminFunc != nil {
		return m.setUserAdminFunc(ctx, userID, isAdmin)
	}
	return nil
}

func TestNewAuthHandlers(t *testing.T) {
	service := &mockAuthService{}
	logger := mocks.NewMockLogger()

	handlers := NewAuthHandlers(service, logger, false, nil)
	if handlers == nil {
		t.Fatal("Expected non-nil handlers")
	}
}

func TestHandleGoogleLogin(t *testing.T) {
	storeStateCalled := false
	service := &mockAuthService{
		getLoginURLFunc: func(state string) string {
			return "https://accounts.google.com/test?state=" + state
		},
		storeOAuthStateFunc: func(ctx context.Context, state string, expiresAt time.Time) error {
			storeStateCalled = true
			if state == "" {
				t.Error("StoreOAuthState called with empty state")
			}
			if expiresAt.Before(time.Now()) {
				t.Error("StoreOAuthState called with past expiry")
			}
			return nil
		},
	}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	w := httptest.NewRecorder()

	handlers.HandleGoogleLogin(w, req)

	// Check redirect
	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", w.Code)
	}

	if !storeStateCalled {
		t.Error("Expected StoreOAuthState to be called")
	}

	// Check Location header
	location := w.Header().Get("Location")
	if location == "" {
		t.Error("Expected Location header")
	}

	// Check state cookie was set
	stateCookie := requireCookie(t, w.Result().Cookies(), stateCookieName)

	if !stateCookie.HttpOnly {
		t.Error("State cookie should be HttpOnly")
	}
}

func TestHandleGoogleLogin_StoreStateError(t *testing.T) {
	service := &mockAuthService{
		storeOAuthStateFunc: func(ctx context.Context, state string, expiresAt time.Time) error {
			return appErrs.NewAppError(appErrs.ErrCodeInternal, "db write failed")
		},
	}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	w := httptest.NewRecorder()

	handlers.HandleGoogleLogin(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 when StoreOAuthState fails, got %d", w.Code)
	}
}

func TestHandleGoogleCallback_MissingState(t *testing.T) {
	service := &mockAuthService{}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test", nil)
	w := httptest.NewRecorder()

	handlers.HandleGoogleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleGoogleCallback_StateMismatch(t *testing.T) {
	service := &mockAuthService{}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test&state=wrong", nil)
	req.AddCookie(&http.Cookie{
		Name:  stateCookieName,
		Value: "correct-state",
	})
	w := httptest.NewRecorder()

	handlers.HandleGoogleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleGoogleCallback_ConsumeStateError(t *testing.T) {
	service := &mockAuthService{
		consumeOAuthStateFunc: func(ctx context.Context, state string) (bool, error) {
			return false, appErrs.NewAppError(appErrs.ErrCodeInternal, "db error")
		},
	}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test&state=valid-state", nil)
	req.AddCookie(&http.Cookie{
		Name:  stateCookieName,
		Value: "valid-state",
	})
	w := httptest.NewRecorder()

	handlers.HandleGoogleCallback(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 when ConsumeOAuthState errors, got %d", w.Code)
	}
}

func TestHandleGoogleCallback_ExpiredState(t *testing.T) {
	consumeCalled := false
	service := &mockAuthService{
		consumeOAuthStateFunc: func(ctx context.Context, state string) (bool, error) {
			consumeCalled = true
			if state != "expired-state" {
				t.Errorf("Expected state %q, got %q", "expired-state", state)
			}
			return false, nil
		},
	}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test&state=expired-state", nil)
	req.AddCookie(&http.Cookie{
		Name:  stateCookieName,
		Value: "expired-state",
	})
	w := httptest.NewRecorder()

	handlers.HandleGoogleCallback(w, req)

	if !consumeCalled {
		t.Error("Expected ConsumeOAuthState to be called")
	}

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for expired/invalid state, got %d", w.Code)
	}
}

func TestHandleLogout(t *testing.T) {
	sessionDeleted := false
	service := &mockAuthService{
		deleteSessionFunc: func(ctx context.Context, sessionID string) error {
			if sessionID == "test-session" {
				sessionDeleted = true
				return nil
			}
			return appErrs.NewAppError(appErrs.ErrCodeInternal, "session not found")
		},
	}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	tests := []struct {
		name          string
		sessionCookie *http.Cookie
		wantStatus    int
		wantDeleted   bool
	}{
		{
			name: "logout with valid session",
			sessionCookie: &http.Cookie{
				Name:  middleware.SessionCookieName,
				Value: "test-session",
			},
			wantStatus:  http.StatusOK,
			wantDeleted: true,
		},
		{
			name:        "logout without session",
			wantStatus:  http.StatusOK,
			wantDeleted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionDeleted = false

			req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
			if tt.sessionCookie != nil {
				req.AddCookie(tt.sessionCookie)
			}
			w := httptest.NewRecorder()

			handlers.HandleLogout(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if sessionDeleted != tt.wantDeleted {
				t.Errorf("Expected sessionDeleted=%v, got %v", tt.wantDeleted, sessionDeleted)
			}

			// Check that session cookie is cleared
			cookies := w.Result().Cookies()
			for _, c := range cookies {
				if c.Name == middleware.SessionCookieName && c.MaxAge != -1 {
					t.Error("Session cookie should be cleared (MaxAge=-1)")
				}
			}
		})
	}
}

func TestHandleGetCurrentUser_Unauthorized(t *testing.T) {
	service := &mockAuthService{}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/user", nil)
	w := httptest.NewRecorder()

	handlers.HandleGetCurrentUser(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestHandleGetCurrentUser_Success(t *testing.T) {
	service := &mockAuthService{}
	logger := mocks.NewMockLogger()
	handlers := NewAuthHandlers(service, logger, false, nil)

	// Create request with user in context
	req := httptest.NewRequest(http.MethodGet, "/api/auth/user", nil)

	user := &auth.User{
		ID:        1,
		GoogleID:  "test-google-123",
		Username:  "testuser",
		Email:     "test@example.com",
		AvatarURL: "https://example.com/avatar.jpg",
	}

	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handlers.HandleGetCurrentUser(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check Content-Type
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", ct)
	}
}

func TestGenerateState(t *testing.T) {
	state1, err1 := auth.GenerateState()
	if err1 != nil {
		t.Errorf("GenerateState() error = %v", err1)
	}

	state2, err2 := auth.GenerateState()
	if err2 != nil {
		t.Errorf("GenerateState() error = %v", err2)
	}

	if state1 == state2 {
		t.Error("GenerateState() should produce unique values")
	}

	if len(state1) < 40 {
		t.Errorf("State too short: %d characters", len(state1))
	}
}

func TestSecureCookies(t *testing.T) {
	service := &mockAuthService{
		getLoginURLFunc: func(state string) string {
			return "https://accounts.google.com/test"
		},
	}
	logger := mocks.NewMockLogger()

	tests := []struct {
		name          string
		secureCookies bool
		wantSecure    bool
	}{
		{
			name:          "production mode - secure cookies",
			secureCookies: true,
			wantSecure:    true,
		},
		{
			name:          "dev mode - insecure cookies",
			secureCookies: false,
			wantSecure:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlers := NewAuthHandlers(service, logger, tt.secureCookies, nil)

			req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
			w := httptest.NewRecorder()

			handlers.HandleGoogleLogin(w, req)

			stateCookie := requireCookie(t, w.Result().Cookies(), stateCookieName)

			if stateCookie.Secure != tt.wantSecure {
				t.Errorf("Expected Secure=%v, got %v", tt.wantSecure, stateCookie.Secure)
			}
		})
	}
}
