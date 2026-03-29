package cardladder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
