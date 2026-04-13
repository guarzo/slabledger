package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// ─── GenerateState ────────────────────────────────────────────────────────────

func TestGenerateState(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "generate state 1"},
		{name: "generate state 2"},
		{name: "generate state 3"},
	}

	states := make(map[string]bool)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := auth.GenerateState()
			if err != nil {
				t.Errorf("GenerateState() error = %v", err)
				return
			}

			if state == "" {
				t.Error("Expected non-empty state")
			}

			// Check uniqueness
			if states[state] {
				t.Error("State should be unique")
			}
			states[state] = true

			// Check length (base64 encoded 32 bytes should be ~44 chars)
			if len(state) < 40 {
				t.Errorf("State too short: %d characters", len(state))
			}
		})
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// newSvc builds an auth.Service backed by a fresh in-memory repository.
// loginURLFn is optional; pass nil for tests that don't exercise GetLoginURL.
func newSvc(loginURLFn func(string) string) (auth.Service, *mocks.InMemoryAuthRepository) {
	repo := mocks.NewInMemoryAuthRepository()
	svc := auth.New(repo, loginURLFn)
	return svc, repo
}

// ─── GetOrCreateUser ─────────────────────────────────────────────────────────

func TestAuthService_GetOrCreateUser_Idempotency(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	u1, err := svc.GetOrCreateUser(ctx, "google-123", "alice", "alice@example.com", "")
	if err != nil {
		t.Fatalf("first GetOrCreateUser: %v", err)
	}
	u2, err := svc.GetOrCreateUser(ctx, "google-123", "alice", "alice@example.com", "")
	if err != nil {
		t.Fatalf("second GetOrCreateUser: %v", err)
	}
	if u1.ID != u2.ID {
		t.Errorf("expected same user ID on second call, got %d and %d", u1.ID, u2.ID)
	}
}

func TestAuthService_GetOrCreateUser_DifferentGoogleIDs(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	u1, _ := svc.GetOrCreateUser(ctx, "google-aaa", "alice", "alice@example.com", "")
	u2, _ := svc.GetOrCreateUser(ctx, "google-bbb", "bob", "bob@example.com", "")

	if u1.ID == u2.ID {
		t.Errorf("expected distinct IDs for different Google IDs, both got %d", u1.ID)
	}
}

// ─── Session round-trip ───────────────────────────────────────────────────────

func TestAuthService_Session_RoundTrip(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	u, err := svc.GetOrCreateUser(ctx, "g-1", "bob", "bob@example.com", "")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	sess, err := svc.CreateSession(ctx, u.ID, "Mozilla/5.0", "127.0.0.1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}

	gotSess, gotUser, err := svc.ValidateSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if gotSess.ID != sess.ID {
		t.Errorf("session ID mismatch: got %s, want %s", gotSess.ID, sess.ID)
	}
	if gotUser.ID != u.ID {
		t.Errorf("user ID mismatch: got %d, want %d", gotUser.ID, u.ID)
	}
}

func TestAuthService_Session_MultipleDistinctSessions(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	u, _ := svc.GetOrCreateUser(ctx, "g-2", "carol", "carol@example.com", "")

	s1, _ := svc.CreateSession(ctx, u.ID, "Chrome", "10.0.0.1")
	s2, _ := svc.CreateSession(ctx, u.ID, "Firefox", "10.0.0.2")

	if s1.ID == s2.ID {
		t.Errorf("expected distinct session IDs, both got %s", s1.ID)
	}
}

// ─── Session delete ───────────────────────────────────────────────────────────

func TestAuthService_Session_Delete(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	u, _ := svc.GetOrCreateUser(ctx, "g-3", "dave", "dave@example.com", "")
	sess, _ := svc.CreateSession(ctx, u.ID, "Safari", "192.168.1.1")

	if err := svc.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, _, err := svc.ValidateSession(ctx, sess.ID)
	if err == nil {
		t.Fatal("expected error after DeleteSession, got nil")
	}
}

// ─── CleanupExpiredSessions ───────────────────────────────────────────────────

func TestAuthService_CleanupExpiredSessions(t *testing.T) {
	svc, repo := newSvc(nil)
	ctx := context.Background()

	u, _ := svc.GetOrCreateUser(ctx, "g-4", "eve", "eve@example.com", "")

	// Seed an expired session directly into the repo (bypasses service expiry logic).
	expiredSess := &auth.Session{
		ID:             "expired-session-id",
		UserID:         u.ID,
		ExpiresAt:      time.Now().Add(-1 * time.Hour),
		CreatedAt:      time.Now().Add(-2 * time.Hour),
		LastAccessedAt: time.Now().Add(-2 * time.Hour),
	}
	if err := repo.CreateSession(ctx, expiredSess); err != nil {
		t.Fatalf("seed expired session: %v", err)
	}

	// Create a valid session via the service.
	activeSess, err := svc.CreateSession(ctx, u.ID, "Edge", "10.1.1.1")
	if err != nil {
		t.Fatalf("CreateSession (active): %v", err)
	}

	count, err := svc.CleanupExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredSessions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 expired session removed, got %d", count)
	}

	// Expired session must no longer be found.
	_, _, err = svc.ValidateSession(ctx, expiredSess.ID)
	if err == nil {
		t.Error("expected error for expired (cleaned-up) session, got nil")
	}

	// Active session must still validate.
	_, _, err = svc.ValidateSession(ctx, activeSess.ID)
	if err != nil {
		t.Errorf("active session should still be valid after cleanup: %v", err)
	}
}

