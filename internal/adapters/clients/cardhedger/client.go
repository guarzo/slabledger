package cardhedger

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

const (
	baseURL = "https://api.cardhedger.com/v1"
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

// Client provides access to the CardHedger API.
type Client struct {
	apiKey      string
	baseURL     string
	httpClient  *httpx.Client
	rateLimiter *rate.Limiter
	logger      observability.Logger

	dailyCalls  *resilience.ResettingCounter
	minuteCalls *resilience.ResettingCounter

	// 429 tracking
	rateLimitHits429 *resilience.ResettingCounter
	last429Time      atomic.Int64 // unix timestamp
}

// NewClient creates a new CardHedger API client.
func NewClient(apiKey string, opts ...ClientOption) *Client {
	config := httpx.DefaultConfig("CardHedger")
	config.DefaultTimeout = 30 * time.Second
	httpClient := httpx.NewClient(config)

	c := &Client{
		apiKey:      apiKey,
		baseURL:     baseURL,
		httpClient:  httpClient,
		dailyCalls:       resilience.NewResettingCounter(24 * time.Hour),
		minuteCalls:      resilience.NewResettingCounter(time.Minute),
		rateLimitHits429: resilience.NewResettingCounter(24 * time.Hour),
		// 100 req/min with burst of 5
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

// Name returns the client name for logging.
func (c *Client) Name() string {
	return pricing.SourceCardHedger
}

// Close closes the client.
func (c *Client) Close() error {
	return nil
}

// DailyCallsUsed returns the approximate number of API calls made today.
func (c *Client) DailyCallsUsed() int {
	return int(c.dailyCalls.Load())
}

// MinuteCallsUsed returns the approximate number of API calls made this minute.
func (c *Client) MinuteCallsUsed() int {
	return int(c.minuteCalls.Load())
}

// RateLimitHits returns the number of 429 responses received since the last daily reset.
func (c *Client) RateLimitHits() int {
	return int(c.rateLimitHits429.Load())
}

// Last429Time returns the time of the last 429 response, or zero time if none.
func (c *Client) Last429Time() time.Time {
	ts := c.last429Time.Load()
	if ts == 0 {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}

func (c *Client) record429(ctx context.Context, path string) {
	c.rateLimitHits429.Inc()
	c.last429Time.Store(time.Now().Unix())
	if c.logger != nil {
		c.logger.Warn(ctx, "cardhedger 429 rate limit hit",
			observability.String("path", path),
			observability.Int("daily_calls", int(c.dailyCalls.Load())),
			observability.Int("minute_calls", int(c.minuteCalls.Load())),
			observability.Int("total_429s_today", int(c.rateLimitHits429.Load())))
	}
}

// SearchCard searches for cards matching the query.
func (c *Client) SearchCard(ctx context.Context, name, set, category string) (*CardSearchResponse, int, http.Header, error) {
	req := CardSearchRequest{
		Search:   name,
		Set:      set,
		Category: category,
		PageSize: 10,
	}
	var resp CardSearchResponse
	statusCode, headers, err := c.post(ctx, "/cards/card-search", req, &resp)
	if err != nil {
		return nil, statusCode, headers, err
	}
	return &resp, statusCode, headers, nil
}

// GetAllPrices returns latest prices across all grades for a card.
func (c *Client) GetAllPrices(ctx context.Context, cardID string) (*AllPricesByCardResponse, int, http.Header, error) {
	req := AllPricesByCardRequest{CardID: cardID}
	var resp AllPricesByCardResponse
	statusCode, headers, err := c.post(ctx, "/cards/all-prices-by-card", req, &resp)
	if err != nil {
		return nil, statusCode, headers, err
	}
	return &resp, statusCode, headers, nil
}

// BatchPriceEstimate returns price estimates for multiple card+grade pairs (max 100).
func (c *Client) BatchPriceEstimate(ctx context.Context, items []PriceEstimateItem) (*BatchPriceEstimateResponse, int, http.Header, error) {
	if len(items) > 100 {
		return nil, 0, nil, apperrors.ProviderInvalidRequest(c.Name(),
			fmt.Errorf("batch-price-estimate: %d items exceeds max 100", len(items)))
	}
	req := BatchPriceEstimateRequest{Items: items}
	var resp BatchPriceEstimateResponse
	statusCode, headers, err := c.post(ctx, "/cards/batch-price-estimate", req, &resp)
	if err != nil {
		return nil, statusCode, headers, err
	}
	return &resp, statusCode, headers, nil
}

// GetPriceUpdates returns price changes since the given timestamp (delta poll).
func (c *Client) GetPriceUpdates(ctx context.Context, since string) (*PriceUpdatesResponse, int, http.Header, error) {
	req := PriceUpdatesRequest{Since: since}
	var resp PriceUpdatesResponse
	statusCode, headers, err := c.post(ctx, "/cards/price-updates", req, &resp)
	if err != nil {
		return nil, statusCode, headers, err
	}
	return &resp, statusCode, headers, nil
}

// CardMatch uses AI-powered natural language matching to find a card.
func (c *Client) CardMatch(ctx context.Context, query, category string, maxCandidates int) (*CardMatchResponse, int, http.Header, error) {
	req := CardMatchRequest{
		Query:         query,
		Category:      category,
		MaxCandidates: maxCandidates,
	}
	var resp CardMatchResponse
	statusCode, headers, err := c.post(ctx, "/cards/card-match", req, &resp)
	if err != nil {
		return nil, statusCode, headers, err
	}
	return &resp, statusCode, headers, nil
}

// CardRequest submits a card request to CardHedger.
func (c *Client) CardRequest(ctx context.Context, req CardRequestBody) (*CardRequestResponse, int, http.Header, error) {
	var resp CardRequestResponse
	statusCode, headers, err := c.post(ctx, "/cards/card-request", req, &resp)
	if err != nil {
		return nil, statusCode, headers, err
	}
	return &resp, statusCode, headers, nil
}

// DetailsByCerts resolves cert numbers to card IDs in batch (max 100).
func (c *Client) DetailsByCerts(ctx context.Context, certs []string, grader string) (*DetailsByCertsResponse, int, http.Header, error) {
	if len(certs) > 100 {
		return nil, 0, nil, apperrors.ProviderInvalidRequest(c.Name(),
			fmt.Errorf("details-by-certs: %d certs exceeds max 100", len(certs)))
	}
	req := DetailsByCertsRequest{
		Certs:  certs,
		Grader: grader,
	}
	var resp DetailsByCertsResponse
	statusCode, headers, err := c.post(ctx, "/cards/details-by-certs", req, &resp)
	if err != nil {
		return nil, statusCode, headers, err
	}
	return &resp, statusCode, headers, nil
}

// post makes a POST request to the CardHedger API.
func (c *Client) post(ctx context.Context, path string, body any, result any) (int, http.Header, error) {
	if !c.Available() {
		return 0, nil, apperrors.ConfigMissing("cardhedger_api_key", "CARD_HEDGER_API_KEY")
	}

	if err := c.rateLimiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return 0, nil, err
		}
		return 0, nil, apperrors.ProviderUnavailable(c.Name(), err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return 0, nil, apperrors.ProviderInvalidRequest(c.Name(), err)
	}

	headers := map[string]string{
		"X-API-Key":    c.apiKey,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	fullURL := c.baseURL + path

	resp, err := c.httpClient.Post(ctx, fullURL, headers, bodyBytes, 30*time.Second)

	// Count every response from the server (including errors like 429)
	if resp != nil {
		c.dailyCalls.Inc()
		c.minuteCalls.Inc()
		if resp.StatusCode == 429 {
			c.record429(ctx, path)
		}
	}

	if err != nil {
		statusCode := 500
		if resp != nil {
			statusCode = resp.StatusCode
		}
		var respHeaders http.Header
		if resp != nil {
			respHeaders = resp.Headers
		}
		return statusCode, respHeaders, err
	}

	if err := json.Unmarshal(resp.Body, result); err != nil {
		return resp.StatusCode, resp.Headers, apperrors.ProviderInvalidResponse(c.Name(), err)
	}

	return resp.StatusCode, resp.Headers, nil
}
