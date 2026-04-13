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
			if states[state] {
				t.Error("State should be unique")
			}
			states[state] = true
			// base64 encoded 32 bytes should be ~44 chars
			if len(state) < 40 {
				t.Errorf("State too short: %d characters", len(state))
			}
		})
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// newSvc builds an auth.Service backed by a fresh in-memory repository.
// opts are forwarded to auth.New; pass auth.WithLoginURLFn(...) for URL tests.
func newSvc(t *testing.T, opts ...auth.Option) (auth.Service, *mocks.InMemoryAuthRepository) {
	t.Helper()
	repo := mocks.NewInMemoryAuthRepository()
	svc, err := auth.New(repo, opts...)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return svc, repo
}

// mustCreateUser is a test helper that fatals on error.
func mustCreateUser(t *testing.T, svc auth.Service, googleID, username, email string) *auth.User {
	t.Helper()
	u, err := svc.GetOrCreateUser(context.Background(), googleID, username, email, "")
	if err != nil {
		t.Fatalf("GetOrCreateUser(%s): %v", googleID, err)
	}
	return u
}

// ─── New constructor ──────────────────────────────────────────────────────────

func TestNew_NilRepo(t *testing.T) {
	_, err := auth.New(nil)
	if err == nil {
		t.Fatal("expected error when repo is nil, got nil")
	}
}

// ─── GetOrCreateUser ─────────────────────────────────────────────────────────

func TestAuthService_GetOrCreateUser(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(svc auth.Service) // optional prior state
		googleID     string
		username     string
		email        string
		wantSameAs   string // if set, expect same ID as the user created with this googleID in setup
		wantDistinct bool   // expect a distinct ID from any prior user
	}{
		{
			name:     "idempotent: same googleID returns same user",
			googleID: "google-123",
			username: "alice",
			email:    "alice@example.com",
			setup: func(svc auth.Service) {
				mustCreateUser(&testing.T{}, svc, "google-123", "alice", "alice@example.com")
			},
		},
		{
			name:         "distinct googleIDs produce distinct users",
			googleID:     "google-bbb",
			username:     "bob",
			email:        "bob@example.com",
			wantDistinct: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSvc(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(svc)
			}

			u1, err := svc.GetOrCreateUser(ctx, tt.googleID, tt.username, tt.email, "")
			if err != nil {
				t.Fatalf("GetOrCreateUser: %v", err)
			}

			u2, err := svc.GetOrCreateUser(ctx, tt.googleID, tt.username, tt.email, "")
			if err != nil {
				t.Fatalf("GetOrCreateUser (second call): %v", err)
			}

			if u1.ID != u2.ID {
				t.Errorf("expected same user ID on repeated call, got %d and %d", u1.ID, u2.ID)
			}

			if tt.wantDistinct {
				other, err := svc.GetOrCreateUser(ctx, "google-aaa", "other", "other@example.com", "")
				if err != nil {
					t.Fatalf("GetOrCreateUser (other): %v", err)
				}
				if u1.ID == other.ID {
					t.Errorf("expected distinct IDs, both got %d", u1.ID)
				}
			}
		})
	}
}

// ─── Session ──────────────────────────────────────────────────────────────────

