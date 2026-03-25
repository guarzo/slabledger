// Package cardhedger provides a client for the CardHedger pricing API.
//
// Price representation: raw API response types (CardPrice, GradePrice, PriceUpdate) carry
// Price as a string because the API returns string-encoded values (e.g. "850", "16999.99").
// Computed/estimated types (PriceEstimateResponse, BatchPriceEstimateResult) use float64
// because they represent calculated estimates with decimal precision. Callers converting
// between the two should use strconv.ParseFloat.
package cardhedger

// --- Card Search ---

// CardSearchRequest is the body for POST /v1/cards/card-search.
type CardSearchRequest struct {
	Search   string `json:"search,omitempty"`
	Set      string `json:"set,omitempty"`
	Category string `json:"category,omitempty"`
	Player   string `json:"player,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// CardSearchResponse is returned from POST /v1/cards/card-search.
type CardSearchResponse struct {
	Pages int              `json:"pages"`
	Count int              `json:"count"`
	Cards []CardSearchCard `json:"cards"`
}

// CardSearchCard represents a card in search results.
type CardSearchCard struct {
	CardID        string      `json:"card_id"`
	Description   string      `json:"description"`
	Player        string      `json:"player"`
	Set           string      `json:"set"`
	Number        string      `json:"number"`
	Variant       string      `json:"variant"`
	Image         string      `json:"image"`
	Category      string      `json:"category"`
	CategoryGroup string      `json:"category_group"`
	SetType       string      `json:"set_type"`
	Sales7Day     int         `json:"7 Day Sales"`
	Sales30Day    int         `json:"30 Day Sales"`
	Rookie        bool        `json:"rookie"`
	Gain          float64     `json:"gain"`
	Prices        []CardPrice `json:"prices"`
}

// CardPrice is a grade+price pair from search results.
type CardPrice struct {
	Grade string `json:"grade"`
	Price string `json:"price"` // String in the API (e.g. "850")
}

// --- All Prices By Card ---

// AllPricesByCardRequest is the body for POST /v1/cards/all-prices-by-card.
type AllPricesByCardRequest struct {
	CardID string `json:"card_id"`
}

// AllPricesByCardResponse is returned from POST /v1/cards/all-prices-by-card.
type AllPricesByCardResponse struct {
	Prices []GradePrice `json:"prices"`
}

// GradePrice is a per-grade price entry.
type GradePrice struct {
	CardID       string `json:"card_id"`
	Grade        string `json:"grade"`  // e.g. "PSA 10", "Raw"
	Grader       string `json:"grader"` // e.g. "PSA", "BGS", "Raw"
	Price        string `json:"price"`  // String (e.g. "16999.99")
	DisplayOrder string `json:"display_order"`
}

// --- Price Estimate ---

// PriceEstimateRequest is the body for POST /v1/cards/price-estimate.
type PriceEstimateRequest struct {
	CardID string `json:"card_id"`
	Grade  string `json:"grade"` // e.g. "PSA 10", "PSA 9"
}

// PriceEstimateResponse is returned from POST /v1/cards/price-estimate.
type PriceEstimateResponse struct {
	Price         float64 `json:"price"`
	PriceLow      float64 `json:"price_low"`
	PriceHigh     float64 `json:"price_high"`
	Confidence    float64 `json:"confidence"`     // 0-1
	Method        string  `json:"method"`         // "direct", "correlated", "segment_fallback"
	FreshnessDays *int    `json:"freshness_days"` // nil for fallback methods
	SupportGrades int     `json:"support_grades"`
	GradeLabel    string  `json:"grade_label"`
	Provider      string  `json:"provider"` // "PSA", "BGS", etc.
	GradeValue    float64 `json:"grade_value"`
}

// --- Batch Price Estimate ---

// BatchPriceEstimateRequest is the body for POST /v1/cards/batch-price-estimate.
type BatchPriceEstimateRequest struct {
	Items []PriceEstimateItem `json:"items"`
}

// PriceEstimateItem is a single card+grade pair for batch estimation.
type PriceEstimateItem struct {
	CardID string `json:"card_id"`
	Grade  string `json:"grade"`
}

// BatchPriceEstimateResponse is returned from POST /v1/cards/batch-price-estimate.
type BatchPriceEstimateResponse struct {
	Results         []BatchPriceEstimateResult `json:"results"`
	TotalRequested  int                        `json:"total_requested"`
	TotalSuccessful int                        `json:"total_successful"`
}

// BatchPriceEstimateResult is a single result from batch estimation.
type BatchPriceEstimateResult struct {
	CardID        string   `json:"card_id"`
	Grade         string   `json:"grade"`
	Price         *float64 `json:"price"`
	PriceLow      *float64 `json:"price_low"`
	PriceHigh     *float64 `json:"price_high"`
	Confidence    *float64 `json:"confidence"`
	Method        *string  `json:"method"`
	FreshnessDays *int     `json:"freshness_days"`
	SupportGrades *int     `json:"support_grades"`
	GradeLabel    *string  `json:"grade_label"`
	Provider      *string  `json:"provider"`
	GradeValue    *float64 `json:"grade_value"`
	Error         *string  `json:"error"`
}

// --- Card Match (AI-powered) ---

// CardMatchRequest is the body for POST /v1/cards/card-match.
type CardMatchRequest struct {
	Query         string `json:"query"`
	Category      string `json:"category,omitempty"`
	MaxCandidates int    `json:"max_candidates,omitempty"`
}

// CardMatchResponse is returned from POST /v1/cards/card-match.
type CardMatchResponse struct {
	Match               *CardMatchResult `json:"match"`
	CandidatesEvaluated int              `json:"candidates_evaluated"`
	SearchQueryUsed     string           `json:"search_query_used"`
}

// CardMatchResult is the matched card from card-match.
type CardMatchResult struct {
	CardID      string      `json:"card_id"`
	Set         string      `json:"set"`
	Number      string      `json:"number"`
	Player      string      `json:"player"`
	Variant     string      `json:"variant"`
	Confidence  float64     `json:"confidence"`
	Reasoning   string      `json:"reasoning"`
	Prices      []CardPrice `json:"prices"`
	Description string      `json:"description"`
	Image       string      `json:"image"`
	Category    string      `json:"category"`
}

// --- Details By Certs (batch cert lookup) ---

// DetailsByCertsRequest is the body for POST /v1/cards/details-by-certs.
type DetailsByCertsRequest struct {
	Certs  []string `json:"certs"`
	Grader string   `json:"grader,omitempty"`
}

// DetailsByCertsResponse is returned from POST /v1/cards/details-by-certs.
type DetailsByCertsResponse struct {
	Results        []CertDetailResult `json:"results"`
	TotalRequested int                `json:"total_requested"`
	TotalFound     int                `json:"total_found"`
}

// CertDetailResult is a single cert lookup result with nested cert_info + card objects.
type CertDetailResult struct {
	CertInfo CertInfo    `json:"cert_info"`
	Card     *CardDetail `json:"card"` // nil when card not in CardHedger DB
}

// CertInfo contains the cert-level metadata from details-by-certs.
type CertInfo struct {
	Cert        string `json:"cert"`
	Grade       string `json:"grade"`
	Description string `json:"description"`
}

// CardDetail contains the matched card identity from details-by-certs.
type CardDetail struct {
	CardID      string `json:"card_id"`
	Description string `json:"description"`
	Player      string `json:"player"`
	Set         string `json:"set"`
	Number      string `json:"number"`
	Variant     string `json:"variant"`
	Category    string `json:"category"`
	Image       string `json:"image"`
}

// --- Card Request ---

// CardRequestBody is the body for POST /v1/cards/card-request.
type CardRequestBody struct {
	Player     string `json:"player"`
	Set        string `json:"set"`
	CardNumber string `json:"card_number"`
	Subset     string `json:"subset,omitempty"`
	ImageURL   string `json:"image_url,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	Token      string `json:"token,omitempty"`
	Variant    string `json:"variant,omitempty"`
}

// CardRequestResponse is returned from POST /v1/cards/card-request.
type CardRequestResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	ID      string `json:"id"`
}

// --- Price Updates (Delta Poll) ---

// PriceUpdatesRequest is the body for POST /v1/cards/price-updates.
type PriceUpdatesRequest struct {
	Since        string   `json:"since"` // ISO timestamp
	IgnoreGrades []string `json:"ignore_grades,omitempty"`
}

// PriceUpdatesResponse is returned from POST /v1/cards/price-updates.
type PriceUpdatesResponse struct {
	Updates []PriceUpdate `json:"updates"`
	Count   int           `json:"count"`
}

// PriceUpdate is a single price change from delta polling.
type PriceUpdate struct {
	Price           string `json:"price"` // String in the API
	SaleDate        string `json:"sale_date"`
	Grade           string `json:"grade"`
	CardDesc        string `json:"card_desc"`
	CardSet         string `json:"card_set"`
	CardNumber      string `json:"card_number"`
	Player          string `json:"player"`
	Variant         string `json:"variant"`
	CardID          string `json:"card_id"`
	UpdateTimestamp string `json:"update_timestamp"`
}
