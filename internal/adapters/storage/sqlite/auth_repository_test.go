package sqlite

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// testEncryptionKey is a 32-byte key for AES-256 encryption in tests.
const testEncryptionKey = "12345678901234567890123456789012"

// newTestEncryptor creates an AES encryptor for tests, failing immediately on error.
func newTestEncryptor(t *testing.T) crypto.Encryptor {
	t.Helper()
	encryptor, err := crypto.NewAESEncryptor(testEncryptionKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}
	return encryptor
}

// setupAuthTestDB creates a temporary test database for auth tests
func setupAuthTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	// Create temporary database file
	tmpFile := t.TempDir() + "/test.db"

	db, err := sql.Open("sqlite3", tmpFile+"?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create auth tables (order matters for foreign key references)
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		google_id TEXT UNIQUE NOT NULL,
		username TEXT,
		email TEXT,
		avatar_url TEXT,
		is_admin BOOLEAN NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_login_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS user_sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_accessed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		user_agent TEXT,
		ip_address TEXT,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS user_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		session_id TEXT REFERENCES user_sessions(id) ON DELETE CASCADE,
		access_token TEXT NOT NULL,
		refresh_token TEXT NOT NULL,
		token_type TEXT DEFAULT 'Bearer',
		expires_at TIMESTAMP NOT NULL,
		scope TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX idx_user_tokens_user_id ON user_tokens(user_id);
	CREATE INDEX idx_user_tokens_session_id ON user_tokens(session_id);
	CREATE UNIQUE INDEX idx_user_tokens_session_unique ON user_tokens(session_id);

	CREATE TABLE IF NOT EXISTS allowed_emails (
		email TEXT PRIMARY KEY COLLATE NOCASE,
		added_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		notes TEXT
	);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(tmpFile)
	}

	return db, cleanup
}

func TestNewAuthRepository(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)

	repo := NewAuthRepository(db, encryptor)
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

func TestCreateUser(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	tests := []struct {
		name      string
		googleID  string
		username  string
		email     string
		avatarURL string
		wantErr   bool
	}{
		{
			name:      "valid user",
			googleID:  "test-google-123",
			username:  "testuser",
			email:     "test@example.com",
			avatarURL: "https://example.com/avatar.jpg",
			wantErr:   false,
		},
		{
			name:      "duplicate google user id",
			googleID:  "test-google-123",
			username:  "testuser2",
			email:     "test2@example.com",
			avatarURL: "https://example.com/avatar2.jpg",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := repo.CreateUser(ctx, tt.googleID, tt.username, tt.email, tt.avatarURL)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if user == nil {
					t.Error("Expected non-nil user")
					return
				}

				if user.ID == 0 {
					t.Error("Expected non-zero user ID")
				}

				if user.GoogleID != tt.googleID {
					t.Errorf("Expected googleID %s, got %s", tt.googleID, user.GoogleID)
				}

				if user.Username != tt.username {
					t.Errorf("Expected username %s, got %s", tt.username, user.Username)
				}

				if user.AvatarURL != tt.avatarURL {
					t.Errorf("Expected avatarURL %s, got %s", tt.avatarURL, user.AvatarURL)
				}
			}
		})
	}
}

func TestGetUserByGoogleID(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	// Create a test user
	createdUser, err := repo.CreateUser(ctx, "test-google-456", "gettest", "get@example.com", "https://example.com/avatar.jpg")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	tests := []struct {
		name     string
		googleID string
		wantErr  bool
	}{
		{
			name:     "existing user",
			googleID: "test-google-456",
			wantErr:  false,
		},
		{
			name:     "non-existent user",
			googleID: "non-existent",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := repo.GetUserByGoogleID(ctx, tt.googleID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetUserByGoogleID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if user == nil {
					t.Error("Expected non-nil user")
					return
				}

				if user.ID != createdUser.ID {
					t.Errorf("Expected user ID %d, got %d", createdUser.ID, user.ID)
				}
			}
		})
	}
}

