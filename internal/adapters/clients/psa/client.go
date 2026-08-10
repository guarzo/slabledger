package psa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	defaultBaseURL = "https://api.psacard.com/publicapi"

	minRequestSpacing = 500 * time.Millisecond // avoid burst requests
)

// CertResponse is the PSA API response for cert lookup.
type CertResponse struct {
	PSACert CertInfo `json:"PSACert"`
}

// CertInfo contains PSA certificate details.
type CertInfo struct {
	CertNumber       string `json:"CertNumber"`
	SpecID           int    `json:"SpecID"`
	Year             string `json:"Year"`
	Brand            string `json:"Brand"`
	Category         string `json:"Category"`
	CardNumber       string `json:"CardNumber"`
	Subject          string `json:"Subject"`
	Variety          string `json:"Variety"`
	CardGrade        string `json:"CardGrade"`
	GradeDescription string `json:"GradeDescription"`
	TotalPopulation  int    `json:"TotalPopulation"`
	PopulationHigher int    `json:"PopulationHigher"`
}

// Client communicates with the PSA public API.
// Supports multiple API tokens for higher throughput — when one token is
// rejected or exhausts its daily quota, the client rotates to the next.
type Client struct {
	httpClient *httpx.Client
	baseURL    string
	pool       *tokenPool
	logger     observability.Logger
	lastCall   atomic.Int64 // unix nano of last API call
}

// NewClient creates a PSA API client.
// The token parameter may be a single key or comma-separated keys
// (e.g., "key1,key2") for automatic failover when one key is rejected or
// rate-limited.
func NewClient(token string, logger observability.Logger) *Client {
	var tokens []string
	for _, t := range strings.Split(token, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	httpCfg := httpx.DefaultConfig("PSA")
	httpCfg.DefaultTimeout = 15 * time.Second
	return &Client{
		httpClient: httpx.NewClient(httpCfg),
		baseURL:    defaultBaseURL,
		pool:       newTokenPool(tokens),
		logger:     logger,
	}
}

// exhaustedError builds the error returned when no token can be tried. The
// distinction matters to callers: a spent pool recovers at UTC rollover and is
// worth retrying, whereas an all-dead pool needs an operator to fix the keys
// and must not be presented to the user as "temporarily unavailable".
func (c *Client) exhaustedError() error {
	total, dead, spent := c.pool.stats()
	if spent > 0 {
		return apperrors.ProviderRateLimited("PSA", "")
	}
	if dead > 0 && dead == total {
		return apperrors.ProviderAuthFailed("PSA",
			fmt.Errorf("all %d PSA API keys rejected as unapproved", total))
	}
	return apperrors.ProviderRateLimited("PSA", "")
}

// doRequest handles the shared logic for PSA API calls: token selection,
// request pacing, rotation on 401/403/429, and response reading.
func (c *Client) doRequest(ctx context.Context, opName, path, certNumber string) (*httpx.Response, error) {
	total, _, _ := c.pool.stats()
	if total == 0 {
		return nil, apperrors.ConfigMissing("PSA API token", "PSA_API_TOKENS")
	}

	maxAttempts := c.pool.usableCount()
	if maxAttempts == 0 {
		return nil, c.exhaustedError()
	}

	// Capture the most recent 429 error from httpx — it carries the parsed
	// Retry-After header in its reset_time context. When every remaining token
	// is spent the loop exhausts, so we return this rather than a fresh empty
	// ProviderRateLimited that has lost the reset window.
	var lastRateLimitErr error
	// Likewise for auth: returning httpx's ProviderAuthFailed keeps the error
	// non-retryable, so a batch of unapproved keys is never dressed up as a
	// transient outage the caller should queue and retry.
	var lastAuthErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		token, ok := c.pool.next()
		if !ok {
			break
		}

		// Pace requests to avoid burst-triggered rate limits
		if last := c.lastCall.Load(); last > 0 {
			elapsed := time.Since(time.Unix(0, last))
			if elapsed < minRequestSpacing {
				select {
				case <-ctx.Done():
					// Wrap the bare context error so callers can classify it as
					// a transient timeout (retryable) rather than an opaque
					// failure. A naked ctx.Err() is context.Canceled/
					// DeadlineExceeded, which carries no provider error code.
					return nil, apperrors.ProviderTimeout("PSA", ctx.Err())
				case <-time.After(minRequestSpacing - elapsed):
				}
			}
		}
		c.lastCall.Store(time.Now().UnixNano())

		url := fmt.Sprintf("%s/%s/%s", c.baseURL, path, certNumber)
		headers := map[string]string{"Authorization": "Bearer " + token}

		// httpx returns both (*Response, error) on HTTP 4xx/5xx, allowing us
		// to inspect the status code even when err != nil.
		resp, err := c.httpClient.Get(ctx, url, headers, 0)
		if err == nil {
			return resp, nil
		}

		status := 0
		if resp != nil {
			status = resp.StatusCode
		}

		switch status {
		case http.StatusTooManyRequests:
			// The key is healthy but spent for the day. Only an approved key
			// gets a quota response at all, so this also confirms the key is
			// good — retire it until UTC rollover, don't discard it.
			lastRateLimitErr = err
			c.pool.markSpent(token)
			if c.pool.advance() {
				c.logger.Info(ctx, "PSA "+opName+": key quota exhausted, retrying with next key",
					observability.String("cert", certNumber))
				continue
			}
			c.logger.Warn(ctx, "PSA "+opName+": rate limited, no usable keys remain",
				observability.String("cert", certNumber))
			return nil, err

		case http.StatusUnauthorized, http.StatusForbidden:
			// PSA returns 403 "Access to this API is limited to approved
			// customers" for keys it never approved. That never resolves on its
			// own, so the key is retired for the process lifetime instead of
			// being retried on every call.
			lastAuthErr = err
			c.pool.markDead(token)
			t, d, s := c.pool.stats()
			c.logger.Warn(ctx, "PSA "+opName+": key rejected, retiring it",
				observability.String("cert", certNumber),
				observability.Int("http_status", status),
				observability.Int("keys_total", t),
				observability.Int("keys_dead", d),
				observability.Int("keys_spent", s))
			if c.pool.advance() {
				continue
			}
			return nil, err

		default:
			c.logger.Info(ctx, "PSA "+opName+": request failed",
				observability.String("cert", certNumber),
				observability.Err(err))
			return nil, apperrors.ProviderUnavailable("PSA", fmt.Errorf("PSA API request: %w", err))
		}
	}

	if lastRateLimitErr != nil {
		return nil, lastRateLimitErr
	}
	if lastAuthErr != nil {
		return nil, lastAuthErr
	}
	return nil, c.exhaustedError()
}

