package psaportal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// tokenProvider yields a valid PSA access token (implemented later by the OAuth token manager).
type tokenProvider interface {
	accessToken(ctx context.Context) (string, error)
}

// Config configures a portal Client; empty URL fields fall back to production defaults.
type Config struct {
	PSABaseURL       string
	LightdashBaseURL string
}

// Client pulls per-cert purchase rows from the PSA portal.
type Client struct {
	tokens       tokenProvider
	http         *httpx.Client
	ld           *lightdashClient
	analyticsURL string
}

// New builds a Client. tp supplies access tokens.
func New(tp tokenProvider, cfg Config) *Client {
	if cfg.PSABaseURL == "" {
		cfg.PSABaseURL = defaultPSABaseURL
	}
	if cfg.LightdashBaseURL == "" {
		cfg.LightdashBaseURL = defaultLightdashBaseURL
	}
	hc := httpx.DefaultConfig("PSAPortal")
	hc.DefaultTimeout = 30 * time.Second
	return &Client{
		tokens:       tp,
		http:         httpx.NewClient(hc),
		ld:           newLightdashClient(cfg.LightdashBaseURL),
		analyticsURL: cfg.PSABaseURL + analyticsPath,
	}
}

// FetchRows walks the full portal chain and returns mapped purchase rows.
func (c *Client) FetchRows(ctx context.Context) ([]inventory.PSAExportRow, error) {
	token, err := c.tokens.accessToken(ctx)
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
	rows := make([]inventory.PSAExportRow, 0, len(raw))
	for _, r := range raw {
		m, err := mapRow(r)
		if err != nil {
			return nil, err
		}
		if m.CertNumber == "" {
			continue
		}
		rows = append(rows, m)
	}
	return rows, nil
}

func (c *Client) fetchEmbedURL(ctx context.Context, token string) (string, error) {
	headers := map[string]string{
		"Cookie":     "accessToken=" + token,
		"User-Agent": browserUA,
		"Accept":     "application/json",
	}
	resp, err := c.http.Get(ctx, c.analyticsURL, headers, 0)
	if err != nil {
		return "", fmt.Errorf("psaportal: analytics fetch: %w", err)
	}
	if resp.StatusCode != 200 {
		if isCloudflareChallenge(resp) {
			return "", fmt.Errorf("psaportal: blocked by Cloudflare (status %d)", resp.StatusCode)
		}
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
	hash := strings.IndexByte(u, '#')
	if hash < 0 {
		return "", "", fmt.Errorf("psaportal: embed url missing token: %q", u)
	}
	jwt = u[hash+1:]
	base := strings.TrimRight(u[:hash], "/")
	seg := strings.Split(base, "/")
	projectUUID = seg[len(seg)-1]
	if projectUUID == "" || jwt == "" {
		return "", "", fmt.Errorf("psaportal: cannot parse embed url: %q", u)
	}
	return projectUUID, jwt, nil
}
