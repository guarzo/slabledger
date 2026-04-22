package dh

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// DH-imposed per-call card_id caps. Callers pass arbitrarily-sized slices
// and these constants control how many HTTP calls the wrapper issues.
const (
	batchAnalyticsMaxIDsPerCall = 100
	demandSignalsMaxIDsPerCall  = 50
)

// BatchAnalytics returns per-card analytics (velocity/trend/saturation/
// price_distribution) for up to 100 cards per HTTP call. Longer inputs are
// auto-chunked and the results are concatenated. Per-card "not computed yet"
// surfaces as a populated Error field on the matching result row — the
// method itself only returns an error for transport/auth failures.
func (c *Client) BatchAnalytics(ctx context.Context, cardIDs []int, fields []string) (*BatchAnalyticsResponse, error) {
	if len(cardIDs) == 0 {
		return &BatchAnalyticsResponse{}, nil
	}

	fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/batch_analytics", c.baseURL)
	out := &BatchAnalyticsResponse{Results: make([]CardAnalytics, 0, len(cardIDs))}

	for start := 0; start < len(cardIDs); start += batchAnalyticsMaxIDsPerCall {
		end := start + batchAnalyticsMaxIDsPerCall
		if end > len(cardIDs) {
			end = len(cardIDs)
		}

		body := struct {
			CardIDs []int    `json:"card_ids"`
			Fields  []string `json:"fields"`
		}{
			CardIDs: cardIDs[start:end],
			Fields:  fields,
		}

		var resp BatchAnalyticsResponse
		if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
			return nil, err
		}
		out.Results = append(out.Results, resp.Results...)
	}

	return out, nil
}

// DemandSignals returns per-card demand signals for up to 50 cards per HTTP
// call. Inputs longer than 50 are auto-chunked.
func (c *Client) DemandSignals(ctx context.Context, cardIDs []int, window string) (*DemandSignalsResponse, error) {
	if len(cardIDs) == 0 {
		return &DemandSignalsResponse{}, nil
	}

	out := &DemandSignalsResponse{DemandSignals: make([]DemandSignal, 0, len(cardIDs))}

	for start := 0; start < len(cardIDs); start += demandSignalsMaxIDsPerCall {
		end := start + demandSignalsMaxIDsPerCall
		if end > len(cardIDs) {
			end = len(cardIDs)
		}

		params := url.Values{}
		for _, id := range cardIDs[start:end] {
			params.Add("card_ids[]", strconv.Itoa(id))
		}
		if window != "" {
			params.Set("window", window)
		}

		fullURL := fmt.Sprintf("%s/api/v1/enterprise/market/demand_signals?%s", c.baseURL, params.Encode())

		var resp DemandSignalsResponse
		if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
			return nil, err
		}
		out.DemandSignals = append(out.DemandSignals, resp.DemandSignals...)
	}

	return out, nil
}

// CharacterDemand aggregates demand signals for a seed set of cards grouped
// by Pokemon character. Auto-chunks at 50 card_ids per call; per-chunk
// character_demand lists are concatenated. Callers that want de-duplicated
// rollups across chunks should aggregate on character_name themselves.
func (c *Client) CharacterDemand(ctx context.Context, cardIDs []int, window string, byEra bool) (*CharacterDemandResponse, error) {
	if len(cardIDs) == 0 {
		return &CharacterDemandResponse{}, nil
	}

	out := &CharacterDemandResponse{CharacterDemand: make([]CharacterDemandEntry, 0)}

	for start := 0; start < len(cardIDs); start += demandSignalsMaxIDsPerCall {
		end := start + demandSignalsMaxIDsPerCall
		if end > len(cardIDs) {
			end = len(cardIDs)
		}

		params := url.Values{}
		for _, id := range cardIDs[start:end] {
			params.Add("card_ids[]", strconv.Itoa(id))
		}
		if window != "" {
			params.Set("window", window)
		}
		if byEra {
			params.Set("by_era", "true")
		}

		fullURL := fmt.Sprintf("%s/api/v1/enterprise/market/demand_signals/character_demand?%s", c.baseURL, params.Encode())

		var resp CharacterDemandResponse
		if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
			return nil, err
		}
		out.CharacterDemand = append(out.CharacterDemand, resp.CharacterDemand...)
	}

	return out, nil
}

