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

func TestNewAuthHandlers(t *testing.T) {
	service := &mocks.AuthServiceMock{}
	logger := mocks.NewMockLogger()

	handlers := NewAuthHandlers(service, logger, false, nil)
	if handlers == nil {
		t.Fatal("Expected non-nil handlers")
	}
}

func TestHandleGoogleLogin(t *testing.T) {
	storeStateCalled := false
	service := &mocks.AuthServiceMock{
		GetLoginURLFn: func(state string) string {
			return "https://accounts.google.com/test?state=" + state
		},
		StoreOAuthStateFn: func(ctx context.Context, state string, expiresAt time.Time) error {
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
	service := &mocks.AuthServiceMock{
		StoreOAuthStateFn: func(ctx context.Context, state string, expiresAt time.Time) error {
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
	service := &mocks.AuthServiceMock{}
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
	service := &mocks.AuthServiceMock{}
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
	service := &mocks.AuthServiceMock{
		ConsumeOAuthStateFn: func(ctx context.Context, state string) (bool, error) {
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
	service := &mocks.AuthServiceMock{
		ConsumeOAuthStateFn: func(ctx context.Context, state string) (bool, error) {
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
	service := &mocks.AuthServiceMock{
		DeleteSessionFn: func(ctx context.Context, sessionID string) error {
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
	service := &mocks.AuthServiceMock{}
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
	service := &mocks.AuthServiceMock{}
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
	service := &mocks.AuthServiceMock{
		GetLoginURLFn: func(state string) string {
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
