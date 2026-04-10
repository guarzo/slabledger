package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Mock auth service for testing
type mockAuthService struct {
	validateSessionFunc    func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error)
	getOrCreateUserFunc    func(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error)
	isEmailAllowedFunc     func(ctx context.Context, email string) (bool, error)
	listAllowedEmailsFunc  func(ctx context.Context) ([]auth.AllowedEmail, error)
	addAllowedEmailFunc    func(ctx context.Context, email string, addedBy int64, notes string) error
	removeAllowedEmailFunc func(ctx context.Context, email string) error
	listUsersFunc          func(ctx context.Context) ([]auth.User, error)
	setUserAdminFunc       func(ctx context.Context, userID int64, isAdmin bool) error
}

func (m *mockAuthService) GetLoginURL(state string) string {
	return ""
}

func (m *mockAuthService) ExchangeCodeForTokens(ctx context.Context, code string) (*auth.UserTokens, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthService) GetUserInfo(ctx context.Context, accessToken string) (*auth.UserInfo, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthService) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	return nil
}

func (m *mockAuthService) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	return true, nil
}

func (m *mockAuthService) CreateSession(ctx context.Context, userID int64, userAgent, ipAddress string) (*auth.Session, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthService) ValidateSession(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
	if m.validateSessionFunc != nil {
		return m.validateSessionFunc(ctx, sessionID)
	}
	return nil, nil, errors.New("not implemented")
}

func (m *mockAuthService) DeleteSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *mockAuthService) CleanupExpiredSessions(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockAuthService) GetOrCreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	if m.getOrCreateUserFunc != nil {
		return m.getOrCreateUserFunc(ctx, googleID, username, email, avatarURL)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAuthService) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	return nil
}

func (m *mockAuthService) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthService) UpdateLastLogin(ctx context.Context, userID int64) error {
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

// Mock logger for auth testing
type mockAuthLogger struct{}

func (m *mockAuthLogger) Debug(ctx context.Context, msg string, fields ...observability.Field) {}
func (m *mockAuthLogger) Info(ctx context.Context, msg string, fields ...observability.Field)  {}
func (m *mockAuthLogger) Warn(ctx context.Context, msg string, fields ...observability.Field)  {}
func (m *mockAuthLogger) Error(ctx context.Context, msg string, fields ...observability.Field) {}
func (m *mockAuthLogger) With(ctx context.Context, fields ...observability.Field) observability.Logger {
	return m
}

func TestNewAuthMiddleware(t *testing.T) {
	service := &mockAuthService{}
	logger := &mockAuthLogger{}

	mw := NewAuthMiddleware(service, logger)
	if mw == nil {
		t.Fatal("Expected non-nil middleware")
	}
}

func TestRequireAuth_NoSession(t *testing.T) {
	service := &mockAuthService{}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("protected"))
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()

	mw.RequireAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestRequireAuth_InvalidSession(t *testing.T) {
	service := &mockAuthService{
		validateSessionFunc: func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
			return nil, nil, errors.New("invalid session")
		},
	}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: "invalid-session",
	})
	w := httptest.NewRecorder()

	mw.RequireAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestRequireAuth_ValidSession(t *testing.T) {
	testUser := &auth.User{
		ID:        1,
		GoogleID:  "google-123",
		Username:  "testuser",
		Email:     "test@example.com",
		AvatarURL: "https://example.com/avatar.png",
	}

	service := &mockAuthService{
		validateSessionFunc: func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
			if sessionID == "valid-session" {
				return &auth.Session{
					ID:        sessionID,
					UserID:    1,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}, testUser, nil
			}
			return nil, nil, errors.New("invalid session")
		},
	}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)

	handlerCalled := false
	var contextUser *auth.User

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// Extract user from context
		user, ok := GetUserFromContext(r.Context())
		if ok {
			contextUser = user
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: "valid-session",
	})
	w := httptest.NewRecorder()

	mw.RequireAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !handlerCalled {
		t.Error("Expected handler to be called")
	}

	if contextUser == nil {
		t.Error("Expected user in context")
	} else {
		if contextUser.ID != testUser.ID {
			t.Errorf("Expected user ID %d, got %d", testUser.ID, contextUser.ID)
		}
	}
}

func TestOptionalAuth_NoSession(t *testing.T) {
	service := &mockAuthService{}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	w := httptest.NewRecorder()

	mw.OptionalAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !handlerCalled {
		t.Error("Expected handler to be called even without session")
	}
}

func TestOptionalAuth_ValidSession(t *testing.T) {
	testUser := &auth.User{
		ID:        1,
		GoogleID:  "google-123",
		Username:  "testuser",
		AvatarURL: "https://example.com/avatar.png",
	}

	service := &mockAuthService{
		validateSessionFunc: func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
			if sessionID == "valid-session" {
				return &auth.Session{
					ID:     sessionID,
					UserID: 1,
				}, testUser, nil
			}
			return nil, nil, errors.New("invalid")
		},
	}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)

	var contextUser *auth.User
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if ok {
			contextUser = user
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: "valid-session",
	})
	w := httptest.NewRecorder()

	mw.OptionalAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if contextUser == nil {
		t.Error("Expected user in context")
	}
}

