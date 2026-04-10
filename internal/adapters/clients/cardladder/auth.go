package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

const (
	defaultAuthBaseURL  = "https://identitytoolkit.googleapis.com"
	defaultTokenBaseURL = "https://securetoken.googleapis.com"
)

// FirebaseAuth handles Firebase email/password authentication.
type FirebaseAuth struct {
	apiKey       string
	authBaseURL  string
	tokenBaseURL string
	httpClient   *httpx.Client
}

// AuthOption configures a FirebaseAuth instance.
type AuthOption func(*FirebaseAuth)

// WithAuthBaseURL overrides the Firebase Auth base URL (for testing).
func WithAuthBaseURL(u string) AuthOption {
	return func(a *FirebaseAuth) { a.authBaseURL = u }
}

// WithTokenBaseURL overrides the Firebase token refresh base URL (for testing).
func WithTokenBaseURL(u string) AuthOption {
	return func(a *FirebaseAuth) { a.tokenBaseURL = u }
}

// WithHTTPXClient overrides the default httpx client (for testing).
func WithHTTPXClient(c *httpx.Client) AuthOption {
	return func(a *FirebaseAuth) { a.httpClient = c }
}

// NewFirebaseAuth creates a Firebase Auth client.
func NewFirebaseAuth(apiKey string, opts ...AuthOption) *FirebaseAuth {
	cfg := httpx.DefaultConfig("CardLadder-Auth")
	cfg.DefaultTimeout = 10 * time.Second
	cfg.RetryPolicy.MaxRetries = 1

	a := &FirebaseAuth{
		apiKey:       apiKey,
		authBaseURL:  defaultAuthBaseURL,
		tokenBaseURL: defaultTokenBaseURL,
		httpClient:   httpx.NewClient(cfg),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Login authenticates with email/password and returns tokens.
func (a *FirebaseAuth) Login(ctx context.Context, email, password string) (*FirebaseAuthResponse, error) {
	reqBody := map[string]any{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal login body: %w", err)
	}

	fullURL := fmt.Sprintf("%s/v1/accounts:signInWithPassword?key=%s",
		a.authBaseURL, url.QueryEscape(a.apiKey))
	resp, err := a.httpClient.Post(ctx, fullURL, map[string]string{"Content-Type": "application/json"}, bodyBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("firebase login: %w", err)
	}

	var authResp FirebaseAuthResponse
	if err := json.Unmarshal(resp.Body, &authResp); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}
	if authResp.IDToken == "" || authResp.RefreshToken == "" {
		return nil, fmt.Errorf("empty token in login response")
	}
	return &authResp, nil
}

// RefreshToken exchanges a refresh token for a new ID token.
func (a *FirebaseAuth) RefreshToken(ctx context.Context, refreshToken string) (*FirebaseRefreshResponse, error) {
	formData := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	bodyBytes := []byte(formData.Encode())

	fullURL := fmt.Sprintf("%s/v1/token?key=%s",
		a.tokenBaseURL, url.QueryEscape(a.apiKey))
	resp, err := a.httpClient.Post(ctx, fullURL, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, bodyBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("firebase refresh: %w", err)
	}

	var refreshResp FirebaseRefreshResponse
	if err := json.Unmarshal(resp.Body, &refreshResp); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}
	if refreshResp.IDToken == "" {
		return nil, fmt.Errorf("empty id_token in refresh response")
	}
	return &refreshResp, nil
}
