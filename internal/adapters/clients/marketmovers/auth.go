package marketmovers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

const (
	defaultAPIBaseURL  = "https://d1ekdvyhrdz9i5.cloudfront.net/trpc"
	pathLoginBasicAuth = "auth.loginWithBasicAuth"
	pathRefreshToken   = "auth.getFreshAccessToken"
)

// Auth handles Market Movers authentication via the tRPC auth endpoints.
type Auth struct {
	baseURL    string
	httpClient *httpx.Client
}

// AuthOption configures an Auth instance.
type AuthOption func(*Auth)

// WithAuthBaseURL overrides the API base URL (for testing).
func WithAuthBaseURL(u string) AuthOption {
	return func(a *Auth) { a.baseURL = u }
}

// WithHTTPXClient overrides the default httpx.Client (for testing or custom config).
func WithHTTPXClient(c *httpx.Client) AuthOption {
	return func(a *Auth) { a.httpClient = c }
}

// NewAuth creates a new Market Movers auth client.
func NewAuth(opts ...AuthOption) *Auth {
	cfg := httpx.DefaultConfig("MarketMovers-Auth")
	cfg.DefaultTimeout = 10 * time.Second
	cfg.RetryPolicy.MaxRetries = 1

	a := &Auth{
		baseURL:    defaultAPIBaseURL,
		httpClient: httpx.NewClient(cfg),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Login authenticates with username/password and returns tokens.
// Credentials are WordPress credentials for sportscardinvestor.com.
func (a *Auth) Login(ctx context.Context, username, password string) (*AuthLoginResponse, error) {
	body := map[string]any{
		"username": username,
		"password": password,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal login body: %w", err)
	}

	fullURL := a.baseURL + "/" + pathLoginBasicAuth
	resp, err := a.httpClient.Post(ctx, fullURL, map[string]string{"Content-Type": "application/json"}, bodyBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("market movers login request: %w", err)
	}

	var envelope tRPCResponse[AuthLoginResponse]
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal login response: %w", err)
	}
	if envelope.Result.Data.AccessToken == "" {
		return nil, parseTRPCError(resp.Body, "login")
	}
	return &envelope.Result.Data, nil
}

// RefreshToken exchanges a refresh token for a new access token.
func (a *Auth) RefreshToken(ctx context.Context, refreshToken string) (*AuthRefreshResponse, error) {
	body := map[string]any{
		"refreshToken": refreshToken,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal refresh body: %w", err)
	}

	fullURL := a.baseURL + "/" + pathRefreshToken
	resp, err := a.httpClient.Post(ctx, fullURL, map[string]string{"Content-Type": "application/json"}, bodyBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("market movers refresh request: %w", err)
	}

	var envelope tRPCResponse[AuthRefreshResponse]
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal refresh response: %w", err)
	}
	if envelope.Result.Data.AccessToken == "" {
		return nil, parseTRPCError(resp.Body, "refresh")
	}
	return &envelope.Result.Data, nil
}

// parseTRPCError extracts the error message from a tRPC error body.
func parseTRPCError(body []byte, op string) error {
	var errResp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		return fmt.Errorf("market movers %s: %s", op, errResp.Error.Message)
	}
	return fmt.Errorf("market movers %s failed: empty response", op)
}