// ─── OAuthState consume-once ──────────────────────────────────────────────────

func TestAuthService_OAuthState_ConsumeOnce(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	state := "random-csrf-token"
	expiry := time.Now().Add(10 * time.Minute)

	if err := svc.StoreOAuthState(ctx, state, expiry); err != nil {
		t.Fatalf("StoreOAuthState: %v", err)
	}

	ok, err := svc.ConsumeOAuthState(ctx, state)
	if err != nil || !ok {
		t.Fatalf("first ConsumeOAuthState: ok=%v err=%v", ok, err)
	}

	ok2, _ := svc.ConsumeOAuthState(ctx, state)
	if ok2 {
		t.Error("expected false on second consume, got true")
	}
}

func TestAuthService_OAuthState_ExpiredNotConsumed(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	state := "expired-csrf-token"
	expiry := time.Now().Add(-1 * time.Minute) // already expired

	if err := svc.StoreOAuthState(ctx, state, expiry); err != nil {
		t.Fatalf("StoreOAuthState: %v", err)
	}

	ok, err := svc.ConsumeOAuthState(ctx, state)
	if err != nil {
		t.Fatalf("ConsumeOAuthState: %v", err)
	}
	if ok {
		t.Error("expected false for expired state token, got true")
	}
}

func TestAuthService_OAuthState_UnknownState(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	ok, err := svc.ConsumeOAuthState(ctx, "nonexistent-state")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for unknown state, got true")
	}
}

// ─── Email allowlist ──────────────────────────────────────────────────────────

func TestAuthService_EmailAllowlist(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	email := "trusted@example.com"

	// Not yet allowed
	ok, err := svc.IsEmailAllowed(ctx, email)
	if err != nil {
		t.Fatalf("IsEmailAllowed (before add): %v", err)
	}
	if ok {
		t.Error("expected false before adding email")
	}

	// Add
	if err := svc.AddAllowedEmail(ctx, email, 1, "test note"); err != nil {
		t.Fatalf("AddAllowedEmail: %v", err)
	}

	// Now allowed
	ok, err = svc.IsEmailAllowed(ctx, email)
	if err != nil {
		t.Fatalf("IsEmailAllowed (after add): %v", err)
	}
	if !ok {
		t.Error("expected true after adding email")
	}

	// Remove
	if err := svc.RemoveAllowedEmail(ctx, email); err != nil {
		t.Fatalf("RemoveAllowedEmail: %v", err)
	}

	// No longer allowed
	ok, err = svc.IsEmailAllowed(ctx, email)
	if err != nil {
		t.Fatalf("IsEmailAllowed (after remove): %v", err)
	}
	if ok {
		t.Error("expected false after removing email")
	}
}

func TestAuthService_ListAllowedEmails(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	emails := []string{"a@example.com", "b@example.com", "c@example.com"}
	for i, e := range emails {
		if err := svc.AddAllowedEmail(ctx, e, int64(i+1), ""); err != nil {
			t.Fatalf("AddAllowedEmail(%s): %v", e, err)
		}
	}

	list, err := svc.ListAllowedEmails(ctx)
	if err != nil {
		t.Fatalf("ListAllowedEmails: %v", err)
	}
	if len(list) != len(emails) {
		t.Errorf("expected %d allowed emails, got %d", len(emails), len(list))
	}
}

// ─── GetUserByID ──────────────────────────────────────────────────────────────

