package marketmovers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

func TestAuth_Login(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+pathLoginBasicAuth {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := tRPCResponse[AuthLoginResponse]{
			Result: struct {
				Data AuthLoginResponse `json:"data"`
			}{
				Data: AuthLoginResponse{
					AccessToken:  "test-token",
					RefreshToken: "test-refresh",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	auth := NewAuth(WithAuthBaseURL(server.URL))
	resp, err := auth.Login(context.Background(), "user", "pass")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.AccessToken != "test-token" {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "test-token")
	}
}

func TestAuth_RefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+pathRefreshToken {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := tRPCResponse[AuthRefreshResponse]{
			Result: struct {
				Data AuthRefreshResponse `json:"data"`
			}{
				Data: AuthRefreshResponse{
					AccessToken: "new-token",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	auth := NewAuth(WithAuthBaseURL(server.URL))
	resp, err := auth.RefreshToken(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if resp.AccessToken != "new-token" {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "new-token")
	}
}

func TestAuth_UsesHTTPXClient(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		resp := tRPCResponse[AuthLoginResponse]{
			Result: struct {
				Data AuthLoginResponse `json:"data"`
			}{
				Data: AuthLoginResponse{
					AccessToken:  "test-token",
					RefreshToken: "test-refresh",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := httpx.DefaultConfig("MarketMovers-Auth-Test")
	cfg.DefaultTimeout = 10 * time.Second
	cfg.RetryPolicy.MaxRetries = 1
	authClient := httpx.NewClient(cfg)

	auth := NewAuth(WithHTTPXClient(authClient), WithAuthBaseURL(server.URL))
	resp, err := auth.Login(context.Background(), "user", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken != "test-token" {
		t.Errorf("got AccessToken %q, want %q", resp.AccessToken, "test-token")
	}
	if !called.Load() {
		t.Error("expected server to be called")
	}
}
