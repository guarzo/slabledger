package cardladder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// parseFirebaseError attempts to extract an error message from a Firebase error response.
func parseFirebaseError(body []byte) string {
	var fbErr struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &fbErr) == nil {
		return fbErr.Error.Message
	}
	return ""
}

const (
	defaultAuthBaseURL  = "https://identitytoolkit.googleapis.com"
	defaultTokenBaseURL = "https://securetoken.googleapis.com"
)

// FirebaseAuth handles Firebase email/password authentication.
type FirebaseAuth struct {
	apiKey       string
	authBaseURL  string
	tokenBaseURL string
	httpClient   *http.Client
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

// NewFirebaseAuth creates a Firebase Auth client.
func NewFirebaseAuth(apiKey string, opts ...AuthOption) *FirebaseAuth {
	a := &FirebaseAuth{
		apiKey:       apiKey,
		authBaseURL:  defaultAuthBaseURL,
		tokenBaseURL: defaultTokenBaseURL,
		httpClient:   &http.Client{},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Login authenticates with email/password and returns tokens.
func (a *FirebaseAuth) Login(ctx context.Context, email, password string) (*FirebaseAuthResponse, error) {
	body := map[string]any{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal login body: %w", err)
	}

	u := fmt.Sprintf("%s/v1/accounts:signInWithPassword?key=%s",
		a.authBaseURL, url.QueryEscape(a.apiKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firebase login request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read login response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if msg := parseFirebaseError(respBody); msg != "" {
			return nil, fmt.Errorf("firebase login failed: %s (status %d)", msg, resp.StatusCode)
		}
		return nil, fmt.Errorf("firebase login failed (status %d)", resp.StatusCode)
	}

	var authResp FirebaseAuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return nil, fmt.Errorf("unmarshal login response: %w", err)
	}
	return &authResp, nil
}

// RefreshToken exchanges a refresh token for a new ID token.
func (a *FirebaseAuth) RefreshToken(ctx context.Context, refreshToken string) (*FirebaseRefreshResponse, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	u := fmt.Sprintf("%s/v1/token?key=%s",
		a.tokenBaseURL, url.QueryEscape(a.apiKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u,
		bytes.NewReader([]byte(form.Encode())))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firebase refresh request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if msg := parseFirebaseError(respBody); msg != "" {
			return nil, fmt.Errorf("firebase refresh failed: %s (status %d)", msg, resp.StatusCode)
		}
		return nil, fmt.Errorf("firebase refresh failed (status %d)", resp.StatusCode)
	}

	var refreshResp FirebaseRefreshResponse
	if err := json.Unmarshal(respBody, &refreshResp); err != nil {
		return nil, fmt.Errorf("unmarshal refresh response: %w", err)
	}
	return &refreshResp, nil
}
