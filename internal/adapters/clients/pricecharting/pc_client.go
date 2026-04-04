package pricecharting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// hasPriceKeysTyped checks if the typed API response contains graded price data.
// Requires at least one graded price field — a response with only LoosePrice (raw)
// is not useful for grading analysis and should fall through to fallback or no-match.
func hasPriceKeysTyped(resp *PriceChartingAPIResponse) bool {
	return resp.ManualPrice != nil || resp.GradedPrice != nil ||
		resp.PSA8Price != nil || resp.BoxOnlyPrice != nil || resp.BGS10Price != nil
}

func (p *PriceCharting) lookupByQueryInternal(ctx context.Context, q string) (*PCMatch, error) {
	// First try /api/product?q=... (best match) with improved query
	optimizedQuery := p.queryHelper.OptimizeQueryForDirectLookup(q)
	u := fmt.Sprintf("https://www.pricecharting.com/api/product?t=%s&q=%s", url.QueryEscape(p.token), url.QueryEscape(optimizedQuery))
	var apiResp PriceChartingAPIResponse
	err := p.httpClient.GetJSON(ctx, u, nil, 0, &apiResp)
	if err == nil && strings.EqualFold(apiResp.Status, "success") && hasPriceKeysTyped(&apiResp) {
		jsonBytes, marshalErr := json.Marshal(apiResp)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal API response: %w", marshalErr)
		}
		match, parseErr := parseAPIResponseWithLogger(jsonBytes, p.logger, ctx)
		if parseErr != nil {
			return nil, fmt.Errorf("parse API response: %w", parseErr)
		}
		return match, nil
	}

	// Propagate context cancellations, timeouts, HTTP errors, and transport-level errors immediately
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if strings.Contains(err.Error(), "HTTP") {
			return nil, err
		}
		var netErr net.Error
		if errors.As(err, &netErr) {
			return nil, err
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, err
		}
	}

	// For test environments, don't use fallback to reduce API call count
	if p.isTestMode() {
		return nil, fmt.Errorf("no product match in test mode")
	}

	// Only use fallback if direct lookup fails and we have a reasonable query
	if len(optimizedQuery) < 10 { // Avoid fallback for very short queries
		return nil, fmt.Errorf("no product match - query too short")
	}

	// Fallback: /api/products to list and then pick the first
	u = fmt.Sprintf("https://www.pricecharting.com/api/products?t=%s&q=%s", url.QueryEscape(p.token), url.QueryEscape(optimizedQuery))
	var many struct {
		Status   string `json:"status"`
		Products []struct {
			ID          string `json:"id"`
			ProductName string `json:"product-name"`
		} `json:"products"`
	}
	if err := p.httpClient.GetJSON(ctx, u, nil, 0, &many); err != nil {
		return nil, err
	}
	if !strings.EqualFold(many.Status, "success") || len(many.Products) == 0 {
		return nil, fmt.Errorf("no product match")
	}
	// Pull full product by id
	id := many.Products[0].ID
	u = fmt.Sprintf("https://www.pricecharting.com/api/product?t=%s&id=%s", url.QueryEscape(p.token), url.QueryEscape(id))
	var fullResp PriceChartingAPIResponse
	if err := p.httpClient.GetJSON(ctx, u, nil, 0, &fullResp); err != nil {
		return nil, err
	}
	if !strings.EqualFold(fullResp.Status, "success") {
		return nil, fmt.Errorf("product fetch failed")
	}
	jsonBytes, marshalErr := json.Marshal(fullResp)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal API response: %w", marshalErr)
	}
	match, parseErr := parseAPIResponseWithLogger(jsonBytes, p.logger, ctx)
	if parseErr != nil {
		return nil, fmt.Errorf("parse API response: %w", parseErr)
	}
	return match, nil
}

// LookupByUPC performs a lookup using Universal Product Code
func (p *PriceCharting) LookupByUPC(ctx context.Context, upc string) (*PCMatch, error) {
	// Check cache first
	if match, found := p.cacheManager.GetCachedUPCMatch(ctx, upc); found {
		p.incrementCachedRequests()
		return match, nil
	}

	// Rate limiting
	if p.rateLimiter != nil {
		if err := p.rateLimiter.WaitContext(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait cancelled: %w", err)
		}
	}

	// API call with UPC
	u := fmt.Sprintf("https://www.pricecharting.com/api/product?t=%s&upc=%s",
		url.QueryEscape(p.token), url.QueryEscape(upc))

	var apiResp PriceChartingAPIResponse
	if err := p.httpClient.GetJSON(ctx, u, nil, 0, &apiResp); err != nil {
		return nil, fmt.Errorf("UPC lookup failed: %w", err)
	}

	if !strings.EqualFold(apiResp.Status, "success") {
		return nil, fmt.Errorf("UPC not found")
	}

	jsonBytes, marshalErr := json.Marshal(apiResp)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal API response: %w", marshalErr)
	}
	match, parseErr := parseAPIResponseWithLogger(jsonBytes, p.logger, ctx)
	if parseErr != nil {
		return nil, fmt.Errorf("parse API response: %w", parseErr)
	}
	match.UPC = upc

	// Store UPC mapping if we have the database
	if p.upcDatabase != nil && match.ProductName != "" {
		mapping := &UPCMapping{
			UPC:         upc,
			ProductID:   match.ID,
			ProductName: match.ProductName,
			Confidence:  1.0,
		}
		p.upcDatabase.Add(mapping)
	}

	// Cache the result
	p.cacheManager.CacheUPCMatch(ctx, upc, match)

	p.incrementRequestCount()
	return match, nil
}

// LookupByProductID fetches a product by its PriceCharting product ID.
func (p *PriceCharting) LookupByProductID(ctx context.Context, productID string) (*PCMatch, error) {
	// Rate limiting
	if p.rateLimiter != nil {
		if err := p.rateLimiter.WaitContext(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait cancelled: %w", err)
		}
	}

	u := fmt.Sprintf("https://www.pricecharting.com/api/product?t=%s&id=%s",
		url.QueryEscape(p.token), url.QueryEscape(productID))
	var fullResp PriceChartingAPIResponse
	if err := p.httpClient.GetJSON(ctx, u, nil, 0, &fullResp); err != nil {
		return nil, err
	}
	if !strings.EqualFold(fullResp.Status, "success") {
		return nil, fmt.Errorf("product fetch failed for id %s", productID)
	}
	jsonBytes, marshalErr := json.Marshal(fullResp)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal API response: %w", marshalErr)
	}
	match, parseErr := parseAPIResponseWithLogger(jsonBytes, p.logger, ctx)
	if parseErr != nil {
		return nil, fmt.Errorf("parse API response: %w", parseErr)
	}
	p.incrementRequestCount()
	return match, nil
}

// OptimizeQuery is a wrapper for queryHelper.OptimizeQuery (for testing)
func (p *PriceCharting) OptimizeQuery(setName, cardName, number string) string {
	return p.queryHelper.OptimizeQuery(setName, cardName, number)
}

// isTestMode returns true when the client is configured with a test token,
// centralising the token list so maintenance happens in one place.
func (p *PriceCharting) isTestMode() bool {
	return p.token == "test" || p.token == "test-token"
}

func (p *PriceCharting) incrementRequestCount() {
	p.mu.Lock()
	p.requestCount++
	p.mu.Unlock()
}

func (p *PriceCharting) incrementCachedRequests() {
	p.mu.Lock()
	p.cachedRequests++
	p.mu.Unlock()
}