func TestAuthService_GetUserByID_NotFound(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	_, err := svc.GetUserByID(ctx, 999)
	if err == nil {
		t.Fatal("expected error for unknown user ID, got nil")
	}
	if !auth.IsUserNotFound(err) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestAuthService_GetUserByID_Exists(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	created, _ := svc.GetOrCreateUser(ctx, "g-5", "frank", "frank@example.com", "")

	fetched, err := svc.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if fetched.Email != created.Email {
		t.Errorf("email mismatch: got %s, want %s", fetched.Email, created.Email)
	}
}

// ─── UpdateLastLogin ──────────────────────────────────────────────────────────

func TestAuthService_UpdateLastLogin(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	u, _ := svc.GetOrCreateUser(ctx, "g-6", "grace", "grace@example.com", "")
	if u.LastLoginAt != nil {
		t.Fatal("expected nil LastLoginAt for brand-new user")
	}

	if err := svc.UpdateLastLogin(ctx, u.ID); err != nil {
		t.Fatalf("UpdateLastLogin: %v", err)
	}

	updated, err := svc.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID after UpdateLastLogin: %v", err)
	}
	if updated.LastLoginAt == nil {
		t.Error("expected non-nil LastLoginAt after UpdateLastLogin")
	}
}

func TestAuthService_UpdateLastLogin_UnknownUser(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	err := svc.UpdateLastLogin(ctx, 9999)
	if err == nil {
		t.Fatal("expected error for unknown user, got nil")
	}
}

// ─── SetUserAdmin / ListUsers ─────────────────────────────────────────────────

func TestAuthService_SetUserAdmin(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	u, _ := svc.GetOrCreateUser(ctx, "g-7", "heidi", "heidi@example.com", "")
	if u.IsAdmin {
		t.Fatal("new user should not be admin by default")
	}

	if err := svc.SetUserAdmin(ctx, u.ID, true); err != nil {
		t.Fatalf("SetUserAdmin(true): %v", err)
	}

	// Verify via ListUsers
	users, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	var found bool
	for _, usr := range users {
		if usr.ID == u.ID {
			found = true
			if !usr.IsAdmin {
				t.Errorf("expected IsAdmin=true for user %d", u.ID)
			}
		}
	}
	if !found {
		t.Errorf("user %d not found in ListUsers", u.ID)
	}

	// Toggle back off
	if err := svc.SetUserAdmin(ctx, u.ID, false); err != nil {
		t.Fatalf("SetUserAdmin(false): %v", err)
	}
	updated, _ := svc.GetUserByID(ctx, u.ID)
	if updated.IsAdmin {
		t.Error("expected IsAdmin=false after toggling off")
	}
}

// ─── GetLoginURL ──────────────────────────────────────────────────────────────

func TestAuthService_GetLoginURL(t *testing.T) {
	svc, _ := newSvc(func(state string) string {
		return "https://example.com/auth?state=" + state
	})

	url := svc.GetLoginURL("test-state")
	expected := "https://example.com/auth?state=test-state"
	if url != expected {
		t.Errorf("GetLoginURL: got %s, want %s", url, expected)
	}
}

// ─── StoreTokens ─────────────────────────────────────────────────────────────

func TestAuthService_StoreTokens(t *testing.T) {
	svc, _ := newSvc(nil)
	ctx := context.Background()

	u, _ := svc.GetOrCreateUser(ctx, "g-8", "ivan", "ivan@example.com", "")
	sess, _ := svc.CreateSession(ctx, u.ID, "curl/7.0", "127.0.0.1")

	tokens := &auth.UserTokens{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-xyz",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scope:        "email profile",
	}

	if err := svc.StoreTokens(ctx, u.ID, sess.ID, tokens); err != nil {
		t.Fatalf("StoreTokens: %v", err)
	}
}

// ─── ExchangeCodeForTokens / GetUserInfo — not implemented stubs ──────────────

func TestAuthService_ExchangeCodeForTokens_NotImplemented(t *testing.T) {
	svc, _ := newSvc(nil)
	_, err := svc.ExchangeCodeForTokens(context.Background(), "code")
	if err == nil {
		t.Fatal("expected error from ExchangeCodeForTokens stub")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Errorf("expected *apperrors.AppError, got %T", err)
	}
}

func TestAuthService_GetUserInfo_NotImplemented(t *testing.T) {
	svc, _ := newSvc(nil)
	_, err := svc.GetUserInfo(context.Background(), "access-token")
	if err == nil {
		t.Fatal("expected error from GetUserInfo stub")
	}
}
