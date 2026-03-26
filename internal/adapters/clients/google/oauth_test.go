package google

import (
	"context"
	"encoding/json"
	"errors"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// Mock repository for testing
type mockRepository struct {
	users    map[string]*auth.User
	tokens   map[int64]*auth.UserTokens
	sessions map[string]*auth.Session

	createUserErr    error
	getUserErr       error
	updateUserErr    error
	storeTokensErr   error
	getTokensErr     error
	updateTokensErr  error
	createSessionErr error
	getSessionErr    error
	updateSessionErr error
	deleteSessionErr error
	deleteExpiredErr error
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		users:    make(map[string]*auth.User),
		tokens:   make(map[int64]*auth.UserTokens),
		sessions: make(map[string]*auth.Session),
	}
}

func (m *mockRepository) CreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	if m.createUserErr != nil {
		return nil, m.createUserErr
	}
	user := &auth.User{
		ID:        int64(len(m.users) + 1),
		GoogleID:  googleID,
		Username:  username,
		Email:     email,
		AvatarURL: avatarURL,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.users[googleID] = user
	return user, nil
}

func (m *mockRepository) GetUserByGoogleID(ctx context.Context, googleID string) (*auth.User, error) {
	if m.getUserErr != nil {
		return nil, m.getUserErr
	}
	user, ok := m.users[googleID]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return user, nil
}

func (m *mockRepository) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	if m.getUserErr != nil {
		return nil, m.getUserErr
	}
	for _, user := range m.users {
		if user.ID == userID {
			return user, nil
		}
	}
	return nil, auth.ErrUserNotFound
}

func (m *mockRepository) UpdateUser(ctx context.Context, user *auth.User) error {
	if m.updateUserErr != nil {
		return m.updateUserErr
	}
	m.users[user.GoogleID] = user
	return nil
}

func (m *mockRepository) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	if m.storeTokensErr != nil {
		return m.storeTokensErr
	}
	m.tokens[userID] = tokens
	return nil
}

func (m *mockRepository) GetTokens(ctx context.Context, userID int64, sessionID string) (*auth.UserTokens, error) {
	if m.getTokensErr != nil {
		return nil, m.getTokensErr
	}
	tokens, ok := m.tokens[userID]
	if !ok {
		return nil, errors.New("tokens not found")
	}
	return tokens, nil
}

func (m *mockRepository) GetTokensByUserID(ctx context.Context, userID int64) (*auth.UserTokens, error) {
	if m.getTokensErr != nil {
		return nil, m.getTokensErr
	}
	tokens, ok := m.tokens[userID]
	if !ok {
		return nil, errors.New("tokens not found")
	}
	return tokens, nil
}

func (m *mockRepository) UpdateTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	if m.updateTokensErr != nil {
		return m.updateTokensErr
	}
	m.tokens[userID] = tokens
	return nil
}

func (m *mockRepository) DeleteTokens(ctx context.Context, userID int64, sessionID string) error {
	delete(m.tokens, userID)
	return nil
}

func (m *mockRepository) DeleteAllUserTokens(ctx context.Context, userID int64) error {
	delete(m.tokens, userID)
	return nil
}

func (m *mockRepository) CreateSession(ctx context.Context, session *auth.Session) error {
	if m.createSessionErr != nil {
		return m.createSessionErr
	}
	m.sessions[session.ID] = session
	return nil
}

func (m *mockRepository) GetSession(ctx context.Context, sessionID string) (*auth.Session, error) {
	if m.getSessionErr != nil {
		return nil, m.getSessionErr
	}
	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (m *mockRepository) UpdateSessionAccess(ctx context.Context, sessionID string) error {
	if m.updateSessionErr != nil {
		return m.updateSessionErr
	}
	if session, ok := m.sessions[sessionID]; ok {
		session.LastAccessedAt = time.Now()
	}
	return nil
}

func (m *mockRepository) DeleteSession(ctx context.Context, sessionID string) error {
	if m.deleteSessionErr != nil {
		return m.deleteSessionErr
	}
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockRepository) DeleteExpiredSessions(ctx context.Context) (int, error) {
	if m.deleteExpiredErr != nil {
		return 0, m.deleteExpiredErr
	}
	count := 0
	now := time.Now()
	for id, session := range m.sessions {
		if session.ExpiresAt.Before(now) {
			delete(m.sessions, id)
			count++
		}
	}
	return count, nil
}

// OAuth State methods
func (m *mockRepository) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	// No-op for tests - state validation handled differently in tests
	return nil
}