func TestAuthService_Session(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, svc auth.Service, repo *mocks.InMemoryAuthRepository)
	}{
		{
			name: "round-trip: created session validates successfully",
			run: func(t *testing.T, svc auth.Service, _ *mocks.InMemoryAuthRepository) {
				ctx := context.Background()
				u := mustCreateUser(t, svc, "g-1", "bob", "bob@example.com")

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
			},
		},
		{
			name: "multiple sessions for same user have distinct IDs",
			run: func(t *testing.T, svc auth.Service, _ *mocks.InMemoryAuthRepository) {
				ctx := context.Background()
				u := mustCreateUser(t, svc, "g-2", "carol", "carol@example.com")

				s1, err := svc.CreateSession(ctx, u.ID, "Chrome", "10.0.0.1")
				if err != nil {
					t.Fatalf("CreateSession s1: %v", err)
				}
				s2, err := svc.CreateSession(ctx, u.ID, "Firefox", "10.0.0.2")
				if err != nil {
					t.Fatalf("CreateSession s2: %v", err)
				}
				if s1.ID == s2.ID {
					t.Errorf("expected distinct session IDs, both got %s", s1.ID)
				}
			},
		},
		{
			name: "deleted session no longer validates",
			run: func(t *testing.T, svc auth.Service, _ *mocks.InMemoryAuthRepository) {
				ctx := context.Background()
				u := mustCreateUser(t, svc, "g-3", "dave", "dave@example.com")

				sess, err := svc.CreateSession(ctx, u.ID, "Safari", "192.168.1.1")
				if err != nil {
					t.Fatalf("CreateSession: %v", err)
				}
				if err := svc.DeleteSession(ctx, sess.ID); err != nil {
					t.Fatalf("DeleteSession: %v", err)
				}
				_, _, err = svc.ValidateSession(ctx, sess.ID)
				if err == nil {
					t.Fatal("expected error after DeleteSession, got nil")
				}
			},
		},
		{
			name: "cleanup removes expired sessions and retains active ones",
			run: func(t *testing.T, svc auth.Service, repo *mocks.InMemoryAuthRepository) {
				ctx := context.Background()
				u := mustCreateUser(t, svc, "g-4", "eve", "eve@example.com")

				// Seed an expired session directly into the repo.
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

				_, _, err = svc.ValidateSession(ctx, expiredSess.ID)
				if err == nil {
					t.Error("expected error for expired (cleaned-up) session, got nil")
				}

				_, _, err = svc.ValidateSession(ctx, activeSess.ID)
				if err != nil {
					t.Errorf("active session should still be valid after cleanup: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newSvc(t)
			tt.run(t, svc, repo)
		})
	}
}

// ─── OAuthState ───────────────────────────────────────────────────────────────

func TestAuthService_OAuthState(t *testing.T) {
	tests := []struct {
		name    string
		state   string
		expiry  func() time.Time
		wantOK  bool // expected result of first ConsumeOAuthState
		consume int  // how many times to consume (default 1)
	}{
		{
			name:   "valid state consumed once returns true",
			state:  "random-csrf-token",
			expiry: func() time.Time { return time.Now().Add(10 * time.Minute) },
			wantOK: true,
		},
		{
			name:   "already-expired state returns false",
			state:  "expired-csrf-token",
			expiry: func() time.Time { return time.Now().Add(-1 * time.Minute) },
			wantOK: false,
		},
		{
			name:   "unknown state returns false",
			state:  "", // won't be stored
			expiry: nil,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSvc(t)
			ctx := context.Background()

			if tt.expiry != nil {
				if err := svc.StoreOAuthState(ctx, tt.state, tt.expiry()); err != nil {
					t.Fatalf("StoreOAuthState: %v", err)
				}
			}

			ok, err := svc.ConsumeOAuthState(ctx, tt.state)
			if err != nil {
				t.Fatalf("ConsumeOAuthState: %v", err)
			}
			if ok != tt.wantOK {
				t.Errorf("ConsumeOAuthState: got %v, want %v", ok, tt.wantOK)
			}
		})
	}

	// Second consume of a valid state must return false (consume-once guarantee).
	t.Run("second consume returns false", func(t *testing.T) {
		svc, _ := newSvc(t)
		ctx := context.Background()

		if err := svc.StoreOAuthState(ctx, "one-time", time.Now().Add(5*time.Minute)); err != nil {
			t.Fatalf("StoreOAuthState: %v", err)
		}
		if _, err := svc.ConsumeOAuthState(ctx, "one-time"); err != nil {
			t.Fatalf("first consume: %v", err)
		}
		ok2, _ := svc.ConsumeOAuthState(ctx, "one-time")
		if ok2 {
			t.Error("expected false on second consume, got true")
		}
	})
}

