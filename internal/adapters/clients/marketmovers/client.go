package marketmovers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

// Client accesses the Market Movers (Sports Card Investor) tRPC API.
type Client struct {
	baseURL     string
	rateLimiter *rate.Limiter
	httpClient  *httpx.Client

	// Token management
	mu           sync.Mutex
	token        TokenState
	auth         *Auth
	refreshToken string

	// For testing: bypass token management
	staticToken string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithClientBaseURL overrides the API base URL (for testing).
func WithClientBaseURL(u string) ClientOption {
	return func(c *Client) { c.baseURL = u }
}

// WithStaticToken sets a fixed bearer token (for testing).
func WithStaticToken(t string) ClientOption {
	return func(c *Client) { c.staticToken = t }
}

// WithTokenManager configures automatic token refresh.
func WithTokenManager(auth *Auth, refreshToken string, tokenExpiry time.Time) ClientOption {
	return func(c *Client) {
		c.auth = auth
		c.refreshToken = refreshToken
		c.token = TokenState{ExpiresAt: tokenExpiry}
	}
}

// NewClient creates a Market Movers API client.
func NewClient(opts ...ClientOption) *Client {
	config := httpx.DefaultConfig("MarketMovers")
	c := &Client{
		baseURL:     defaultAPIBaseURL,
		rateLimiter: rate.NewLimiter(rate.Limit(2), 2), // 2 req/sec
		httpClient:  httpx.NewClient(config),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Available returns true if the client has valid credentials.
func (c *Client) Available() bool {
	return c.staticToken != "" || (c.auth != nil && c.refreshToken != "")
}

// UpdateCredentials atomically replaces the auth instance and refresh token.
func (c *Client) UpdateCredentials(auth *Auth, refreshToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth = auth
	c.refreshToken = refreshToken
	c.token = TokenState{} // invalidate cached token
}

// SetToken directly sets the current token state (used during initial setup).
func (c *Client) SetToken(accessToken string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = TokenState{AccessToken: accessToken, ExpiresAt: expiresAt}
}

// SetRefreshToken updates the stored refresh token.
func (c *Client) SetRefreshToken(refreshToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshToken = refreshToken
}

// FetchActiveSales returns active listings for a set of collectible IDs.
// listingTypes may be nil to return all types (Auction, FixedPrice, etc.).
func (c *Client) FetchActiveSales(ctx context.Context, collectibleIDs []int64, listingTypes []string, page, limit int) (*ActiveSalesResponse, error) {
	filters := map[string]any{
		"collectibleType": "sports-card",
		"collectibleIds":  collectibleIDs,
	}
	if len(listingTypes) > 0 {
		filters["listingTypes"] = listingTypes
	}
	input := map[string]any{
		"filters": filters,
		"sort":    []map[string]string{{"sortBy": "endTime", "sortDirection": "asc"}},
		"limit":   limit,
		"offset":  page * limit,
	}
	var resp tRPCResponse[ActiveSalesResponse]
	if err := c.doQuery(ctx, "private.sales.active", input, &resp); err != nil {
		return nil, fmt.Errorf("fetch active sales: %w", err)
	}
	return &resp.Result.Data, nil
}

// FetchCompletedSales returns historical completed sales for a collectible ID.
func (c *Client) FetchCompletedSales(ctx context.Context, collectibleID int64, page, limit int) (*CompletedSalesResponse, error) {
	input := map[string]any{
		"collectibleType": "sports-card",
		"filters": map[string]any{
			"collectibleIds": []int64{collectibleID},
		},
		"sort":   []map[string]string{{"sortBy": "saleDate", "sortDirection": "desc"}},
		"limit":  limit,
		"offset": page * limit,
	}
	var resp tRPCResponse[CompletedSalesResponse]
	if err := c.doQuery(ctx, "private.sales.completed", input, &resp); err != nil {
		return nil, fmt.Errorf("fetch completed sales for %d: %w", collectibleID, err)
	}
	return &resp.Result.Data, nil
}

// FetchCompletedSummaries returns daily avg price summaries for a collectible.
// Returns the most recent `limit` days of data, sorted newest-first.
func (c *Client) FetchCompletedSummaries(ctx context.Context, collectibleID int64, limit int) (*CompletedSummariesResponse, error) {
	input := map[string]any{
		"collectibleType": "sports-card",
		"filters": map[string]any{
			"collectibleIds": []int64{collectibleID},
		},
		"sort":   []map[string]string{{"sortBy": "formattedDate", "sortDirection": "desc"}},
		"limit":  limit,
		"offset": 0,
	}
	var resp tRPCResponse[CompletedSummariesResponse]
	if err := c.doQuery(ctx, "private.sales.completedSummaries", input, &resp); err != nil {
		return nil, fmt.Errorf("fetch completed summaries for %d: %w", collectibleID, err)
	}
	return &resp.Result.Data, nil
}

// SearchCollectibles searches for card/collectible IDs by name query.
func (c *Client) SearchCollectibles(ctx context.Context, query string, page, limit int) (*CollectiblesSearchResponse, error) {
	input := map[string]any{
		"searchQueryText": query,
		"filters": map[string]any{
			"collectibleTypes": []string{"sports-card"},
		},
		"sort":   []map[string]string{{"sortBy": "stats.last14.totalSalesCount", "sortDirection": "desc"}},
		"limit":  limit,
		"offset": page * limit,
	}
	var resp tRPCResponse[CollectiblesSearchResponse]
	if err := c.doQuery(ctx, "private.collectibles.search", input, &resp); err != nil {
		return nil, fmt.Errorf("search collectibles for %q: %w", query, err)
	}
	return &resp.Result.Data, nil
}

// FetchDailyStats returns daily aggregated sale stats for a collectible from a given date onward.
func (c *Client) FetchDailyStats(ctx context.Context, collectibleID int64, dateFrom time.Time) (*DailyStatsResponse, error) {
	input := map[string]any{
		"collectibleType": "sports-card",
		"filters": map[string]any{
			"collectibleIds": []int64{collectibleID},
			"dateFromISO":    dateFrom.Format("2006-01-02"),
		},
		"aggregateBy": "day",
	}
	var resp tRPCResponse[DailyStatsResponse]
	if err := c.doQuery(ctx, "private.collectibles.stats.dailyStatsV2", input, &resp); err != nil {
		return nil, fmt.Errorf("fetch daily stats for %d: %w", collectibleID, err)
	}
	return &resp.Result.Data, nil
}

// AvgRecentPrice returns the count-weighted average sale price over the most recent nDays.
// Returns 0 if there are no sales in the window.
func (c *Client) AvgRecentPrice(ctx context.Context, collectibleID int64, nDays int) (float64, error) {
	dateFrom := time.Now().AddDate(0, 0, -nDays)
	stats, err := c.FetchDailyStats(ctx, collectibleID, dateFrom)
	if err != nil {
		return 0, err
	}
	var totalAmount float64
	var totalCount int
	for _, s := range stats.DailyStats {
		totalAmount += s.AverageSalePrice * float64(s.TotalSalesCount)
		totalCount += s.TotalSalesCount
	}
	if totalCount == 0 {
		return 0, nil
	}
	return totalAmount / float64(totalCount), nil
}

// AddCollectionItem adds a single item to the user's MM collection.
// Requires a valid collectibleId (from SearchCollectibles) and purchase details.
func (c *Client) AddCollectionItem(ctx context.Context, input AddCollectionItemInput) (*AddCollectionItemResponse, error) {
	var resp tRPCResponse[AddCollectionItemResponse]
	if err := c.doMutation(ctx, "private.collection.items.add", input, &resp); err != nil {
		return nil, fmt.Errorf("add collection item: %w", err)
	}
	return &resp.Result.Data, nil
}

// AddMultipleCollectionItems adds multiple items to the user's MM collection in a single request.
func (c *Client) AddMultipleCollectionItems(ctx context.Context, items []AddCollectionItemInput) (*AddMultipleCollectionItemsResponse, error) {
	var resp tRPCResponse[AddMultipleCollectionItemsResponse]
	if err := c.doMutation(ctx, "private.collection.items.addMultiple", items, &resp); err != nil {
		return nil, fmt.Errorf("add multiple collection items: %w", err)
	}
	return &resp.Result.Data, nil
}

// RemoveCollectionItem removes an item from the user's MM collection.
func (c *Client) RemoveCollectionItem(ctx context.Context, collectionItemID int64, collectibleType string) error {
	input := map[string]any{
		"collectionItemId": collectionItemID,
		"collectibleType":  collectibleType,
	}
	var resp tRPCResponse[map[string]any]
	if err := c.doMutation(ctx, "private.collection.items.remove", input, &resp); err != nil {
		return fmt.Errorf("remove collection item %d: %w", collectionItemID, err)
	}
	return nil
}

// checkTRPCError checks a response body for a tRPC error envelope and returns
// a formatted error if one is present, or nil otherwise.
func checkTRPCError(body []byte) error {
	var errCheck struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal(body, &errCheck); jsonErr == nil && errCheck.Error != nil {
		return fmt.Errorf("trpc error: %s", errCheck.Error.Message)
	}
	return nil
}

// doMutation executes a tRPC POST mutation (mutate procedure).
func (c *Client) doMutation(ctx context.Context, path string, input any, result any) error {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("get auth token: %w", err)
	}

	bodyBytes, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal mutation input: %w", err)
	}

	u := c.baseURL + "/" + path
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	resp, err := c.httpClient.Post(ctx, u, headers, bodyBytes, 0)
	if err != nil {
		// httpx returns both resp and err for HTTP 4xx/5xx — check for
		// tRPC validation error in the body before falling back to the
		// generic HTTP error.
		if resp != nil && len(resp.Body) > 0 {
			if trpcErr := checkTRPCError(resp.Body); trpcErr != nil {
				return trpcErr
			}
		}
		return fmt.Errorf("http request: %w", err)
	}

	// Check for tRPC error in response body before unmarshalling.
	if trpcErr := checkTRPCError(resp.Body); trpcErr != nil {
		return trpcErr
	}

	if err := json.Unmarshal(resp.Body, result); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

// doQuery executes a tRPC GET query (query procedure).
func (c *Client) doQuery(ctx context.Context, path string, input any, result any) error {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("get auth token: %w", err)
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}

	u := c.baseURL + "/" + path + "?input=" + url.QueryEscape(string(inputJSON))
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}

	resp, err := c.httpClient.Get(ctx, u, headers, 0)
	if err != nil {
		// httpx returns both resp and err for HTTP 4xx/5xx — check for
		// tRPC error in the body before falling back to the generic HTTP error.
		if resp != nil && len(resp.Body) > 0 {
			if trpcErr := checkTRPCError(resp.Body); trpcErr != nil {
				return trpcErr
			}
		}
		return fmt.Errorf("http request: %w", err)
	}

	// Check for tRPC error in response body before unmarshalling
	if trpcErr := checkTRPCError(resp.Body); trpcErr != nil {
		return trpcErr
	}

	if err := json.Unmarshal(resp.Body, result); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

// getToken returns a valid access token, refreshing if needed.
// Uses double-check locking so the lock is not held during the network call.
func (c *Client) getToken(ctx context.Context) (string, error) {
	if c.staticToken != "" {
		return c.staticToken, nil
	}

	c.mu.Lock()
	// Return cached token if still valid (with 5min buffer).
	if c.token.AccessToken != "" && time.Now().Add(5*time.Minute).Before(c.token.ExpiresAt) {
		token := c.token.AccessToken
		c.mu.Unlock()
		return token, nil
	}
	if c.auth == nil || c.refreshToken == "" {
		c.mu.Unlock()
		return "", fmt.Errorf("no auth credentials configured")
	}
	// Capture credentials under the lock before releasing it.
	auth := c.auth
	refreshTok := c.refreshToken
	c.mu.Unlock()

	// Perform the network call outside the lock to avoid holding it during I/O.
	resp, err := auth.RefreshToken(ctx, refreshTok)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}

	// Parse expiry from JWT payload (exp claim).
	expiry := parseJWTExpiry(resp.AccessToken)

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check: another goroutine may have refreshed the token while we
	// were waiting for the network call to complete.
	if c.token.AccessToken != "" && time.Now().Add(5*time.Minute).Before(c.token.ExpiresAt) {
		return c.token.AccessToken, nil
	}
	c.token = TokenState{
		AccessToken: resp.AccessToken,
		ExpiresAt:   expiry,
	}
	return c.token.AccessToken, nil
}

// ParseJWTExpiry extracts the exp claim from a JWT without verification.
// Exported so handlers can parse access token expiry after login.
func ParseJWTExpiry(token string) time.Time {
	return parseJWTExpiry(token)
}

// parseJWTExpiry extracts the exp claim from a JWT without verification.
func parseJWTExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Now().Add(55 * time.Minute) // safe default
	}
	// JWT payload is base64url encoded without padding.
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Now().Add(55 * time.Minute)
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil || claims.Exp == 0 {
		return time.Now().Add(55 * time.Minute)
	}
	return time.Unix(claims.Exp, 0)
}
