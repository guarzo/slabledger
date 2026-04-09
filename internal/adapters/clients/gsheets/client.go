package gsheets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	defaultBaseURL = "https://sheets.googleapis.com"
	requestTimeout = 30 * time.Second
)

// Client reads data from Google Sheets using service account credentials.
type Client struct {
	httpClient *http.Client
	baseURL    string
	creds      *ServiceAccountCredentials
	token      *cachedToken
	logger     observability.Logger
}

// New creates a new Google Sheets client from service account JSON credentials.
func New(credentialsJSON string, logger observability.Logger) (*Client, error) {
	creds, err := parseServiceAccountCredentials(credentialsJSON)
	if err != nil {
		return nil, fmt.Errorf("gsheets: %w", err)
	}
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    defaultBaseURL,
		creds:      creds,
		token:      &cachedToken{},
		logger:     logger,
	}, nil
}

// ReadSheet fetches all values from the specified spreadsheet and sheet/tab.
// If sheetName is empty, it defaults to "Sheet1".
// Returns rows as [][]string, compatible with encoding/csv output.
func (c *Client) ReadSheet(ctx context.Context, spreadsheetID, sheetName string) ([][]string, error) {
	if sheetName == "" {
		sheetName = "Sheet1"
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("gsheets: auth: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v4/spreadsheets/%s/values/%s",
		c.baseURL,
		url.PathEscape(spreadsheetID),
		url.PathEscape(sheetName),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gsheets: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gsheets: fetch sheet: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gsheets: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gsheets: API returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var result sheetsValueRange
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("gsheets: decode response: %w", err)
	}

	if len(result.Values) == 0 {
		return nil, fmt.Errorf("gsheets: sheet is empty (no rows returned)")
	}

	return result.Values, nil
}

// sheetsValueRange represents the Google Sheets API v4 ValueRange response.
type sheetsValueRange struct {
	Range  string     `json:"range"`
	Values [][]string `json:"values"`
}

// getToken returns a valid access token, refreshing if needed.
func (c *Client) getToken(ctx context.Context) (string, error) {
	if !c.token.isExpired() {
		return c.token.get(), nil
	}

	jwt, err := buildJWT(c.creds)
	if err != nil {
		return "", err
	}

	formData := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jwt},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.creds.TokenURI,
		strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	c.token.set(tokenResp.AccessToken, expiry)

	c.logger.Info(ctx, "gsheets: refreshed access token",
		observability.String("email", c.creds.ClientEmail))

	return tokenResp.AccessToken, nil
}

// truncate returns the first n characters of s, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