func TestOptionalAuth_InvalidSession(t *testing.T) {
	service := &mockAuthService{
		validateSessionFunc: func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
			return nil, nil, errors.New("invalid session")
		},
	}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)

	handlerCalled := false
	var contextUser *auth.User

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		user, ok := GetUserFromContext(r.Context())
		if ok {
			contextUser = user
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: "invalid-session",
	})
	w := httptest.NewRecorder()

	mw.OptionalAuth(handler).ServeHTTP(w, req)

	// Should still call handler even with invalid session
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !handlerCalled {
		t.Error("Expected handler to be called even with invalid session")
	}

	if contextUser != nil {
		t.Error("Expected no user in context with invalid session")
	}
}

func TestGetUserFromContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		wantUser bool
		wantOK   bool
	}{
		{
			name: "user in context",
			ctx: context.WithValue(context.Background(), UserContextKey, &auth.User{
				ID:       1,
				Username: "testuser",
			}),
			wantUser: true,
			wantOK:   true,
		},
		{
			name:     "no user in context",
			ctx:      context.Background(),
			wantUser: false,
			wantOK:   false,
		},
		{
			name:     "wrong type in context",
			ctx:      context.WithValue(context.Background(), UserContextKey, "not a user"),
			wantUser: false,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, ok := GetUserFromContext(tt.ctx)

			if ok != tt.wantOK {
				t.Errorf("Expected ok=%v, got %v", tt.wantOK, ok)
			}

			if tt.wantUser && user == nil {
				t.Error("Expected non-nil user")
			}

			if !tt.wantUser && user != nil {
				t.Error("Expected nil user")
			}
		})
	}
}

func TestLocalAPIToken_ValidToken(t *testing.T) {
	service := &mockAuthService{
		getOrCreateUserFunc: func(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
			return &auth.User{
				ID:       42,
				Username: username,
				Email:    email,
				IsAdmin:  false, // DB default, middleware should override
			}, nil
		},
	}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)
	mw.WithLocalAPIToken("test-secret-token")

	handlerCalled := false
	var contextUser *auth.User

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		user, ok := GetUserFromContext(r.Context())
		if ok {
			contextUser = user
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()

	mw.RequireAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !handlerCalled {
		t.Error("Expected handler to be called")
	}
	if contextUser == nil {
		t.Fatal("Expected user in context")
	}
	if contextUser.ID != 42 {
		t.Errorf("Expected user ID 42 from DB, got %d", contextUser.ID)
	}
	if contextUser.Username != "local-api" {
		t.Errorf("Expected username 'local-api', got %q", contextUser.Username)
	}
	if !contextUser.IsAdmin {
		t.Error("Expected local API user to be admin")
	}
}

func TestLocalAPIToken_FallbackToSyntheticUser(t *testing.T) {
	// When authService is nil, should fall back to synthetic user
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(nil, logger)
	mw.WithLocalAPIToken("test-secret-token")

	var contextUser *auth.User
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if ok {
			contextUser = user
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()

	mw.RequireAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if contextUser == nil {
		t.Fatal("Expected user in context")
	}
	if contextUser.ID != 0 {
		t.Errorf("Expected synthetic user ID 0, got %d", contextUser.ID)
	}
	if contextUser.Username != "local-api" {
		t.Errorf("Expected username 'local-api', got %q", contextUser.Username)
	}
	if !contextUser.IsAdmin {
		t.Error("Expected local API user to be admin")
	}
}

func TestLocalAPIToken_InvalidToken(t *testing.T) {
	service := &mockAuthService{}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)
	mw.WithLocalAPIToken("test-secret-token")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()

	mw.RequireAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestLocalAPIToken_NotConfigured(t *testing.T) {
	service := &mockAuthService{}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)
	// No WithLocalAPIToken call — token not configured

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	w := httptest.NewRecorder()

	mw.RequireAuth(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 when token not configured, got %d", w.Code)
	}
}

func TestMiddlewareChain(t *testing.T) {
	testUser := &auth.User{
		ID:       1,
		Username: "testuser",
	}

	service := &mockAuthService{
		validateSessionFunc: func(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
			return &auth.Session{ID: sessionID, UserID: 1}, testUser, nil
		},
	}
	logger := &mockAuthLogger{}
	mw := NewAuthMiddleware(service, logger)

	// Test that middleware can be chained
	var executionOrder []string

	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "mw1-before")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "mw1-after")
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, "handler")
		w.WriteHeader(http.StatusOK)
	})

	// Chain: middleware1 -> RequireAuth -> handler
	chain := middleware1(mw.RequireAuth(finalHandler))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: "test-session",
	})
	w := httptest.NewRecorder()

	chain.ServeHTTP(w, req)

	expectedOrder := []string{"mw1-before", "handler", "mw1-after"}
	if len(executionOrder) != len(expectedOrder) {
		t.Errorf("Expected %d execution steps, got %d", len(expectedOrder), len(executionOrder))
	}

	for i, step := range expectedOrder {
		if i >= len(executionOrder) {
			t.Errorf("Expected step %d to be %s, but step is missing", i, step)
		} else if executionOrder[i] != step {
			t.Errorf("Expected step %d to be %s, got %s", i, step, executionOrder[i])
		}
	}
}