// TopCharacters is an unseeded market-wide scan of top characters by demand
// score. Single HTTP call.
func (c *Client) TopCharacters(ctx context.Context, limit int, era string) (*TopCharactersResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if era != "" {
		params.Set("era", era)
	}

	fullURL := fmt.Sprintf("%s/api/v1/enterprise/market/demand_signals/top_characters", c.baseURL)
	if encoded := params.Encode(); encoded != "" {
		fullURL += "?" + encoded
	}

	var resp TopCharactersResponse
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, translateAnalyticsErr(err)
	}
	return &resp, nil
}

// CharacterVelocity returns characters ranked by velocity metrics. Paginated;
// one HTTP call per invocation — callers iterate pages themselves.
func (c *Client) CharacterVelocity(ctx context.Context, opts CharacterListOpts) (*CharacterVelocityResponse, error) {
	fullURL := c.baseURL + "/api/v1/enterprise/characters/velocity" + encodeCharacterListQuery(opts)

	var resp CharacterVelocityResponse
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, translateAnalyticsErr(err)
	}
	return &resp, nil
}

// CharacterSaturation returns characters ranked by supply saturation.
// Paginated; one HTTP call per invocation.
func (c *Client) CharacterSaturation(ctx context.Context, opts CharacterListOpts) (*CharacterSaturationResponse, error) {
	fullURL := c.baseURL + "/api/v1/enterprise/characters/saturation" + encodeCharacterListQuery(opts)

	var resp CharacterSaturationResponse
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, translateAnalyticsErr(err)
	}
	return &resp, nil
}

// CharacterLeaderboard returns a ranked leaderboard of characters by the
// chosen metric, with optional era and grade filters.
func (c *Client) CharacterLeaderboard(ctx context.Context, metric, era string, grade int, limit int) (*LeaderboardResponse, error) {
	params := url.Values{}
	if metric != "" {
		params.Set("metric", metric)
	}
	if era != "" {
		params.Set("era", era)
	}
	if grade > 0 {
		params.Set("grade", strconv.Itoa(grade))
	}
	if limit > 0 {
		params.Set("per_page", strconv.Itoa(limit))
	}

	fullURL := c.baseURL + "/api/v1/enterprise/characters/leaderboard"
	if encoded := params.Encode(); encoded != "" {
		fullURL += "?" + encoded
	}

	var resp LeaderboardResponse
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, translateAnalyticsErr(err)
	}
	return &resp, nil
}

// encodeCharacterListQuery builds the querystring shared by
// /characters/velocity and /characters/saturation.
func encodeCharacterListQuery(opts CharacterListOpts) string {
	params := url.Values{}
	if opts.SortBy != "" {
		params.Set("sort_by", opts.SortBy)
	}
	if opts.SortDir != "" {
		params.Set("sort_dir", opts.SortDir)
	}
	if opts.Page > 0 {
		params.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	if encoded := params.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

// GradedSalesAnalytics returns graded-only sales analytics for a card with
// the given grading company + grade filter. DH requires both filters —
// calling this without them returns a 400 with "Invalid grading company or
// grade combination". The returned envelope carries period_stats (7d/30d/90d
// summary buckets), recent_sales (up to 10), a 90d price distribution
// histogram, and a by_company cross-company breakdown.
func (c *Client) GradedSalesAnalytics(ctx context.Context, cardID int, gradingCompany, grade string) (*GradedSalesAnalyticsResponse, error) {
	params := url.Values{}
	params.Set("grading_company", gradingCompany)
	params.Set("grade", grade)
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/%d/graded-sales-analytics?%s", c.baseURL, cardID, params.Encode())

	var resp GradedSalesAnalyticsResponse
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, translateAnalyticsErr(err)
	}
	return &resp, nil
}

// translateAnalyticsErr maps a DH 404 with body `{"error":"analytics_not_computed"}`
// to the typed sentinel ErrAnalyticsNotComputed. All other errors pass through
// unchanged. httpx's 404 handler includes the (sanitized) response body in the
// returned error message, which is where the marker lives.
func translateAnalyticsErr(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "analytics_not_computed") {
		return ErrAnalyticsNotComputed
	}
	return err
}
