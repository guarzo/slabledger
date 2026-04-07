package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

const defaultSearchURL = "https://search-zzvl7ri3bq-uc.a.run.app/search"

// Client accesses Card Ladder's Cloud Run search API.
type Client struct {
	searchURL   string
	rateLimiter *rate.Limiter
	httpClient  *httpx.Client

	// Token management
	mu           sync.Mutex
	token        TokenState
	auth         *FirebaseAuth
	refreshToken string

	// For testing: bypass token management
	staticToken string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the search endpoint URL (for testing).
func WithBaseURL(u string) ClientOption {
	return func(c *Client) { c.searchURL = u }
}

// WithStaticToken sets a fixed bearer token (for testing).
func WithStaticToken(t string) ClientOption {
	return func(c *Client) { c.staticToken = t }
}

// WithTokenManager configures automatic token refresh.
func WithTokenManager(auth *FirebaseAuth, refreshToken string, tokenExpiry time.Time) ClientOption {
	return func(c *Client) {
		c.auth = auth
		c.refreshToken = refreshToken
		c.token = TokenState{ExpiresAt: tokenExpiry}
	}
}

// NewClient creates a Card Ladder API client.
func NewClient(opts ...ClientOption) *Client {
	config := httpx.DefaultConfig("CardLadder")

	c := &Client{
		searchURL:   defaultSearchURL,
		rateLimiter: rate.NewLimiter(rate.Limit(1), 1), // 1 req/sec
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

// FetchCollectionPage fetches one page of collection cards.
func (c *Client) FetchCollectionPage(ctx context.Context, collectionID string, page, limit int) (*SearchResponse[CollectionCard], error) {
	params := url.Values{
		"index":     {"collectioncards"},
		"query":     {""},
		"page":      {strconv.Itoa(page)},
		"limit":     {strconv.Itoa(limit)},
		"filters":   {fmt.Sprintf("collectionId:%s|hasQuantityAvailable:true", collectionID)},
		"sort":      {"player"},
		"direction": {"asc"},
	}
	var resp SearchResponse[CollectionCard]
	if err := c.doGet(ctx, params, &resp); err != nil {
		return nil, fmt.Errorf("fetch collection page %d: %w", page, err)
	}
	return &resp, nil
}

// FetchAllCollection fetches all collection cards, paginating automatically.
func (c *Client) FetchAllCollection(ctx context.Context, collectionID string) ([]CollectionCard, error) {
	const pageSize = 100
	var all []CollectionCard
	for page := 0; ; page++ {
		resp, err := c.FetchCollectionPage(ctx, collectionID, page, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Hits...)
		if len(all) >= resp.TotalHits || len(resp.Hits) < pageSize {
			break
		}
	}
	return all, nil
}

// FetchSalesComps fetches sales comps for a card+grade.
func (c *Client) FetchSalesComps(ctx context.Context, gemRateID, condition, grader string, page, limit int) (*SearchResponse[SaleComp], error) {
	params := url.Values{
		"index":   {"salesarchive"},
		"query":   {""},
		"page":    {strconv.Itoa(page)},
		"limit":   {strconv.Itoa(limit)},
		"filters": {fmt.Sprintf("condition:%s|gemRateId:%s|gradingCompany:%s", condition, gemRateID, grader)},
		"sort":    {"date"},
	}
	var resp SearchResponse[SaleComp]
	if err := c.doGet(ctx, params, &resp); err != nil {
		return nil, fmt.Errorf("fetch sales comps for %s: %w", gemRateID, err)
	}
	return &resp, nil
}

// FetchCardCatalog searches the CL cards index (full catalog).
// Filters are key:value pairs (e.g. "condition:PSA 10") joined with "|".
// Results are sorted by score descending.
func (c *Client) FetchCardCatalog(ctx context.Context, query string, filters map[string]string, page, limit int) (*SearchResponse[CatalogCard], error) {
	params := url.Values{
		"index":     {"cards"},
		"query":     {query},
		"page":      {strconv.Itoa(page)},
		"limit":     {strconv.Itoa(limit)},
		"sort":      {"score"},
		"direction": {"desc"},
	}
	if len(filters) > 0 {
		keys := make([]string, 0, len(filters))
		for k := range filters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(filters))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s:%s", k, filters[k]))
		}
		params.Set("filters", strings.Join(parts, "|"))
	}

	var resp SearchResponse[CatalogCard]
	if err := c.doGet(ctx, params, &resp); err != nil {
		return nil, fmt.Errorf("fetch card catalog for %q: %w", query, err)
	}
	return &resp, nil
}

func (c *Client) doGet(ctx context.Context, params url.Values, result any) error {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("get auth token: %w", err)
	}

	u := c.searchURL + "?" + params.Encode()
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}

	resp, err := c.httpClient.Get(ctx, u, headers, 0)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}

	if err := json.Unmarshal(resp.Body, result); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

func (c *Client) getToken(ctx context.Context) (string, error) {
	if c.staticToken != "" {
		return c.staticToken, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid (with 5min buffer)
	if c.token.IDToken != "" && time.Now().Add(5*time.Minute).Before(c.token.ExpiresAt) {
		return c.token.IDToken, nil
	}

	if c.auth == nil || c.refreshToken == "" {
		return "", fmt.Errorf("no auth credentials configured")
	}

	resp, err := c.auth.RefreshToken(ctx, c.refreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}

	expSec, err := strconv.Atoi(resp.ExpiresIn)
	if err != nil || expSec <= 0 {
		expSec = 3600
	}
	c.token = TokenState{
		IDToken:   resp.IDToken,
		ExpiresAt: time.Now().Add(time.Duration(expSec) * time.Second),
	}
	if resp.RefreshToken != "" {
		c.refreshToken = resp.RefreshToken
	}
	return c.token.IDToken, nil
}

// SetToken directly sets the current token state (used during initial setup).
func (c *Client) SetToken(idToken string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = TokenState{IDToken: idToken, ExpiresAt: expiresAt}
}

// SetRefreshToken updates the stored refresh token (used after config save persists a new one).
func (c *Client) SetRefreshToken(refreshToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshToken = refreshToken
}

// UpdateCredentials atomically replaces the auth instance and refresh token.
// This avoids a race between writing auth fields and getToken reading them.
func (c *Client) UpdateCredentials(auth *FirebaseAuth, refreshToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth = auth
	c.refreshToken = refreshToken
	// Invalidate cached token so the next call uses the new credentials.
	c.token = TokenState{}
}