func TestStoreAndGetTokens(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	// Create a test user
	user, err := repo.CreateUser(ctx, "test-google-789", "tokentest", "token@example.com", "https://example.com/avatar.jpg")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create a session first (required for token storage with multi-device support)
	session := &auth.Session{
		ID:             "test-session-for-tokens",
		UserID:         user.ID,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
		UserAgent:      "TestBrowser/1.0",
		IPAddress:      "127.0.0.1",
	}
	if err := repo.CreateSession(ctx, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	tokens := &auth.UserTokens{
		AccessToken:  "test-access-token-123",
		RefreshToken: "test-refresh-token-456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		Scope:        "test-scope",
	}

	// Store tokens
	err = repo.StoreTokens(ctx, user.ID, session.ID, tokens)
	if err != nil {
		t.Fatalf("StoreTokens() error = %v", err)
	}

	// Retrieve tokens
	retrievedTokens, err := repo.GetTokens(ctx, user.ID, session.ID)
	if err != nil {
		t.Fatalf("GetTokens() error = %v", err)
	}

	// Verify tokens
	if retrievedTokens.AccessToken != tokens.AccessToken {
		t.Errorf("Expected access token %s, got %s", tokens.AccessToken, retrievedTokens.AccessToken)
	}

	if retrievedTokens.RefreshToken != tokens.RefreshToken {
		t.Errorf("Expected refresh token %s, got %s", tokens.RefreshToken, retrievedTokens.RefreshToken)
	}

	if retrievedTokens.Scope != tokens.Scope {
		t.Errorf("Expected scope %s, got %s", tokens.Scope, retrievedTokens.Scope)
	}
}

func TestUpdateTokens(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	// Create user and initial tokens
	user, err := repo.CreateUser(ctx, "test-update", "updatetest", "update@example.com", "https://example.com/avatar.jpg")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create a session first (required for token storage with multi-device support)
	session := &auth.Session{
		ID:             "test-session-for-update",
		UserID:         user.ID,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
		UserAgent:      "TestBrowser/1.0",
		IPAddress:      "127.0.0.1",
	}
	if err := repo.CreateSession(ctx, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	initialTokens := &auth.UserTokens{
		AccessToken:  "initial-access",
		RefreshToken: "initial-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scope:        "initial-scope",
	}
	if err := repo.StoreTokens(ctx, user.ID, session.ID, initialTokens); err != nil {
		t.Fatalf("Failed to store initial tokens: %v", err)
	}

	// Update tokens
	updatedTokens := &auth.UserTokens{
		AccessToken:  "updated-access",
		RefreshToken: "initial-refresh", // Keep refresh token
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		Scope:        "initial-scope",
	}

	err = repo.UpdateTokens(ctx, user.ID, session.ID, updatedTokens)
	if err != nil {
		t.Fatalf("UpdateTokens() error = %v", err)
	}

	// Retrieve and verify
	retrieved, err := repo.GetTokens(ctx, user.ID, session.ID)
	if err != nil {
		t.Fatalf("Failed to get tokens: %v", err)
	}
	if retrieved.AccessToken != "updated-access" {
		t.Errorf("Expected updated access token, got %s", retrieved.AccessToken)
	}
}

func TestCreateAndGetSession(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	// Create a test user
	user, err := repo.CreateUser(ctx, "test-session", "sessiontest", "session@example.com", "https://example.com/avatar.jpg")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	session := &auth.Session{
		ID:             "test-session-123",
		UserID:         user.ID,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
		UserAgent:      "Mozilla/5.0",
		IPAddress:      "127.0.0.1",
	}

	// Create session
	err = repo.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Retrieve session
	retrieved, err := repo.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, retrieved.ID)
	}

	if retrieved.UserID != user.ID {
		t.Errorf("Expected user ID %d, got %d", user.ID, retrieved.UserID)
	}
}

