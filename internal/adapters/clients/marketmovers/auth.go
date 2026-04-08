package marketmovers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultAPIBaseURL  = "https://d1ekdvyhrdz9i5.cloudfront.net/trpc"
	pathLoginBasicAuth = "auth.loginWithBasicAuth"
	pathRefreshToken   = "auth.getFreshAccessToken"
)

// Auth handles Market Movers authentication via the tRPC auth endpoints.
type Auth struct {
	baseURL    string
	httpClient *http.Client
}

// AuthOption configures an Auth instance.
type AuthOption func(*Auth)

// WithAuthBaseURL overrides the API base URL (for testing).
func WithAuthBaseURL(u string) AuthOption {
	return func(a *Auth) { a.baseURL = u }
}

// NewAuth creates a new Market Movers auth client.
func NewAuth(opts ...AuthOption) *Auth {
	a := &Auth{
		baseURL:    defaultAPIBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/"+pathLoginBasicAuth, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("market movers login request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read login response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("market movers login failed (status %d): %s", resp.StatusCode, truncate(respBody, 200))
	}

	var envelope tRPCResponse[AuthLoginResponse]
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal login response: %w", err)
	}
	if envelope.Result.Data.AccessToken == "" {
		// Might be a tRPC error body
		return nil, parseTRPCError(respBody, "login")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/"+pathRefreshToken, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("market movers refresh request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("market movers refresh failed (status %d): %s", resp.StatusCode, truncate(respBody, 200))
	}

	var envelope tRPCResponse[AuthRefreshResponse]
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal refresh response: %w", err)
	}
	if envelope.Result.Data.AccessToken == "" {
		return nil, parseTRPCError(respBody, "refresh")
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

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