// ─── Email allowlist ──────────────────────────────────────────────────────────

func TestAuthService_EmailAllowlist(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, svc auth.Service)
	}{
		{
			name: "add then IsEmailAllowed returns true",
			run: func(t *testing.T, svc auth.Service) {
				ctx := context.Background()
				email := "trusted@example.com"

				ok, err := svc.IsEmailAllowed(ctx, email)
				if err != nil {
					t.Fatalf("IsEmailAllowed (before add): %v", err)
				}
				if ok {
					t.Error("expected false before adding email")
				}

				if err := svc.AddAllowedEmail(ctx, email, 1, "test note"); err != nil {
					t.Fatalf("AddAllowedEmail: %v", err)
				}

				ok, err = svc.IsEmailAllowed(ctx, email)
				if err != nil {
					t.Fatalf("IsEmailAllowed (after add): %v", err)
				}
				if !ok {
					t.Error("expected true after adding email")
				}
			},
		},
		{
			name: "remove then IsEmailAllowed returns false",
			run: func(t *testing.T, svc auth.Service) {
				ctx := context.Background()
				email := "trusted@example.com"

				if err := svc.AddAllowedEmail(ctx, email, 1, ""); err != nil {
					t.Fatalf("AddAllowedEmail: %v", err)
				}
				if err := svc.RemoveAllowedEmail(ctx, email); err != nil {
					t.Fatalf("RemoveAllowedEmail: %v", err)
				}

				ok, err := svc.IsEmailAllowed(ctx, email)
				if err != nil {
					t.Fatalf("IsEmailAllowed (after remove): %v", err)
				}
				if ok {
					t.Error("expected false after removing email")
				}
			},
		},
		{
			name: "ListAllowedEmails returns all added emails",
			run: func(t *testing.T, svc auth.Service) {
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
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSvc(t)
			tt.run(t, svc)
		})
	}
}

// ─── GetUserByID ──────────────────────────────────────────────────────────────

func TestAuthService_GetUserByID(t *testing.T) {
	tests := []struct {
		name         string
		seedGoogleID string // if set, create this user first
		lookupID     int64
		wantErr      bool
		wantNotFound bool
	}{
		{
			name:         "returns error for unknown user ID",
			lookupID:     999,
			wantErr:      true,
			wantNotFound: true,
		},
		{
			name:         "returns user for known ID",
			seedGoogleID: "g-5",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSvc(t)
			ctx := context.Background()

			lookupID := tt.lookupID
			if tt.seedGoogleID != "" {
				u := mustCreateUser(t, svc, tt.seedGoogleID, "frank", "frank@example.com")
				lookupID = u.ID
			}

			fetched, err := svc.GetUserByID(ctx, lookupID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantNotFound && !auth.IsUserNotFound(err) {
					t.Errorf("expected ErrUserNotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetUserByID: %v", err)
			}
			if fetched.Email != "frank@example.com" {
				t.Errorf("email mismatch: got %s, want frank@example.com", fetched.Email)
			}
		})
	}
}

// ─── UpdateLastLogin ──────────────────────────────────────────────────────────

func TestAuthService_UpdateLastLogin(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		seedUser bool
		wantErr  bool
	}{
		{
			name:     "succeeds for existing user and sets LastLoginAt",
			seedUser: true,
			wantErr:  false,
		},
		{
			name:    "returns error for unknown user",
			userID:  9999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSvc(t)
			ctx := context.Background()

			uid := tt.userID
			if tt.seedUser {
				u := mustCreateUser(t, svc, "g-6", "grace", "grace@example.com")
				if u.LastLoginAt != nil {
					t.Fatal("expected nil LastLoginAt for brand-new user")
				}
				uid = u.ID
			}

			err := svc.UpdateLastLogin(ctx, uid)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateLastLogin: %v", err)
			}

			updated, err := svc.GetUserByID(ctx, uid)
			if err != nil {
				t.Fatalf("GetUserByID after UpdateLastLogin: %v", err)
			}
			if updated.LastLoginAt == nil {
				t.Error("expected non-nil LastLoginAt after UpdateLastLogin")
			}
		})
	}
}