func TestDeleteSession(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	// Create user and session
	user, err := repo.CreateUser(ctx, "test-delete", "deletetest", "delete@example.com", "https://example.com/avatar.jpg")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	session := &auth.Session{
		ID:        "delete-session",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := repo.CreateSession(ctx, session); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Delete session
	err = repo.DeleteSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	// Verify deletion
	_, err = repo.GetSession(ctx, session.ID)
	if err == nil {
		t.Error("Expected error when getting deleted session")
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	// Create user
	user, err := repo.CreateUser(ctx, "test-cleanup", "cleanuptest", "cleanup@example.com", "")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create mix of valid and expired sessions
	validSession := &auth.Session{
		ID:        "valid-session",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := repo.CreateSession(ctx, validSession); err != nil {
		t.Fatalf("Failed to create valid session: %v", err)
	}

	expiredSession1 := &auth.Session{
		ID:        "expired-1",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}
	if err := repo.CreateSession(ctx, expiredSession1); err != nil {
		t.Fatalf("Failed to create expired session 1: %v", err)
	}

	expiredSession2 := &auth.Session{
		ID:        "expired-2",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(-2 * time.Hour),
		CreatedAt: time.Now().Add(-3 * time.Hour),
	}
	if err := repo.CreateSession(ctx, expiredSession2); err != nil {
		t.Fatalf("Failed to create expired session 2: %v", err)
	}

	// Delete expired sessions
	count, err := repo.DeleteExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions() error = %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 expired sessions deleted, got %d", count)
	}

	// Verify valid session still exists
	_, err = repo.GetSession(ctx, "valid-session")
	if err != nil {
		t.Error("Valid session should still exist")
	}

	// Verify expired sessions are gone
	_, err = repo.GetSession(ctx, "expired-1")
	if err == nil {
		t.Error("Expired session should be deleted")
	}
}

func TestUpdateSessionAccess(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	// Create user and session
	user, err := repo.CreateUser(ctx, "test-access", "accesstest", "access@example.com", "")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	oldTime := time.Now().Add(-1 * time.Hour)
	session := &auth.Session{
		ID:             "access-session",
		UserID:         user.ID,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		CreatedAt:      oldTime,
		LastAccessedAt: oldTime,
	}
	if err := repo.CreateSession(ctx, session); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Update session access
	time.Sleep(10 * time.Millisecond) // Small delay to ensure timestamp difference
	err = repo.UpdateSessionAccess(ctx, session.ID)
	if err != nil {
		t.Fatalf("UpdateSessionAccess() error = %v", err)
	}

	// Retrieve and verify
	retrieved, err := repo.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}
	if !retrieved.LastAccessedAt.After(oldTime) {
		t.Error("LastAccessedAt should be updated to a more recent time")
	}
}

func TestTokenEncryption(t *testing.T) {
	db, cleanup := setupAuthTestDB(t)
	defer cleanup()

	encryptor := newTestEncryptor(t)
	repo := NewAuthRepository(db, encryptor)
	ctx := context.Background()

	// Create user
	user, err := repo.CreateUser(ctx, "test-encryption", "encrypttest", "encrypt@example.com", "")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create a session first (required for token storage with multi-device support)
	session := &auth.Session{
		ID:             "test-session-for-encryption",
		UserID:         user.ID,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
		UserAgent:      "TestBrowser/1.0",
		IPAddress:      "127.0.0.1",
	}
	if err := repo.CreateSession(ctx, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Store tokens
	originalTokens := &auth.UserTokens{
		AccessToken:  "secret-access-token",
		RefreshToken: "secret-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		Scope:        "test-scope",
	}
	if err := repo.StoreTokens(ctx, user.ID, session.ID, originalTokens); err != nil {
		t.Fatalf("Failed to store tokens: %v", err)
	}

	// Query database directly to verify tokens are encrypted
	var storedAccess, storedRefresh string
	err = db.QueryRow("SELECT access_token, refresh_token FROM user_tokens WHERE user_id = ?", user.ID).
		Scan(&storedAccess, &storedRefresh)
	if err != nil {
		t.Fatalf("Failed to query tokens: %v", err)
	}

	// Verify tokens are encrypted (not plaintext)
	if storedAccess == originalTokens.AccessToken {
		t.Error("Access token should be encrypted in database")
	}
	if storedRefresh == originalTokens.RefreshToken {
		t.Error("Refresh token should be encrypted in database")
	}

	// Verify retrieval decrypts correctly
	retrieved, err := repo.GetTokens(ctx, user.ID, session.ID)
	if err != nil {
		t.Fatalf("Failed to get tokens: %v", err)
	}
	if retrieved.AccessToken != originalTokens.AccessToken {
		t.Error("Decrypted access token should match original")
	}
	if retrieved.RefreshToken != originalTokens.RefreshToken {
		t.Error("Decrypted refresh token should match original")
	}
}
