package dh

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"net/url"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	defaultTimeout       = 30 * time.Second
	defaultRateLimRPS    = 1
	providerName         = "doubleholo"
	apiKeyHeader         = "X-Integration-API-Key"
	enterpriseAuthHeader = "Authorization"
)

// ClientOption configures a Client after construction.
type ClientOption func(*Client)

// WithLogger sets the logger for structured logging.
func WithLogger(l observability.Logger) ClientOption {
	if l == nil {
		return func(*Client) {}
	}
	return func(c *Client) { c.logger = l }
}

// WithRateLimitRPS sets the self-imposed rate limit in requests per second.
func WithRateLimitRPS(rps int) ClientOption {
	return func(c *Client) {
		if rps > 0 {
			c.limiter = rate.NewLimiter(rate.Limit(rps), rps)
		}
	}
}

// WithEnterpriseKey sets the Bearer token for enterprise API endpoints.
func WithEnterpriseKey(key string) ClientOption {
	return func(c *Client) { c.enterpriseKey = key }
}

// Client provides access to the DH market intelligence API.
type Client struct {
	apiKey        string
	enterpriseKey string
	baseURL       string
	httpClient    *httpx.Client
	limiter       *rate.Limiter
	logger        observability.Logger
	timeout       time.Duration
}

// NewClient creates a new DH API client.
func NewClient(baseURL, apiKey string, opts ...ClientOption) *Client {
	config := httpx.DefaultConfig("DH")
	config.DefaultTimeout = defaultTimeout
	httpClient := httpx.NewClient(config)

	c := &Client{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
		limiter:    rate.NewLimiter(rate.Limit(defaultRateLimRPS), defaultRateLimRPS),
		timeout:    defaultTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Available returns true if the API key is configured.
func (c *Client) Available() bool {
	return c.apiKey != ""
}

// Search queries the DH catalog for cards matching the query string.
func (c *Client) Search(ctx context.Context, query string, limit int) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	fullURL := fmt.Sprintf("%s/api/v1/integrations/catalog/search?%s", c.baseURL, params.Encode())

	var resp SearchResponse
	if err := c.get(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Match uses AI-powered matching to find the best card for a title/SKU pair.
func (c *Client) Match(ctx context.Context, title, sku string) (*MatchResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/integrations/match", c.baseURL)
	body := MatchRequest{
		Title: title,
		SKU:   sku,
	}

	var resp MatchResponse
	if err := c.post(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// MarketData returns market data for a specific card at tier3.
func (c *Client) MarketData(ctx context.Context, cardID string) (*MarketDataResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/integrations/market_data/%s?tier=tier3", c.baseURL, url.PathEscape(cardID))

	var resp MarketDataResponse
	if err := c.get(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Suggestions returns AI-generated buy/sell suggestions.
func (c *Client) Suggestions(ctx context.Context) (*SuggestionsResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/integrations/suggestions", c.baseURL)

	var resp SuggestionsResponse
	if err := c.get(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// get performs a GET request with rate limiting, auth headers, and JSON unmarshal.
func (c *Client) get(ctx context.Context, fullURL string, dest any) error {
	if !c.Available() {
		return apperrors.ConfigMissing("dh_api_key", "DH_INTEGRATION_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	headers := map[string]string{
		apiKeyHeader: c.apiKey,
		"Accept":     "application/json",
	}

	resp, err := c.httpClient.Get(ctx, fullURL, headers, c.timeout)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return apperrors.ProviderInvalidResponse(providerName, err)
	}
	return nil
}

// EnterpriseAvailable returns true if the enterprise API key is configured.
func (c *Client) EnterpriseAvailable() bool {
	return c.enterpriseKey != ""
}

// getEnterprise performs a GET request with Bearer auth for the enterprise API.
func (c *Client) getEnterprise(ctx context.Context, fullURL string, dest any) error {
	if !c.EnterpriseAvailable() {
		return apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	headers := map[string]string{
		enterpriseAuthHeader: "Bearer " + c.enterpriseKey,
		"Accept":             "application/json",
	}

	resp, err := c.httpClient.Get(ctx, fullURL, headers, c.timeout)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return apperrors.ProviderInvalidResponse(providerName, err)
	}
	return nil
}

// postEnterprise performs a POST request with Bearer auth for the enterprise API.
func (c *Client) postEnterprise(ctx context.Context, fullURL string, body any, dest any) error {
	if !c.EnterpriseAvailable() {
		return apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return apperrors.ProviderInvalidRequest(providerName, err)
	}

	headers := map[string]string{
		enterpriseAuthHeader: "Bearer " + c.enterpriseKey,
		"Content-Type":       "application/json",
		"Accept":             "application/json",
	}

	resp, err := c.httpClient.Post(ctx, fullURL, headers, bodyBytes, c.timeout)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return apperrors.ProviderInvalidResponse(providerName, err)
	}
	return nil
}

// post performs a POST request with rate limiting, auth headers, and JSON unmarshal.
func (c *Client) post(ctx context.Context, fullURL string, body any, dest any) error {
	if !c.Available() {
		return apperrors.ConfigMissing("dh_api_key", "DH_INTEGRATION_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return apperrors.ProviderInvalidRequest(providerName, err)
	}

	headers := map[string]string{
		apiKeyHeader:   c.apiKey,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	resp, err := c.httpClient.Post(ctx, fullURL, headers, bodyBytes, c.timeout)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return apperrors.ProviderInvalidResponse(providerName, err)
	}
	return nil
}