func (m *mockRepository) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	// Always return true for tests - actual state validation tested elsewhere
	return true, nil
}

func (m *mockRepository) CleanupExpiredOAuthStates(ctx context.Context) (int, error) {
	// No-op for tests
	return 0, nil
}

func (m *mockRepository) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	return false, nil
}

func (m *mockRepository) ListAllowedEmails(ctx context.Context) ([]auth.AllowedEmail, error) {
	return nil, nil
}

func (m *mockRepository) AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error {
	return nil
}

func (m *mockRepository) RemoveAllowedEmail(ctx context.Context, email string) error {
	return nil
}

func (m *mockRepository) ListUsers(ctx context.Context) ([]auth.User, error) {
	return nil, nil
}

func (m *mockRepository) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	return nil
}

func TestNewOAuthService(t *testing.T) {
	repo := newMockRepository()
	logger := mocks.NewMockLogger()

	service := NewOAuthService(
		repo,
		logger,
		"test-client-id",
		"test-client-secret",
		"http://localhost/callback",
		[]string{"openid", "email", "profile"},
	)

	if service == nil {
		t.Fatal("Expected service to be non-nil")
	}
}

func TestGetLoginURL(t *testing.T) {
	repo := newMockRepository()
	logger := mocks.NewMockLogger()

	service := NewOAuthService(
		repo,
		logger,
		"test-client-id",
		"test-client-secret",
		"http://localhost/callback",
		[]string{"openid", "email", "profile"},
	)

	state := "test-state"
	url := service.GetLoginURL(state)

	if url == "" {
		t.Error("Expected non-empty URL")
	}

	// Check if URL contains Google OAuth base
	wantURL := "https://accounts.google.com/o/oauth2/v2/auth"
	if !strings.Contains(url, wantURL) {
		t.Errorf("Expected URL to contain %s, got %s", wantURL, url)
	}

	// Check if URL contains state parameter
	if !strings.Contains(url, "state="+state) {
		t.Errorf("Expected URL to contain state parameter")
	}

	// Check if URL contains client_id
	if !strings.Contains(url, "client_id=test-client-id") {
		t.Errorf("Expected URL to contain client_id parameter")
	}
}

func TestCreateSession(t *testing.T) {
	repo := newMockRepository()
	logger := mocks.NewMockLogger()
	service := NewOAuthService(
		repo,
		logger,
		"test-client-id",
		"test-client-secret",
		"http://localhost/callback",
		[]string{"openid", "email", "profile"},
	)

	ctx := context.Background()

	tests := []struct {
		name      string
		userID    int64
		userAgent string
		ipAddress string
		wantErr   bool
	}{
		{
			name:      "valid session creation",
			userID:    1,
			userAgent: "Mozilla/5.0",
			ipAddress: "127.0.0.1",
			wantErr:   false,
		},
		{
			name:      "empty user agent",
			userID:    2,
			userAgent: "",
			ipAddress: "127.0.0.1",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := service.CreateSession(ctx, tt.userID, tt.userAgent, tt.ipAddress)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if session == nil {
					t.Error("Expected non-nil session")
					return
				}

				if session.ID == "" {
					t.Error("Expected non-empty session ID")
				}

				if session.UserID != tt.userID {
					t.Errorf("Expected userID %d, got %d", tt.userID, session.UserID)
				}

				if session.ExpiresAt.Before(time.Now()) {
					t.Error("Expected expiration in the future")
				}
			}
		})
	}
}

