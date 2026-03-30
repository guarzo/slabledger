package justtcg

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

const (
	defaultBaseURL = "https://api.justtcg.com/v1"
)

// ClientOption configures a Client after construction.
type ClientOption func(*Client)

// WithLogger sets the logger for structured 429 warnings.
func WithLogger(l observability.Logger) ClientOption {
	if l == nil {
		return func(*Client) {}
	}
	return func(c *Client) { c.logger = l }
}

// Client provides access to the JustTCG API.
type Client struct {
	apiKey      string
	baseURL     string
	httpClient  *httpx.Client
	rateLimiter *rate.Limiter
	logger      observability.Logger

	// Daily counter
	dailyCalls atomic.Int64
}

// NewClient creates a new JustTCG API client.
func NewClient(apiKey string, opts ...ClientOption) *Client {
	config := httpx.DefaultConfig("JustTCG")
	config.DefaultTimeout = 30 * time.Second
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     2,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		BackoffFactor:  2.0,
	}
	httpClient := httpx.NewClient(config)

	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: httpClient,
		// 100 req/min with burst of 5 (Pro plan)
		rateLimiter: rate.NewLimiter(rate.Limit(100.0/60.0), 5),
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

// DailyCalls returns the approximate number of API calls made today.
func (c *Client) DailyCalls() int64 {
	return c.dailyCalls.Load()
}

// SearchCards searches for cards matching the query and optional set filter.
// Calls GET /cards?q=...&game=pokemon&set=...&limit=20
func (c *Client) SearchCards(ctx context.Context, query, set string) ([]Card, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("game", "pokemon")
	params.Set("limit", "20")
	if set != "" {
		params.Set("set", set)
	}
	path := "/cards?" + params.Encode()

	var result cardsResponse
	if _, err := c.get(ctx, path, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// SearchSets searches for sets matching the query.
// Calls GET /sets?game=pokemon&q=...
func (c *Client) SearchSets(ctx context.Context, query string) ([]Set, error) {
	params := url.Values{}
	params.Set("game", "pokemon")
	params.Set("q", query)
	path := "/sets?" + params.Encode()

	var result setsResponse
	if _, err := c.get(ctx, path, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// BatchLookup fetches card details for a list of card IDs (max 100).
// Calls POST /cards with an array of {"cardId": "..."} objects.
// Returns nil for nil or empty input.
func (c *Client) BatchLookup(ctx context.Context, cardIDs []string) ([]Card, error) {
	if len(cardIDs) == 0 {
		return nil, nil
	}
	if len(cardIDs) > 100 {
		return nil, apperrors.ProviderInvalidRequest("justtcg",
			fmt.Errorf("batch-lookup: %d items exceeds max 100", len(cardIDs)))
	}

	items := make([]batchLookupItem, len(cardIDs))
	for i, id := range cardIDs {
		items[i] = batchLookupItem{CardID: id}
	}

	bodyBytes, err := json.Marshal(items)
	if err != nil {
		return nil, apperrors.ProviderInvalidRequest("justtcg", err)
	}

	var result cardsResponse
	if _, err := c.post(ctx, "/cards", bodyBytes, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// get makes a GET request to the JustTCG API.
func (c *Client) get(ctx context.Context, path string, result any) (int, error) {
	if !c.Available() {
		return 0, apperrors.ConfigMissing("justtcg_api_key", "JUSTTCG_API_KEY")
	}

	if err := c.rateLimiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return 0, err
		}
		return 0, apperrors.ProviderUnavailable("justtcg", err)
	}

	headers := map[string]string{
		"X-API-Key": c.apiKey,
		"Accept":    "application/json",
	}

	fullURL := c.baseURL + path
	resp, err := c.httpClient.Get(ctx, fullURL, headers, 30*time.Second)

	if resp != nil {
		c.dailyCalls.Add(1)
		if resp.StatusCode == http.StatusTooManyRequests {
			c.log429(ctx, path)
			retryAfter := ""
			if resp.Headers != nil {
				retryAfter = resp.Headers.Get("Retry-After")
			}
			return resp.StatusCode, apperrors.ProviderRateLimited("justtcg", retryAfter)
		}
	}

	if err != nil {
		statusCode := 500
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return statusCode, err
	}

	if err := json.Unmarshal(resp.Body, result); err != nil {
		return resp.StatusCode, apperrors.ProviderInvalidResponse("justtcg", err)
	}

	return resp.StatusCode, nil
}

// post makes a POST request to the JustTCG API.
func (c *Client) post(ctx context.Context, path string, body []byte, result any) (int, error) {
	if !c.Available() {
		return 0, apperrors.ConfigMissing("justtcg_api_key", "JUSTTCG_API_KEY")
	}

	if err := c.rateLimiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return 0, err
		}
		return 0, apperrors.ProviderUnavailable("justtcg", err)
	}

	headers := map[string]string{
		"X-API-Key":    c.apiKey,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	fullURL := c.baseURL + path
	resp, err := c.httpClient.Post(ctx, fullURL, headers, body, 30*time.Second)

	if resp != nil {
		c.dailyCalls.Add(1)
		if resp.StatusCode == http.StatusTooManyRequests {
			c.log429(ctx, path)
			retryAfter := ""
			if resp.Headers != nil {
				retryAfter = resp.Headers.Get("Retry-After")
			}
			return resp.StatusCode, apperrors.ProviderRateLimited("justtcg", retryAfter)
		}
	}

	if err != nil {
		statusCode := 500
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return statusCode, err
	}

	if err := json.Unmarshal(resp.Body, result); err != nil {
		return resp.StatusCode, apperrors.ProviderInvalidResponse("justtcg", err)
	}

	return resp.StatusCode, nil
}

func (c *Client) log429(ctx context.Context, path string) {
	if c.logger != nil {
		c.logger.Warn(ctx, "justtcg 429 rate limit hit",
			observability.String("path", path),
			observability.Int("daily_calls", int(c.dailyCalls.Load())))
	}
}
