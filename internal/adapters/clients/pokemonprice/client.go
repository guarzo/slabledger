package pokemonprice

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/constants"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	baseAPIURL = "https://www.pokemonpricetracker.com/api/v2"

	// DailyLimit is the PokemonPrice API daily call budget (200k credits / 2 credits per includeEbay call).
	// Business tier: 200,000 credits/day, 500 calls/minute.
	DailyLimit = 100000
)

// Client provides access to the PokemonPriceTracker API
type Client struct {
	apiKey      string
	httpClient  *httpx.Client
	rateLimiter *rate.Limiter
	logger      observability.Logger
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithLogger sets an optional logger for debug diagnostics.
func WithLogger(logger observability.Logger) ClientOption {
	return func(c *Client) { c.logger = logger }
}

// NewClient creates a new PokemonPriceTracker API client
func NewClient(apiKey string, opts ...ClientOption) *Client {
	config := httpx.DefaultConfig("PokemonPriceTracker")
	config.DefaultTimeout = 15 * time.Second
	httpClient := httpx.NewClient(config)

	c := &Client{
		apiKey:     apiKey,
		httpClient: httpClient,
		// Business tier: 500 calls/min ≈ 8.3/sec; use 8/sec with burst of 4
		rateLimiter: rate.NewLimiter(rate.Limit(8), 4),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Available returns true if the API key is configured
func (c *Client) Available() bool {
	return c.apiKey != ""
}

// Name returns the client name for logging
func (c *Client) Name() string {
	return "pokemonprice"
}

// Close closes the client and releases resources
func (c *Client) Close() error {
	return nil
}

// GetPrice fetches pricing data for a Pokemon card (without eBay graded data).
// Returns the card data, HTTP status code, response headers, and error.
func (c *Client) GetPrice(ctx context.Context, setName, cardName, cardNumber string) (*CardPriceData, int, http.Header, error) {
	return c.getPrice(ctx, setName, cardName, cardNumber, false)
}

// GetPriceWithGraded fetches pricing data including eBay graded sales data.
// Costs 2 API credits instead of 1. Returns rich per-grade data in CardPriceData.Ebay.
func (c *Client) GetPriceWithGraded(ctx context.Context, setName, cardName, cardNumber string) (*CardPriceData, int, http.Header, error) {
	return c.getPrice(ctx, setName, cardName, cardNumber, true)
}

func (c *Client) getPrice(ctx context.Context, setName, cardName, cardNumber string, includeEbay bool) (*CardPriceData, int, http.Header, error) {
	if !c.Available() {
		return nil, 0, nil, apperrors.ConfigMissing("pokemonprice_api_key", "POKEMONPRICE_TRACKER_API_KEY")
	}

	// Normalize card name through the standard PSA pipeline:
	// NormalizePurchaseName expands PSA abbreviations (-HOLO, -REV.FOIL), replaces
	// hyphens, and cleans brackets/embedded numbers.
	// SimplifyForSearch strips trailing noise words and truncates after type suffixes.
	// StripVariantSuffix removes variant descriptors (Holo, Reverse Foil) —
	// PokemonPrice API returns 0 results with these, but finds cards by number matching.
	cleanName := cardutil.NormalizePurchaseName(cardName)
	cleanName = cardutil.SimplifyForSearch(cleanName)
	cleanName = cardutil.StripVariantSuffix(cleanName)
	normalizedSet := cardutil.NormalizeSetNameForSearch(setName)

	// Normalize the target card number for result matching
	collectorNum := cardutil.ExtractCollectorNumber(cardName)
	if collectorNum == "" {
		collectorNum = cardutil.NormalizeCardNumber(cardNumber)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + c.apiKey,
		"Accept":        "application/json",
	}

	// Wait for rate limiter
	if err := c.rateLimiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return nil, 0, nil, err
		}
		return nil, 0, nil, apperrors.ProviderUnavailable(c.Name(), err)
	}

	// Build API request — search by name only, filter results by number.
	// Skip the set param for generic set names — they provide no disambiguation
	// and cause matchByCardNumber to incorrectly accept candidates[0].
	droppedSet := isGenericSet(ctx, normalizedSet)
	endpoint := baseAPIURL + "/cards"
	params := url.Values{}
	params.Set("search", cleanName)
	if normalizedSet != "" && !droppedSet {
		params.Set("set", normalizedSet)
	}
	if includeEbay {
		params.Set("includeEbay", "true")
	}

	fullURL := endpoint + "?" + params.Encode()

	resp, err := c.httpClient.Get(ctx, fullURL, headers, 15*time.Second)
	if err != nil {
		statusCode, respHeaders, httpErr := c.handleHTTPError(ctx, resp, err)
		return nil, statusCode, respHeaders, httpErr
	}

	var apiResp CardsResponse
	if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
		return nil, resp.StatusCode, resp.Headers, apperrors.ProviderInvalidResponse(c.Name(), err)
	}

	// If set filter returned no results, retry without set filter.
	// Only retry when the set name is meaningful — generic sets like "TCG Cards"
	// provide no disambiguation, causing card number collisions across sets
	// (e.g., promo #161 vs Prismatic Evolutions #161/131).
	if len(apiResp.Data) == 0 && normalizedSet != "" && !droppedSet {
		droppedSet = true

		if c.logger != nil {
			c.logger.Debug(ctx, "pokemonprice retrying without set filter",
				observability.String("card", cleanName),
				observability.String("dropped_set", normalizedSet))
		}

		// Wait for rate limiter again
		if err := c.rateLimiter.Wait(ctx); err != nil {
			if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
				return nil, 0, nil, err
			}
			return nil, 0, nil, apperrors.ProviderUnavailable(c.Name(), err)
		}

		retryParams := url.Values{}
		retryParams.Set("search", cleanName)
		if includeEbay {
			retryParams.Set("includeEbay", "true")
		}

		retryURL := endpoint + "?" + retryParams.Encode()
		resp, err = c.httpClient.Get(ctx, retryURL, headers, 15*time.Second)
		if err != nil {
			statusCode, respHeaders, httpErr := c.handleHTTPError(ctx, resp, err)
			return nil, statusCode, respHeaders, httpErr
		}

		if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
			return nil, resp.StatusCode, resp.Headers, apperrors.ProviderInvalidResponse(c.Name(), err)
		}

		if c.logger != nil {
			c.logger.Debug(ctx, "pokemonprice retry results",
				observability.String("card", cleanName),
				observability.Int("result_count", len(apiResp.Data)))
		}
	}

	if len(apiResp.Data) == 0 {
		return nil, resp.StatusCode, resp.Headers, apperrors.ProviderNotFound(c.Name(), cleanName+" ["+setName+"]")
	}

	// If we have a card number, try to find the exact match
	if collectorNum != "" {
		match := matchByCardNumber(ctx, apiResp.Data, collectorNum, normalizedSet, droppedSet)
		if match != nil {
			return match, resp.StatusCode, resp.Headers, nil
		}
		// Log candidate set names when number matched but set overlap rejected
		if c.logger != nil && droppedSet {
			var candidateSets []string
			for _, d := range apiResp.Data {
				if cardutil.NormalizeCardNumber(d.CardNumber) == collectorNum {
					candidateSets = append(candidateSets, d.SetName)
				}
			}
			if len(candidateSets) > 0 {
				c.logger.Debug(ctx, "pokemonprice number match rejected by set overlap",
					observability.String("card", cleanName),
					observability.String("number", collectorNum),
					observability.String("expected_set", normalizedSet),
					observability.String("candidate_sets", strings.Join(candidateSets, ", ")))
			}
		}
		// No result matched the collector number — don't return wrong card
		return nil, resp.StatusCode, resp.Headers, apperrors.ProviderNotFound(c.Name(), cleanName+" #"+collectorNum+" ["+setName+"]")
	}

	// No collector number specified — return first result, but guard against
	// cross-set matches when we dropped the set filter during retry
	if droppedSet {
		return nil, resp.StatusCode, resp.Headers, apperrors.ProviderNotFound(c.Name(), cleanName+" ["+setName+"]")
	}
	return &apiResp.Data[0], resp.StatusCode, resp.Headers, nil
}