func TestValidateSession(t *testing.T) {
	repo := newMockRepository()
	logger := mocks.NewMockLogger()
	service := NewOAuthService(
		repo,
		logger,
		"test-client-id",
		"test-client-secret",
		"http://localhost/callback",
		[]string{"openid", "email", "profile"},
	)

	ctx := context.Background()

	// Create a test user
	user := &auth.User{
		ID:       1,
		GoogleID: "test-google-user",
		Username: "testuser",
		Email:    "test@example.com",
	}
	repo.users["test-google-user"] = user

	// Create a valid session
	validSession := &auth.Session{
		ID:             "valid-session-id",
		UserID:         1,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
	}
	repo.sessions["valid-session-id"] = validSession

	// Create an expired session
	expiredSession := &auth.Session{
		ID:             "expired-session-id",
		UserID:         1,
		ExpiresAt:      time.Now().Add(-1 * time.Hour),
		CreatedAt:      time.Now().Add(-2 * time.Hour),
		LastAccessedAt: time.Now().Add(-2 * time.Hour),
	}
	repo.sessions["expired-session-id"] = expiredSession

	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
	}{
		{
			name:      "valid session",
			sessionID: "valid-session-id",
			wantErr:   false,
		},
		{
			name:      "expired session",
			sessionID: "expired-session-id",
			wantErr:   true,
		},
		{
			name:      "non-existent session",
			sessionID: "non-existent",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, user, err := service.ValidateSession(ctx, tt.sessionID)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if session == nil {
					t.Error("Expected non-nil session")
				}
				if user == nil {
					t.Error("Expected non-nil user")
				}
			}
		})
	}
}

