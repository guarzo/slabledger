package gsheets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const defaultBaseURL = "https://sheets.googleapis.com"

// Option configures a Client after construction.
type Option func(*Client)

// WithDataClient sets the httpx client used for Sheets API data calls.
func WithDataClient(c *httpx.Client) Option {
	return func(cl *Client) { cl.dataClient = c }
}

// WithAuthClient sets the httpx client used for OAuth token exchange.
func WithAuthClient(c *httpx.Client) Option {
	return func(cl *Client) { cl.authClient = c }
}

// WithBaseURL overrides the Sheets API base URL (useful for testing).
func WithBaseURL(u string) Option {
	return func(cl *Client) { cl.baseURL = u }
}

// Client reads data from Google Sheets using service account credentials.
type Client struct {
	dataClient   *httpx.Client
	authClient   *httpx.Client
	baseURL      string
	creds        *ServiceAccountCredentials
	token        *cachedToken
	refreshGroup singleflight.Group // deduplicates concurrent token refreshes
	logger       observability.Logger
}

// New creates a new Google Sheets client from service account JSON credentials.
func New(credentialsJSON string, logger observability.Logger, opts ...Option) (*Client, error) {
	creds, err := parseServiceAccountCredentials(credentialsJSON)
	if err != nil {
		return nil, fmt.Errorf("gsheets: %w", err)
	}

	dataCfg := httpx.DefaultConfig("GoogleSheets")
	dataCfg.DefaultTimeout = 30 * time.Second

	authCfg := httpx.DefaultConfig("GoogleSheets-Auth")
	authCfg.DefaultTimeout = 10 * time.Second
	authCfg.RetryPolicy.MaxRetries = 1

	c := &Client{
		dataClient: httpx.NewClient(dataCfg),
		authClient: httpx.NewClient(authCfg),
		baseURL:    defaultBaseURL,
		creds:      creds,
		token:      &cachedToken{},
		logger:     logger,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
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

	resp, err := c.dataClient.Get(ctx, apiURL, map[string]string{"Authorization": "Bearer " + token}, 0)
	if err != nil {
		return nil, fmt.Errorf("gsheets: fetch sheet: %w", err)
	}

	var result sheetsValueRange
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, apperrors.ProviderInvalidResponse("GoogleSheets", fmt.Errorf("decode response: %w", err))
	}

	if len(result.Values) == 0 {
		return nil, apperrors.ProviderInvalidResponse("GoogleSheets", fmt.Errorf("sheet is empty (no rows returned)"))
	}

	return result.Values, nil
}

// sheetsValueRange represents the Google Sheets API v4 ValueRange response.
type sheetsValueRange struct {
	Range  string     `json:"range"`
	Values [][]string `json:"values"`
}

// getToken returns a valid access token, refreshing if needed.
// Uses singleflight to ensure exactly one concurrent refresh operation.
func (c *Client) getToken(ctx context.Context) (string, error) {
	if !c.token.isExpired() {
		return c.token.get(), nil
	}

	v, err, _ := c.refreshGroup.Do("refresh", func() (any, error) {
		// Double-check after acquiring the singleflight lock — another goroutine
		// may have already refreshed while we waited.
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

		resp, err := c.authClient.Post(ctx, c.creds.TokenURI,
			map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			[]byte(formData.Encode()), 0)
		if err != nil {
			return "", fmt.Errorf("token exchange: %w", err)
		}

		var tokenResp tokenResponse
		if err := json.Unmarshal(resp.Body, &tokenResp); err != nil {
			return "", apperrors.ProviderInvalidResponse("GoogleSheets", fmt.Errorf("decode token response: %w", err))
		}

		if tokenResp.AccessToken == "" {
			return "", apperrors.ProviderAuthFailed("GoogleSheets", fmt.Errorf("token exchange returned empty access token"))
		}

		expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		c.token.set(tokenResp.AccessToken, expiry)

		c.logger.Info(ctx, "gsheets: refreshed access token",
			observability.String("email", c.creds.ClientEmail))

		return tokenResp.AccessToken, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}