// handleHTTPError extracts status code and headers from a potentially-nil response
// and wraps the error as ProviderUnavailable unless it's a context cancellation.
func (c *Client) handleHTTPError(ctx context.Context, resp *httpx.Response, err error) (int, http.Header, error) {
	statusCode := 500
	if resp != nil {
		statusCode = resp.StatusCode
	}
	var respHeaders http.Header
	if resp != nil {
		respHeaders = resp.Headers
	}
	if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return statusCode, respHeaders, err
	}
	return statusCode, respHeaders, apperrors.ProviderUnavailable(c.Name(), err)
}

// matchByCardNumber finds the best result matching the collector number.
// When the set filter was dropped (droppedSet=true), requires the result's set
// name to partially match the original set to avoid cross-set collisions.
func matchByCardNumber(ctx context.Context, data []CardPriceData, collectorNum, originalSet string, droppedSet bool) *CardPriceData {
	var candidates []*CardPriceData
	for i := range data {
		resultNum := cardutil.NormalizeCardNumber(data[i].CardNumber)
		if resultNum == collectorNum {
			candidates = append(candidates, &data[i])
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Single match — safe to return
	if len(candidates) == 1 && !droppedSet {
		return candidates[0]
	}

	// When set filter was dropped, disambiguate by set name.
	// Prefer results whose set name partially matches the original.
	if droppedSet {
		if originalSet == "" {
			// No set info to disambiguate — ambiguous, don't guess
			return nil
		}
		for _, c := range candidates {
			if setOverlaps(ctx, c.SetName, originalSet) {
				return c
			}
		}
		// No set overlap — ambiguous, don't guess
		return nil
	}

	// Multiple matches with set filter active — return first
	return candidates[0]
}

// setOverlaps returns true if the result set name shares significant words
// with the original normalized set name (e.g., "Expedition" matches
// PokemonPrice's "[Expedition]").
// Returns false for empty resultSet to avoid treating missing metadata as a match.
func setOverlaps(ctx context.Context, resultSet, normalizedSet string) bool {
	if resultSet == "" {
		return false
	}
	return cardutil.MatchesSetOverlap(resultSet, normalizedSet)
}

// isGenericSet delegates to the centralized generic set check.
func isGenericSet(ctx context.Context, set string) bool {
	return constants.IsGenericSetName(set)
}
