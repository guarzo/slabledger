// Package psaexchange provides an HTTP adapter for the PSA-Exchange catalog API.
package psaexchange

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	domain "github.com/guarzo/slabledger/internal/domain/psaexchange"
)

const defaultBaseURL = "https://psa-exchange-catalog.com"
const defaultTimeout = 30 * time.Second

// Client is the HTTP adapter implementing domain.CatalogClient.
type Client struct {
	http      *httpx.Client
	baseURL   string
	token     string
	buyerCID  string
	timeout   time.Duration
	userAgent string
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the default upstream base URL.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithToken sets the buyer access token (required for CategoryURL only).
func WithToken(t string) Option { return func(c *Client) { c.token = t } }

// WithBuyerCID sets the buyer CID (reserved for v2 watchlist/offers).
func WithBuyerCID(cid string) Option { return func(c *Client) { c.buyerCID = cid } }

// WithTimeout overrides the per-request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// NewClient builds a PSA-Exchange adapter on top of the project's httpx client.
func NewClient(h *httpx.Client, opts ...Option) *Client {
	c := &Client{
		http:      h,
		baseURL:   defaultBaseURL,
		timeout:   defaultTimeout,
		userAgent: "slabledger/1.0",
	}
	for _, o := range opts {
		if o != nil {
			o(c)
		}
	}
	return c
}

// FetchCatalog calls GET /api/catalog.
func (c *Client) FetchCatalog(ctx context.Context) (domain.Catalog, error) {
	var out domain.Catalog
	u := c.baseURL + "/api/catalog"
	if err := c.http.GetJSON(ctx, u, c.headers(), c.timeout, &out); err != nil {
		return domain.Catalog{}, fmt.Errorf("psaexchange: fetch catalog: %w", err)
	}
	return out, nil
}

// FetchCardLadder calls GET /api/cardladder?cert=<cert>.
func (c *Client) FetchCardLadder(ctx context.Context, cert string) (domain.CardLadder, error) {
	var out domain.CardLadder
	q := url.Values{"cert": []string{cert}}
	u := c.baseURL + "/api/cardladder?" + q.Encode()
	if err := c.http.GetJSON(ctx, u, c.headers(), c.timeout, &out); err != nil {
		return domain.CardLadder{}, fmt.Errorf("psaexchange: fetch cardladder %s: %w", cert, err)
	}
	return out, nil
}

// CategoryURL returns the public catalog URL filtered to the given category.
// Returns "" when no token is configured.
func (c *Client) CategoryURL(category string) string {
	if c.token == "" {
		return ""
	}
	q := url.Values{"cat": []string{category}}
	return fmt.Sprintf("%s/catalog/%s?%s", c.baseURL, c.token, q.Encode())
}

// Token returns the configured token.
func (c *Client) Token() string { return c.token }

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string { return c.baseURL }

func (c *Client) headers() map[string]string {
	return map[string]string{
		"User-Agent": c.userAgent,
		"Accept":     "application/json",
	}
}

// Compile-time check that *Client implements domain.CatalogClient.
var _ domain.CatalogClient = (*Client)(nil)
