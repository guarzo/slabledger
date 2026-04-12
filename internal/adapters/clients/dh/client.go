package dh

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"strings"
	"sync"
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

// WithPSAKeys sets comma-separated PSA API keys for cert resolution.
// Keys are rotated when a PSA rate limit is encountered.
func WithPSAKeys(keys string) ClientOption {
	return func(c *Client) {
		c.psaKeys = parsePSAKeys(keys)
	}
}

// Client provides access to the DH market intelligence API.
type Client struct {
	enterpriseKey string
	psaKeys       []string   // PSA API keys for cert resolution rotation
	psaKeyIndex   int        // current key index; guarded by psaMu
	psaMu         sync.Mutex // protects psaKeyIndex
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
		opt(c)
	}
	return c
}

// Health returns the API health tracker for this client.
func (c *Client) Health() *HealthTracker {
	return c.health
}

// recordHealth safely records a success or failure, handling nil tracker.
func (c *Client) recordHealth(success bool) {
	if c.health != nil {
		if success {
			c.health.RecordSuccess()
		} else {
			c.health.RecordFailure()
		}
	}
}

// CardLookup returns card details and market data from the enterprise API.
func (c *Client) CardLookup(ctx context.Context, cardID int) (*CardLookupResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/lookup?card_id=%d", c.baseURL, cardID)

	var resp CardLookupResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	if resp.Card.ID <= 0 {
		return nil, apperrors.ProviderInvalidResponse(providerName,
			fmt.Errorf("card lookup returned non-positive ID for card_id=%d", cardID))
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
	for i, sale := range resp.Sales {
		if sale.Price <= 0 || strings.TrimSpace(sale.SoldAt) == "" {
			return nil, apperrors.ProviderInvalidResponse(providerName,
				fmt.Errorf("sale[%d] has non-positive price or empty date for card_id=%d", i, cardID))
		}
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
		CardID:    lookup.Card.ID,
		CardTitle: lookup.Card.Name,
	}

	if lookup.MarketData.MidPrice != nil {
		resp.CurrentPrice = *lookup.MarketData.MidPrice
		resp.HasData = true
	}
	if lookup.MarketData.LowPrice != nil {
		resp.PeriodLow = *lookup.MarketData.LowPrice
		resp.HasData = true
	}
	if lookup.MarketData.HighPrice != nil {
		resp.PeriodHigh = *lookup.MarketData.HighPrice
		resp.HasData = true
	}

	sales, err := c.RecentSales(ctx, cardID)
	if err != nil {
		if c.logger != nil {
			c.logger.Warn(ctx, "dh: recent sales fetch failed, returning partial market data",
				observability.Int("card_id", cardID), observability.Err(err))
		}
	} else if len(sales) > 0 {
		resp.RecentSales = sales
		resp.HasData = true
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

// waitForRateLimit waits for the rate limiter and returns context errors unchanged.
func (c *Client) waitForRateLimit(ctx context.Context) error {
	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}
	return nil
}

// getEnterprise performs a GET request with Bearer auth for the enterprise API.
func (c *Client) getEnterprise(ctx context.Context, fullURL string, dest any) error {
	if !c.EnterpriseAvailable() {
		return apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
	}

	if err := c.waitForRateLimit(ctx); err != nil {
		return err
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
// extraHeaders are merged into the request (may be nil).
func (c *Client) doEnterprise(ctx context.Context, method, fullURL string, body any, dest any, extraHeaders ...map[string]string) error {
	if !c.EnterpriseAvailable() {
		return apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
	}

	if err := c.waitForRateLimit(ctx); err != nil {
		return err
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
	for _, eh := range extraHeaders {
		for k, v := range eh {
			headers[k] = v
		}
	}

	if c.logger != nil {
		c.logger.Debug(ctx, "dh: enterprise request",
			observability.String("method", method),
			observability.String("url", fullURL),
			observability.Int("body_bytes", len(bodyBytes)))
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
		if c.logger != nil {
			c.logger.Error(ctx, "dh: enterprise request failed",
				observability.String("method", method),
				observability.String("url", fullURL),
				observability.Err(err))
		}
		return err
	}

	if c.logger != nil {
		c.logger.Debug(ctx, "dh: enterprise response",
			observability.Int("status_code", resp.StatusCode),
			observability.Int("body_bytes", len(resp.Body)))
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

// currentPSAKey returns the current PSA API key, or "" if none configured or all exhausted.
func (c *Client) currentPSAKey() string {
	c.psaMu.Lock()
	defer c.psaMu.Unlock()
	if len(c.psaKeys) == 0 || c.psaKeyIndex >= len(c.psaKeys) {
		return ""
	}
	return c.psaKeys[c.psaKeyIndex]
}

// RotatePSAKey advances to the next PSA API key. Returns true if there are
// more keys to try, false if we've cycled through all of them.
func (c *Client) RotatePSAKey() bool {
	c.psaMu.Lock()
	defer c.psaMu.Unlock()
	if len(c.psaKeys) <= 1 {
		return false
	}
	c.psaKeyIndex++
	return c.psaKeyIndex < len(c.psaKeys)
}

// ResetPSAKeyRotation resets the key index to the first key.
func (c *Client) ResetPSAKeyRotation() {
	c.psaMu.Lock()
	defer c.psaMu.Unlock()
	c.psaKeyIndex = 0
}

// PSAKeyRotator can rotate PSA API keys when rate limited.
type PSAKeyRotator interface {
	RotatePSAKey() bool
	ResetPSAKeyRotation()
}

// IsPSARateLimitError returns true if the error indicates DH's cert resolution
// hit a PSA API rate limit. These errors are returned as HTTP 422 and can be
// resolved by rotating to a different PSA API key.
func IsPSARateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "PSA API rate limit") ||
		strings.Contains(msg, "daily limit reached")
}

// ResolveCertWithRotation calls resolve, rotating PSA API keys on rate limit errors.
// rotateFn should be nil when the resolver doesn't support key rotation.
func ResolveCertWithRotation(
	ctx context.Context,
	req CertResolveRequest,
	resolve func(context.Context, CertResolveRequest) (*CertResolution, error),
	rotateFn func() bool,
	logger observability.Logger,
	logPrefix string,
) (*CertResolution, error) {
	resp, err := resolve(ctx, req)
	if err == nil || rotateFn == nil {
		return resp, err
	}

	for IsPSARateLimitError(err) {
		if !rotateFn() {
			return nil, err
		}
		logger.Info(ctx, logPrefix+": PSA key rate limited, rotated to next key",
			observability.String("cert", req.CertNumber))
		resp, err = resolve(ctx, req)
		if err == nil {
			return resp, nil
		}
	}

	return nil, err
}

// parsePSAKeys splits a comma-separated key string into trimmed, non-empty keys.
func parsePSAKeys(raw string) []string {
	var keys []string
	for _, k := range strings.Split(raw, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// GenerateInstagramPost initiates a DH-side Instagram post generation.
// Returns the numeric post_id for use with PollInstagramPostStatus.
// Requires EnterpriseAvailable() == true.
func (c *Client) GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error) {
	url := c.baseURL + "/api/v1/enterprise/instagram/generate"
	req := DHInstagramGenerateRequest{
		Scope:    scope,
		Strategy: strategy,
		Headline: headline,
	}
	var resp DHInstagramGenerateResponse
	if err := c.postEnterprise(ctx, url, req, &resp); err != nil {
		return 0, err
	}
	return resp.PostID, nil
}

// PollInstagramPostStatus returns the current render status and, when ready,
// the public slide image URLs for the given post_id.
// Requires EnterpriseAvailable() == true.
func (c *Client) PollInstagramPostStatus(ctx context.Context, postID int64) (*DHInstagramStatusResponse, error) {
	url := fmt.Sprintf("%s/api/v1/enterprise/instagram/posts/%d/status", c.baseURL, postID)
	var resp DHInstagramStatusResponse
	if err := c.getEnterprise(ctx, url, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