func TestDeleteSession(t *testing.T) {
	repo := newMockRepository()
	logger := mocks.NewMockLogger()
	service := NewOAuthService(
		repo,
		logger,
		"test-client-id",
		"test-client-secret",
		"http://localhost/callback",
		[]string{"openid", "email", "profile"},
	)

	ctx := context.Background()

	// Create a test session
	repo.sessions["test-session"] = &auth.Session{
		ID:     "test-session",
		UserID: 1,
	}

	err := service.DeleteSession(ctx, "test-session")
	if err != nil {
		t.Errorf("DeleteSession() error = %v", err)
	}

	// Verify session was deleted
	if _, exists := repo.sessions["test-session"]; exists {
		t.Error("Expected session to be deleted")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	repo := newMockRepository()
	logger := mocks.NewMockLogger()
	service := NewOAuthService(
		repo,
		logger,
		"test-client-id",
		"test-client-secret",
		"http://localhost/callback",
		[]string{"openid", "email", "profile"},
	)

	ctx := context.Background()

	// Create mix of valid and expired sessions
	repo.sessions["valid-1"] = &auth.Session{
		ID:        "valid-1",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	repo.sessions["expired-1"] = &auth.Session{
		ID:        "expired-1",
		UserID:    2,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	repo.sessions["expired-2"] = &auth.Session{
		ID:        "expired-2",
		UserID:    3,
		ExpiresAt: time.Now().Add(-2 * time.Hour),
	}

	count, err := service.CleanupExpiredSessions(ctx)
	if err != nil {
		t.Errorf("CleanupExpiredSessions() error = %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 sessions cleaned, got %d", count)
	}

	// Verify only valid session remains
	if len(repo.sessions) != 1 {
		t.Errorf("Expected 1 session remaining, got %d", len(repo.sessions))
	}
}

func TestGetOrCreateUser(t *testing.T) {
	repo := newMockRepository()
	logger := mocks.NewMockLogger()
	service := NewOAuthService(
		repo,
		logger,
		"test-client-id",
		"test-client-secret",
		"http://localhost/callback",
		[]string{"openid", "email", "profile"},
	)

	ctx := context.Background()

	tests := []struct {
		name      string
		googleID  string
		username  string
		email     string
		avatarURL string
		existing  bool
	}{
		{
			name:      "create new user",
			googleID:  "new-user",
			username:  "newuser",
			email:     "new@example.com",
			avatarURL: "https://example.com/avatar.jpg",
			existing:  false,
		},
		{
			name:      "get existing user",
			googleID:  "existing-user",
			username:  "existinguser",
			email:     "existing@example.com",
			avatarURL: "https://example.com/existing.jpg",
			existing:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pre-create user if testing existing user
			if tt.existing {
				repo.users[tt.googleID] = &auth.User{
					ID:        1,
					GoogleID:  tt.googleID,
					Username:  tt.username,
					Email:     tt.email,
					AvatarURL: tt.avatarURL,
				}
			}

			user, err := service.GetOrCreateUser(ctx, tt.googleID, tt.username, tt.email, tt.avatarURL)
			if err != nil {
				t.Errorf("GetOrCreateUser() error = %v", err)
				return
			}

			if user == nil {
				t.Error("Expected non-nil user")
				return
			}

			if user.GoogleID != tt.googleID {
				t.Errorf("Expected googleID %s, got %s", tt.googleID, user.GoogleID)
			}
		})
	}
}

func TestStoreTokens(t *testing.T) {
	repo := newMockRepository()
	logger := mocks.NewMockLogger()
	service := NewOAuthService(
		repo,
		logger,
		"test-client-id",
		"test-client-secret",
		"http://localhost/callback",
		[]string{"openid", "email", "profile"},
	)

	ctx := context.Background()

	tokens := &auth.UserTokens{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scope:        "openid email profile",
	}

	err := service.StoreTokens(ctx, 1, "test-session-id", tokens)
	if err != nil {
		t.Errorf("StoreTokens() error = %v", err)
	}

	// Verify tokens were stored
	stored, ok := repo.tokens[1]
	if !ok {
		t.Error("Expected tokens to be stored")
		return
	}

	if stored.AccessToken != tokens.AccessToken {
		t.Errorf("Expected access token %s, got %s", tokens.AccessToken, stored.AccessToken)
	}
}

func TestGetUserInfo(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   map[string]string
		wantErr    bool
		wantID     string
		wantName   string
		wantEmail  string
		wantAvatar string
	}{
		{
			name:       "successful response",
			statusCode: http.StatusOK,
			response: map[string]string{
				"id":      "google-123",
				"name":    "Test User",
				"email":   "test@example.com",
				"picture": "https://example.com/avatar.jpg",
			},
			wantErr:    false,
			wantID:     "google-123",
			wantName:   "Test User",
			wantEmail:  "test@example.com",
			wantAvatar: "https://example.com/avatar.jpg",
		},
		{
			name:       "successful response without picture",
			statusCode: http.StatusOK,
			response: map[string]string{
				"id":    "google-456",
				"name":  "No Avatar User",
				"email": "noavatar@example.com",
			},
			wantErr:    false,
			wantID:     "google-456",
			wantName:   "No Avatar User",
			wantEmail:  "noavatar@example.com",
			wantAvatar: "",
		},
		{
			name:       "error response",
			statusCode: http.StatusUnauthorized,
			response:   map[string]string{"error": "invalid_token"},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify authorization header
				authHeader := r.Header.Get("Authorization")
				if authHeader != "Bearer test-access-token" {
					t.Errorf("Expected Authorization header 'Bearer test-access-token', got %q", authHeader)
				}

				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			repo := newMockRepository()
			logger := mocks.NewMockLogger()
			service := NewOAuthService(repo, logger, "id", "secret", "http://localhost/cb", nil)

			parsed, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("parse test server URL: %v", err)
			}
			testHost := parsed.Host
			dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 5 * time.Second}
			transport := &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, network, testHost)
				},
				TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test-only
				TLSHandshakeTimeout: 5 * time.Second,
			}
			httpCfg := httpx.DefaultConfig("test")
			httpCfg.Transport = transport
			service.httpClient = httpx.NewClient(httpCfg)

			ctx := context.Background()
			info, err := service.GetUserInfo(ctx, "test-access-token")

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if info.ProviderID != tt.wantID {
				t.Errorf("Expected ProviderID %q, got %q", tt.wantID, info.ProviderID)
			}
			if info.Name != tt.wantName {
				t.Errorf("Expected Name %q, got %q", tt.wantName, info.Name)
			}
			if info.Email != tt.wantEmail {
				t.Errorf("Expected Email %q, got %q", tt.wantEmail, info.Email)
			}
			if info.AvatarURL != tt.wantAvatar {
				t.Errorf("Expected AvatarURL %q, got %q", tt.wantAvatar, info.AvatarURL)
			}
		})
	}
}