// GetCert looks up a PSA certificate by number.
func (c *Client) GetCert(ctx context.Context, certNumber string) (*CertInfo, error) {
	resp, err := c.doRequest(ctx, "cert lookup", "cert/GetByCertNumber", certNumber)
	if err != nil {
		return nil, err
	}

	var certResp CertResponse
	if decErr := json.Unmarshal(resp.Body, &certResp); decErr != nil {
		c.logger.Info(ctx, "PSA cert lookup: decode error",
			observability.String("cert", certNumber),
			observability.Err(decErr))
		return nil, apperrors.ProviderInvalidResponse("PSA", fmt.Errorf("decode response for cert %s: %w", certNumber, decErr))
	}

	if certResp.PSACert.CertNumber == "" {
		return nil, apperrors.ProviderNotFound("PSA", fmt.Sprintf("cert %s", certNumber))
	}
	if certResp.PSACert.CardGrade == "" {
		return nil, apperrors.ProviderInvalidResponse("PSA",
			fmt.Errorf("cert %s has empty CardGrade", certNumber))
	}

	c.logger.Info(ctx, "PSA cert lookup: success",
		observability.String("cert", certNumber),
		observability.String("subject", certResp.PSACert.Subject),
		observability.String("grade", certResp.PSACert.CardGrade))

	return &certResp.PSACert, nil
}

// ImageInfo represents a PSA slab image.
type ImageInfo struct {
	IsFrontImage bool   `json:"IsFrontImage"`
	ImageURL     string `json:"ImageURL"`
}

// GetImages fetches slab images for a PSA certificate.
func (c *Client) GetImages(ctx context.Context, certNumber string) ([]ImageInfo, error) {
	resp, err := c.doRequest(ctx, "image fetch", "cert/GetImagesByCertNumber", certNumber)
	if err != nil {
		return nil, err
	}

	var images []ImageInfo
	if decErr := json.Unmarshal(resp.Body, &images); decErr != nil {
		c.logger.Info(ctx, "PSA image fetch: decode error",
			observability.String("cert", certNumber),
			observability.Err(decErr))
		return nil, apperrors.ProviderInvalidResponse("PSA", fmt.Errorf("decode images response: %w", decErr))
	}

	return images, nil
}

// gradeRegex matches grade numbers (including half-grades) in PSA grade strings
// like "GEM MT 10", "NM-MT 8", "NM-MT 8.5", "MINT 9".
var gradeRegex = regexp.MustCompile(`\b(\d{1,2}(?:\.\d)?)\b`)

// ParseGrade extracts a numeric grade from a PSA grade string.
// Returns a float64 to support half-grades (e.g., "NM-MT 8.5" → 8.5).
func ParseGrade(gradeStr string) float64 {
	matches := gradeRegex.FindAllStringSubmatch(gradeStr, -1)
	if len(matches) == 0 {
		return 0
	}
	// Take the last number found (handles "NM-MT 8" -> 8, "GEM MT 10" -> 10)
	lastMatch := matches[len(matches)-1][1]
	grade, err := strconv.ParseFloat(lastMatch, 64)
	if err != nil || grade < 1 || grade > 10 {
		return 0
	}
	return grade
}

// BuildCardName constructs a display-friendly card name from PSA cert fields.
// Uses Subject (the core card identity) and appends Variety when present
// (e.g., "1ST EDITION", "SHADOWLESS") for accurate price lookups.
func BuildCardName(info *CertInfo) string {
	name := info.Subject
	if name == "" {
		name = info.Category
	}
	if name != "" && info.Variety != "" {
		name = name + " " + info.Variety
	}
	return name
}
