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

// mapRows converts flattened Lightdash rows into PSAExportRows. One malformed
// row must not abort the whole sync; log and skip it, matching the CSV import
// path (importPSARows). Rows without a cert number are silently dropped.
func mapRows(ctx context.Context, raw []map[string]string, logger observability.Logger) ([]inventory.PSAExportRow, error) {
	rows := make([]inventory.PSAExportRow, 0, len(raw))
	droppedNoCert := 0
	for _, r := range raw {
		m, err := mapRow(r)
		if err != nil {
			logger.Warn(ctx, "psaportal: skipping malformed row", observability.Err(err))
			continue
		}
		if m.CertNumber == "" {
			droppedNoCert++
			continue
		}
		rows = append(rows, m)
	}
	// Surface cert-less drops: if the Lightdash cert fieldId ever shifts, every
	// row loses its cert and silently drops, leaving the sync reporting success
	// with zero rows and no breadcrumb. A count makes that failure visible.
	if droppedNoCert > 0 {
		logger.Warn(ctx, "psaportal: dropped rows with no cert number",
			observability.Int("count", droppedNoCert), observability.Int("total", len(raw)))
	}
	if len(raw) > 0 && len(rows) == 0 {
		if droppedNoCert == len(raw) {
			return nil, fmt.Errorf("psaportal: all %d rows dropped for missing cert number (Lightdash cert fieldId may have shifted)", len(raw))
		}
		return nil, fmt.Errorf("psaportal: all %d rows failed to map", len(raw))
	}
	return rows, nil
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

// TokenProvider yields a valid PSA access token.
type TokenProvider interface {
	AccessToken(ctx context.Context) (string, error)
}

// Config configures a portal Client; an empty PSABaseURL falls back to the
// production default.
type Config struct {
	PSABaseURL string
}

// Client fetches and pushes PSA Buyer Campaign Manager campaign config. It is
// used only by the Cloudflare-trusted psa-harvest job — the campaign read/write
// hop to psacard.com lives entirely in the harvester, never the main app.
type Client struct {
	tokens     TokenProvider
	http       *httpx.Client
	psaBaseURL string
	logger     observability.Logger
}

// Option configures optional Client dependencies.
type Option func(*Client)

// WithLogger attaches a logger (used to report skipped malformed campaigns).
func WithLogger(l observability.Logger) Option {
	return func(c *Client) { c.logger = l }
}

// New builds a Client. tp supplies access tokens.
func New(tp TokenProvider, cfg Config, opts ...Option) *Client {
	if cfg.PSABaseURL == "" {
		cfg.PSABaseURL = defaultPSABaseURL
	}
	hc := httpx.DefaultConfig("PSAPortal")
	hc.DefaultTimeout = 30 * time.Second
	c := &Client{
		tokens:     tp,
		http:       httpx.NewClient(hc),
		psaBaseURL: cfg.PSABaseURL,
		logger:     observability.NewNoopLogger(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// baseURL returns the configured PSA portal base URL.
func (c *Client) baseURL() string { return c.psaBaseURL }