// ─── SetUserAdmin / ListUsers ─────────────────────────────────────────────────

func TestAuthService_AdminManagement(t *testing.T) {
	tests := []struct {
		name      string
		setAdmin  bool
		wantAdmin bool
	}{
		{name: "set admin true", setAdmin: true, wantAdmin: true},
		{name: "set admin false (toggle off)", setAdmin: false, wantAdmin: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSvc(t)
			ctx := context.Background()

			u := mustCreateUser(t, svc, "g-7", "heidi", "heidi@example.com")
			if u.IsAdmin {
				t.Fatal("new user should not be admin by default")
			}

			if err := svc.SetUserAdmin(ctx, u.ID, tt.setAdmin); err != nil {
				t.Fatalf("SetUserAdmin(%v): %v", tt.setAdmin, err)
			}

			// Confirm via ListUsers.
			users, err := svc.ListUsers(ctx)
			if err != nil {
				t.Fatalf("ListUsers: %v", err)
			}
			var found bool
			for _, usr := range users {
				if usr.ID == u.ID {
					found = true
					if usr.IsAdmin != tt.wantAdmin {
						t.Errorf("IsAdmin: got %v, want %v", usr.IsAdmin, tt.wantAdmin)
					}
				}
			}
			if !found {
				t.Errorf("user %d not found in ListUsers", u.ID)
			}

			// Also confirm via GetUserByID.
			updated, err := svc.GetUserByID(ctx, u.ID)
			if err != nil {
				t.Fatalf("GetUserByID: %v", err)
			}
			if updated.IsAdmin != tt.wantAdmin {
				t.Errorf("GetUserByID IsAdmin: got %v, want %v", updated.IsAdmin, tt.wantAdmin)
			}
		})
	}
}

// ─── GetLoginURL ──────────────────────────────────────────────────────────────

func TestAuthService_GetLoginURL(t *testing.T) {
	tests := []struct {
		name    string
		fn      func(string) string
		state   string
		wantURL string
	}{
		{
			name:    "custom loginURLFn is called with state",
			fn:      func(state string) string { return "https://example.com/auth?state=" + state },
			state:   "test-state",
			wantURL: "https://example.com/auth?state=test-state",
		},
		{
			name:    "no loginURLFn returns empty string",
			fn:      nil,
			state:   "any",
			wantURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []auth.Option
			if tt.fn != nil {
				opts = append(opts, auth.WithLoginURLFn(tt.fn))
			}
			svc, _ := newSvc(t, opts...)

			got := svc.GetLoginURL(tt.state)
			if got != tt.wantURL {
				t.Errorf("GetLoginURL: got %q, want %q", got, tt.wantURL)
			}
		})
	}
}

// ─── StoreTokens ─────────────────────────────────────────────────────────────

func TestAuthService_StoreTokens(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()

	u := mustCreateUser(t, svc, "g-8", "ivan", "ivan@example.com")
	sess, err := svc.CreateSession(ctx, u.ID, "curl/7.0", "127.0.0.1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

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

// ─── Not-implemented stubs ────────────────────────────────────────────────────

func TestAuthService_NotImplemented(t *testing.T) {
	tests := []struct {
		name string
		call func(svc auth.Service) error
	}{
		{
			name: "ExchangeCodeForTokens returns AppError",
			call: func(svc auth.Service) error {
				_, err := svc.ExchangeCodeForTokens(context.Background(), "code")
				return err
			},
		},
		{
			name: "GetUserInfo returns error",
			call: func(svc auth.Service) error {
				_, err := svc.GetUserInfo(context.Background(), "access-token")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSvc(t)
			err := tt.call(svc)
			if err == nil {
				t.Fatal("expected error from not-implemented stub, got nil")
			}
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) {
				t.Errorf("expected *apperrors.AppError, got %T: %v", err, err)
			}
		})
	}
}
