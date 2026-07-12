package psaportal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// TokenProvider yields a valid PSA access token.
type TokenProvider interface {
	AccessToken(ctx context.Context) (string, error)
}

// Config configures a portal Client; empty URL fields fall back to production defaults.
type Config struct {
	PSABaseURL       string
	LightdashBaseURL string
}

// Client pulls per-cert purchase rows from the PSA portal.
type Client struct {
	tokens       TokenProvider
	http         *httpx.Client
	ld           *lightdashClient
	analyticsURL string
	logger       observability.Logger
}

// Option configures optional Client dependencies.
type Option func(*Client)

// WithLogger attaches a logger (used to report skipped malformed rows).
func WithLogger(l observability.Logger) Option {
	return func(c *Client) { c.logger = l }
}

// New builds a Client. tp supplies access tokens.
func New(tp TokenProvider, cfg Config, opts ...Option) *Client {
	if cfg.PSABaseURL == "" {
		cfg.PSABaseURL = defaultPSABaseURL
	}
	if cfg.LightdashBaseURL == "" {
		cfg.LightdashBaseURL = defaultLightdashBaseURL
	}
	hc := httpx.DefaultConfig("PSAPortal")
	hc.DefaultTimeout = 30 * time.Second
	c := &Client{
		tokens:       tp,
		http:         httpx.NewClient(hc),
		ld:           newLightdashClient(cfg.LightdashBaseURL),
		analyticsURL: cfg.PSABaseURL + analyticsPath,
		logger:       observability.NewNoopLogger(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// FetchRows walks the full portal chain and returns mapped purchase rows.
func (c *Client) FetchRows(ctx context.Context) ([]inventory.PSAExportRow, error) {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	embedURL, err := c.fetchEmbedURL(ctx, token)
	if err != nil {
		return nil, err
	}
	projectUUID, embedJWT, err := parseEmbedURL(embedURL)
	if err != nil {
		return nil, err
	}
	tileUUID, err := c.ld.findTileUUIDBySlug(ctx, projectUUID, embedJWT, itemizedPurchasesSlug)
	if err != nil {
		return nil, err
	}
	raw, err := c.ld.tileRows(ctx, projectUUID, embedJWT, tileUUID)
	if err != nil {
		return nil, err
	}
	return mapRows(ctx, raw, c.logger)
}

// mapRows converts flattened Lightdash rows into PSAExportRows. One malformed
// row must not abort the whole sync; log and skip it, matching the CSV import
// path (importPSARows). Rows without a cert number are silently dropped.
func mapRows(ctx context.Context, raw []map[string]string, logger observability.Logger) ([]inventory.PSAExportRow, error) {
	rows := make([]inventory.PSAExportRow, 0, len(raw))
	for _, r := range raw {
		m, err := mapRow(r)
		if err != nil {
			logger.Warn(ctx, "psaportal: skipping malformed row", observability.Err(err))
			continue
		}
		if m.CertNumber == "" {
			continue
		}
		rows = append(rows, m)
	}
	if len(raw) > 0 && len(rows) == 0 {
		return nil, fmt.Errorf("psaportal: all %d rows failed to map", len(raw))
	}
	return rows, nil
}

func (c *Client) fetchEmbedURL(ctx context.Context, token string) (string, error) {
	headers := map[string]string{
		"Cookie":     "accessToken=" + token,
		"User-Agent": browserUA,
		"Accept":     "application/json",
	}
	// httpx returns a non-nil resp alongside err for >=400 responses, so inspect
	// resp on the error path (that's where a Cloudflare 403/503 challenge lands).
	resp, err := c.http.Get(ctx, c.analyticsURL, headers, 0)
	if err != nil {
		if resp != nil && isCloudflareChallenge(resp) {
			return "", fmt.Errorf("psaportal: blocked by Cloudflare (status %d): %w", resp.StatusCode, err)
		}
		return "", fmt.Errorf("psaportal: analytics fetch: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("psaportal: analytics status %d", resp.StatusCode)
	}
	v, err := DecodeSvelteKitValue(resp.Body, "embedUrl")
	if err != nil {
		return "", err
	}
	return strings.Trim(string(v), `"`), nil
}

// isCloudflareChallenge detects a Cloudflare interstitial so the failure is unambiguous.
func isCloudflareChallenge(resp *httpx.Response) bool {
	if strings.EqualFold(resp.Headers.Get("Server"), "cloudflare") &&
		(resp.StatusCode == 403 || resp.StatusCode == 503) {
		return true
	}
	b := string(resp.Body)
	return strings.Contains(b, "Just a moment") || strings.Contains(b, "cf-challenge")
}

// parseEmbedURL splits "https://host/embed/{projectUUID}#{jwt}".
func parseEmbedURL(u string) (projectUUID, jwt string, err error) {
	base, token, found := strings.Cut(u, "#")
	if !found {
		return "", "", fmt.Errorf("psaportal: embed url missing token: %q", u)
	}
	jwt = token
	base = strings.TrimRight(base, "/")
	seg := strings.Split(base, "/")
	projectUUID = seg[len(seg)-1]
	if projectUUID == "" || jwt == "" {
		return "", "", fmt.Errorf("psaportal: cannot parse embed url: %q", u)
	}
	return projectUUID, jwt, nil
}
