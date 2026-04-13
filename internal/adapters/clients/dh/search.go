package dh

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// --- Match Types ---

// MatchRequest is the request body for POST /enterprise/match.
type MatchRequest struct {
	Title      string          `json:"title,omitempty"`
	Metafields *MatchMetafield `json:"metafields,omitempty"`
	SKU        string          `json:"sku,omitempty"`
}

// MatchMetafield provides structured card metadata for matching.
type MatchMetafield struct {
	Pokemon string `json:"pokemon"`
	SetName string `json:"set_name"`
	Number  string `json:"number"`
}

// MatchResponse is the response from POST /enterprise/match.
type MatchResponse struct {
	Matched     bool       `json:"matched"`
	DHCardID    *int       `json:"dh_card_id"`
	Confidence  float64    `json:"confidence"`
	MatchMethod *string    `json:"match_method"` // "exact", "fuzzy", "sku"
	Card        *MatchCard `json:"card"`
}

// MatchCard is the card detail returned in a match response.
type MatchCard struct {
	DHCardID int    `json:"dh_card_id"`
	Name     string `json:"name"`
	SetName  string `json:"set_name"`
	Number   string `json:"number"`
	ImageURL string `json:"image_url"`
}

// --- Search Types ---

// SearchFilters contains query parameters for GET /enterprise/search.
type SearchFilters struct {
	Query        string
	Language     string
	Set          string
	Number       string
	Rarity       string
	Era          string
	Holo         string
	ReverseHolo  string
	FirstEdition string
	MinPrice     string
	MaxPrice     string
	Page         int
	PerPage      int
}

// SearchResponse is the response from GET /enterprise/search.
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Meta    SearchMeta     `json:"meta"`
}

// SearchResult is a single card in search results.
type SearchResult struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	SetName            string  `json:"set_name"`
	Number             string  `json:"number"`
	Rarity             string  `json:"rarity"`
	Language           string  `json:"language"`
	Era                string  `json:"era"`
	Artist             *string `json:"artist"`
	ImageURL           *string `json:"image_url"`
	MarketPrice        float64 `json:"market_price"`
	IsHolo             *bool   `json:"is_holo"`
	IsReverseHolo      *bool   `json:"is_reverse_holo"`
	IsFirstEdition     *bool   `json:"is_first_edition"`
	TCGPlayerProductID *int    `json:"tcgplayer_product_id"`
}

// SearchMeta holds pagination and query metadata for search results.
type SearchMeta struct {
	Query            *string `json:"query"`
	TotalHits        int     `json:"total_hits"`
	FilteredHits     int     `json:"filtered_hits"`
	Page             int     `json:"page"`
	PerPage          int     `json:"per_page"`
	TotalPages       int     `json:"total_pages"`
	ProcessingTimeMS *int    `json:"processing_time_ms"`
}

// --- Client Methods ---

// MatchCard attempts to match a title/SKU/metafields to a DH card.
func (c *Client) MatchCard(ctx context.Context, req MatchRequest) (*MatchResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/match", c.baseURL)

	var resp MatchResponse
	if err := c.postEnterprise(ctx, fullURL, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SearchCards performs a full-text search across the DH card catalog.
func (c *Client) SearchCards(ctx context.Context, filters SearchFilters) (*SearchResponse, error) {
	params := url.Values{}
	if filters.Query != "" {
		params.Set("query", filters.Query)
	}
	if filters.Language != "" {
		params.Set("language", filters.Language)
	}
	if filters.Set != "" {
		params.Set("set", filters.Set)
	}
	if filters.Number != "" {
		params.Set("number", filters.Number)
	}
	if filters.Rarity != "" {
		params.Set("rarity", filters.Rarity)
	}
	if filters.Era != "" {
		params.Set("era", filters.Era)
	}
	if filters.Holo != "" {
		params.Set("holo", filters.Holo)
	}
	if filters.ReverseHolo != "" {
		params.Set("reverse_holo", filters.ReverseHolo)
	}
	if filters.FirstEdition != "" {
		params.Set("first_edition", filters.FirstEdition)
	}
	if filters.MinPrice != "" {
		params.Set("min_price", filters.MinPrice)
	}
	if filters.MaxPrice != "" {
		params.Set("max_price", filters.MaxPrice)
	}
	if filters.Page > 0 {
		params.Set("page", strconv.Itoa(filters.Page))
	}
	if filters.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(filters.PerPage))
	}

	fullURL := fmt.Sprintf("%s/api/v1/enterprise/search?%s", c.baseURL, params.Encode())

	var resp SearchResponse
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, err
	}
	for i, item := range resp.Results {
		if item.Name == "" {
			return nil, apperrors.ProviderInvalidResponse(providerName,
				fmt.Errorf("search result[%d] has empty name", i))
		}
	}
	return &resp, nil
}
