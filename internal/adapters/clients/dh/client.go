package dh

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
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
	enterpriseKey string
	baseURL       string
	httpClient    *httpx.Client
	limiter       *rate.Limiter
	logger        observability.Logger
	timeout       time.Duration
	health        *HealthTracker
}

// NewClient creates a new DH API client.
func NewClient(baseURL string, opts ...ClientOption) *Client {
	config := httpx.DefaultConfig("DH")
	config.DefaultTimeout = defaultTimeout
	httpClient := httpx.NewClient(config)

	c := &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		limiter:    rate.NewLimiter(rate.Limit(defaultRateLimRPS), defaultRateLimRPS),
		timeout:    defaultTimeout,
		health:     NewHealthTracker(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Health returns the API health tracker for this client.
func (c *Client) Health() *HealthTracker {
	return c.health
}

// recordHealth safely records a success or failure, handling nil tracker.
func (c *Client) recordHealth(success bool) {
	if c.health == nil {
		return
	}
	if success {
		c.health.RecordSuccess()
	} else {
		c.health.RecordFailure()
	}
}

// CardLookup returns card details and market data from the enterprise API.
func (c *Client) CardLookup(ctx context.Context, cardID int) (*CardLookupResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/lookup?card_id=%d", c.baseURL, cardID)

	var resp CardLookupResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RecentSales returns recent sales for a card from the enterprise API.
func (c *Client) RecentSales(ctx context.Context, cardID int) ([]RecentSale, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/%d/recent-sales", c.baseURL, cardID)

	var resp struct {
		Sales []RecentSale `json:"sales"`
	}
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return resp.Sales, nil
}

// MarketDataEnterprise fetches market data from enterprise endpoints and
// assembles a MarketDataResponse compatible with existing consumer code.
func (c *Client) MarketDataEnterprise(ctx context.Context, cardID int) (*MarketDataResponse, error) {
	lookup, err := c.CardLookup(ctx, cardID)
	if err != nil {
		return nil, err
	}

	resp := &MarketDataResponse{
		HasData:   true,
		CardID:    lookup.Card.ID,
		CardTitle: lookup.Card.Name,
	}

	if lookup.MarketData.MidPrice != nil {
		resp.CurrentPrice = *lookup.MarketData.MidPrice
	}
	if lookup.MarketData.LowPrice != nil {
		resp.PeriodLow = *lookup.MarketData.LowPrice
	}
	if lookup.MarketData.HighPrice != nil {
		resp.PeriodHigh = *lookup.MarketData.HighPrice
	}

	sales, err := c.RecentSales(ctx, cardID)
	if err == nil {
		resp.RecentSales = sales
	}

	return resp, nil
}

// Suggestions returns AI-generated buy/sell suggestions via the enterprise API.
func (c *Client) Suggestions(ctx context.Context) (*SuggestionsResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/suggestions", c.baseURL)

	var resp SuggestionsResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
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
		c.recordHealth(false)
		return err
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		c.recordHealth(false)
		return apperrors.ProviderInvalidResponse(providerName, err)
	}
	c.recordHealth(true)
	return nil
}

// doEnterprise performs an authenticated enterprise API request with rate limiting.
// body may be nil for bodyless requests (e.g. GET-like deletes).
func (c *Client) doEnterprise(ctx context.Context, method, fullURL string, body any, dest any) error {
	if !c.EnterpriseAvailable() {
		return apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return apperrors.ProviderInvalidRequest(providerName, err)
		}
	}

	headers := map[string]string{
		enterpriseAuthHeader: "Bearer " + c.enterpriseKey,
		"Accept":             "application/json",
	}
	if len(bodyBytes) > 0 {
		headers["Content-Type"] = "application/json"
	}

	resp, err := c.httpClient.Do(ctx, httpx.Request{
		Method:  method,
		URL:     fullURL,
		Headers: headers,
		Body:    bodyBytes,
		Timeout: c.timeout,
	})
	if err != nil {
		c.recordHealth(false)
		return err
	}

	if dest != nil {
		if err := json.Unmarshal(resp.Body, dest); err != nil {
			c.recordHealth(false)
			return apperrors.ProviderInvalidResponse(providerName, err)
		}
	}
	c.recordHealth(true)
	return nil
}

func (c *Client) postEnterprise(ctx context.Context, fullURL string, body any, dest any) error {
	return c.doEnterprise(ctx, "POST", fullURL, body, dest)
}

func (c *Client) patchEnterprise(ctx context.Context, fullURL string, body any, dest any) error {
	return c.doEnterprise(ctx, "PATCH", fullURL, body, dest)
}

func (c *Client) deleteEnterprise(ctx context.Context, fullURL string, body any, dest any) error {
	return c.doEnterprise(ctx, "DELETE", fullURL, body, dest)
}

