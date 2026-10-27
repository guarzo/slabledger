package cardladder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

func TestFirebaseLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts:signInWithPassword" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-api-key" {
			t.Fatalf("unexpected api key: %s", r.URL.Query().Get("key"))
		}
		json.NewEncoder(w).Encode(FirebaseAuthResponse{
			IDToken:      "test-id-token",
			RefreshToken: "test-refresh-token",
			ExpiresIn:    "3600",
		})
	}))
	defer server.Close()

	auth := NewFirebaseAuth("test-api-key", WithAuthBaseURL(server.URL))
	resp, err := auth.Login(context.Background(), "user@example.com", "password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.IDToken != "test-id-token" {
		t.Errorf("IDToken = %q, want %q", resp.IDToken, "test-id-token")
	}
	if resp.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", resp.RefreshToken, "test-refresh-token")
	}
}

func TestFirebaseRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(FirebaseRefreshResponse{
			IDToken:      "new-id-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    "3600",
		})
	}))
	defer server.Close()

	auth := NewFirebaseAuth("test-api-key", WithTokenBaseURL(server.URL))
	resp, err := auth.RefreshToken(context.Background(), "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if resp.IDToken != "new-id-token" {
		t.Errorf("IDToken = %q, want %q", resp.IDToken, "new-id-token")
	}
}

// Both Firebase endpoints must accept the request with the browser's Origin;
// neither should invent a Referer when the browser's policy omits it.
func TestFirebaseAuth_BrowserOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, path, contentType, response string
		call                              func(*FirebaseAuth) error
	}{
		{
			name: "login", path: "/v1/accounts:signInWithPassword", contentType: "application/json",
			response: `{"idToken":"fixture-id","refreshToken":"fixture-refresh"}`,
			call: func(auth *FirebaseAuth) error {
				_, err := auth.Login(context.Background(), "user@example.com", "fixture-password")
				return err
			},
		},
		{
			name: "refresh", path: "/v1/token", contentType: "application/x-www-form-urlencoded",
			response: `{"id_token":"fixture-id","refresh_token":"fixture-refresh"}`,
			call: func(auth *FirebaseAuth) error {
				_, err := auth.RefreshToken(context.Background(), "fixture-refresh")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Origin") != "https://app.cardladder.com" {
					http.Error(w, "browser origin required", http.StatusForbidden)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != tc.path {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Content-Type"); got != tc.contentType {
					t.Errorf("Content-Type = %q, want %q", got, tc.contentType)
				}
				if got := r.Header.Get("Referer"); got != "" {
					t.Errorf("unexpected Referer = %q", got)
				}
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			auth := NewFirebaseAuth("fixture-key", WithAuthBaseURL(server.URL), WithTokenBaseURL(server.URL))
			if err := tc.call(auth); err != nil {
				t.Fatalf("Firebase request with browser headers failed: %v", err)
			}
		})
	}
}

func TestFirebaseAuth_UsesHTTPXClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(FirebaseAuthResponse{
			IDToken:      "httpx-id-token",
			RefreshToken: "httpx-refresh-token",
			ExpiresIn:    "3600",
		})
	}))
	defer server.Close()

	cfg := httpx.DefaultConfig("CardLadder-Auth-Test")
	client := httpx.NewClient(cfg)

	auth := NewFirebaseAuth("test-api-key",
		WithAuthBaseURL(server.URL),
		WithHTTPXClient(client),
	)
	resp, err := auth.Login(context.Background(), "user@example.com", "password123")
	if err != nil {
		t.Fatalf("Login with httpx client failed: %v", err)
	}
	if resp.IDToken != "httpx-id-token" {
		t.Errorf("IDToken = %q, want %q", resp.IDToken, "httpx-id-token")
	}
}

func TestFirebaseLogin_EmptyTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(FirebaseAuthResponse{
			IDToken:      "",
			RefreshToken: "",
			ExpiresIn:    "3600",
		})
	}))
	defer server.Close()

	auth := NewFirebaseAuth("test-api-key", WithAuthBaseURL(server.URL))
	_, err := auth.Login(context.Background(), "user@example.com", "password123")
	if err == nil {
		t.Fatal("expected error for empty tokens, got nil")
	}
	if !strings.Contains(err.Error(), "empty token") {
		t.Errorf("expected error about empty token, got: %v", err)
	}
}
