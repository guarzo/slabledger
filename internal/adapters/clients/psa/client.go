package psa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// tokenDayKey uniquely identifies a token + UTC day combination.
type tokenDayKey struct {
	token string
	day   string // UTC date string "2006-01-02"
}

// tokenDayCounter tracks API calls per token per UTC day.
type tokenDayCounter struct {
	mu     sync.Mutex
	counts map[tokenDayKey]int32
}

func newTokenDayCounter() *tokenDayCounter {
	return &tokenDayCounter{counts: make(map[tokenDayKey]int32)}
}

// add increments and returns the new count for the given token on the current UTC day.
// Automatically rolls over when the date changes.
func (c *tokenDayCounter) add(token string) int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	today := time.Now().UTC().Format("2006-01-02")
	key := tokenDayKey{token: token, day: today}

	// Clean stale entries from previous days
	for k := range c.counts {
		if k.day != today {
			delete(c.counts, k)
		}
	}

	c.counts[key]++
	return c.counts[key]
}

const (
	defaultBaseURL = "https://api.psacard.com/publicapi"

	// PSA API allows 100 calls per day. Stop making calls after this threshold
	// to leave headroom for manual lookups.
	dailyCallLimit    = 90
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
// Supports multiple API tokens for higher throughput — when one token
// hits its daily limit, the client rotates to the next.
type Client struct {
	httpClient  *httpx.Client
	baseURL     string
	tokens      []string // one or more API tokens
	logger      observability.Logger
	dailyCounts *tokenDayCounter
	lastCall    atomic.Int64 // unix nano of last API call
	tokenIdx    atomic.Int32 // current token index
}

// NewClient creates a PSA API client.
// The token parameter may be a single key or comma-separated keys
// (e.g., "key1,key2") for automatic failover when one key is rate-limited.
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
		httpClient:  httpx.NewClient(httpCfg),
		baseURL:     defaultBaseURL,
		tokens:      tokens,
		logger:      logger,
		dailyCounts: newTokenDayCounter(),
	}
}

// currentToken returns the active API token.
func (c *Client) currentToken() string {
	if len(c.tokens) == 0 {
		return ""
	}
	idx := int(c.tokenIdx.Load()) % len(c.tokens)
	return c.tokens[idx]
}

// rotateToken switches to the next API token (when the current one is rate-limited).
func (c *Client) rotateToken() bool {
	if len(c.tokens) <= 1 {
		return false
	}
	newIdx := (c.tokenIdx.Add(1)) % int32(len(c.tokens))
	c.logger.Info(context.Background(), "rotating to next PSA API key",
		observability.Int("key_index", int(newIdx)),
		observability.Int("total_keys", len(c.tokens)))
	return true
}

// doRequest handles the shared logic for PSA API calls: budget enforcement,
// request pacing, token rotation on 429, and response reading.
func (c *Client) doRequest(ctx context.Context, opName, path, certNumber string) (*httpx.Response, error) {
	if len(c.tokens) == 0 {
		return nil, fmt.Errorf("PSA API token not configured")
	}

	maxAttempts := len(c.tokens)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		token := c.currentToken()
		calls := c.dailyCounts.add(token)
		if calls > dailyCallLimit {
			c.logger.Warn(ctx, "PSA daily call limit reached for "+opName,
				observability.String("cert", certNumber),
				observability.Int("calls", int(calls)))
			if c.rotateToken() {
				continue
			}
			return nil, fmt.Errorf("PSA daily call limit (%d) reached", dailyCallLimit)
		}

		// Pace requests to avoid burst-triggered rate limits
		if last := c.lastCall.Load(); last > 0 {
			elapsed := time.Since(time.Unix(0, last))
			if elapsed < minRequestSpacing {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(minRequestSpacing - elapsed):
				}
			}
		}
		c.lastCall.Store(time.Now().UnixNano())

		url := fmt.Sprintf("%s/%s/%s", c.baseURL, path, certNumber)
		headers := map[string]string{"Authorization": "Bearer " + token}

		resp, err := c.httpClient.Get(ctx, url, headers, 15*time.Second)
		if err != nil {
			// 429: rotate to the next token and retry rather than giving up immediately.
			if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
				if c.rotateToken() {
					c.logger.Info(ctx, "PSA "+opName+": rate limited, retrying with backup key",
						observability.String("cert", certNumber))
					continue
				}
				c.logger.Warn(ctx, "PSA "+opName+": rate limited, no backup keys available",
					observability.String("cert", certNumber))
				return nil, fmt.Errorf("PSA API returned 429 for cert %s (all keys exhausted)", certNumber)
			}
			c.logger.Info(ctx, "PSA "+opName+": request failed",
				observability.String("cert", certNumber),
				observability.Err(err))
			return nil, fmt.Errorf("PSA API request: %w", err)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("PSA API returned 429 for cert %s (all keys exhausted)", certNumber)
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
		return nil, fmt.Errorf("decode PSA response: %w", decErr)
	}

	if certResp.PSACert.CertNumber == "" {
		return nil, fmt.Errorf("cert %s not found", certNumber)
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
		return nil, fmt.Errorf("decode PSA images response: %w", decErr)
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
