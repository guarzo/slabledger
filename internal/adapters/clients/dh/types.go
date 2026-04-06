// Package dh provides a client for the DH pricing and market data API.
package dh

// SearchResponse is the response from GET /catalog/search.
type SearchResponse struct {
	Cards []SearchCard `json:"cards"`
}

// SearchCard is a single card result from the catalog search.
type SearchCard struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	SetName  string `json:"set"`
	Number   string `json:"number"`
	ImageURL string `json:"image_url"`
}

// MatchRequest is the body for POST /catalog/match.
type MatchRequest struct {
	Title      string            `json:"title"`
	SKU        string            `json:"sku,omitempty"`
	Metafields map[string]string `json:"metafields,omitempty"`
}

// MatchResponse is returned from POST /catalog/match.
type MatchResponse struct {
	Success     bool    `json:"success"`
	CardID      int     `json:"card_id"`
	CardTitle   string  `json:"card_title"`
	Confidence  float64 `json:"confidence"`
	MatchMethod string  `json:"match_method"`
}

// MarketDataResponse is returned from GET /market/{card_id}.
type MarketDataResponse struct {
	Tier           int                 `json:"tier"`
	HasData        bool                `json:"has_data"`
	CardID         int                 `json:"card_id"`
	CardTitle      string              `json:"card_title"`
	CurrentPrice   float64             `json:"current_price"`
	PeriodLow      float64             `json:"period_low"`
	PeriodHigh     float64             `json:"period_high"`
	PriceChange    float64             `json:"price_change"`
	PriceChangePct float64             `json:"price_change_pct"`
	PriceHistory   [][]any             `json:"price_history"`
	Periods        map[string]Period   `json:"periods"`
	RecentSales    []RecentSale        `json:"recent_sales"`
	Population     []PopEntry          `json:"population"`
	Insights       *InsightsData       `json:"insights"`
	Sentiment      *SentimentData      `json:"sentiment"`
	GradingROI     *GradingROIResponse `json:"grading_roi"`
	PriceForecast  *ForecastData       `json:"price_forecast"`
}

// Period holds price statistics for a named time window (e.g. "7d", "30d", "90d").
type Period struct {
	CurrentPrice   float64 `json:"current_price"`
	PeriodLow      float64 `json:"period_low"`
	PeriodHigh     float64 `json:"period_high"`
	PriceChange    float64 `json:"price_change"`
	PriceChangePct float64 `json:"price_change_pct"`
	PriceHistory   [][]any `json:"price_history"`
}

// RecentSale is a single completed sale from recent sales history.
type RecentSale struct {
	SoldAt         string  `json:"sold_at"`
	GradingCompany string  `json:"grading_company"`
	Grade          string  `json:"grade"`
	Price          float64 `json:"price"`
	Platform       string  `json:"platform"`
}

// PopEntry is a single population report entry for a grading company and grade.
type PopEntry struct {
	GradingCompany string `json:"grading_company"`
	Grade          string `json:"grade"`
	Count          int    `json:"count"`
}

// InsightsData holds AI-generated headline and detail insights for a card.
type InsightsData struct {
	Headline string `json:"headline"`
	Detail   string `json:"detail"`
}

// SentimentData holds social sentiment metrics for a card.
type SentimentData struct {
	Score        float64 `json:"score"`
	MentionCount int     `json:"mention_count"`
	Trend        string  `json:"trend"`
}

// GradingROIData holds the estimated ROI for a given grade.
type GradingROIData struct {
	Grade        string  `json:"grade"`
	AvgSalePrice float64 `json:"avg_sale_price"`
	ROI          float64 `json:"roi"`
}

// GradingROIResponse wraps the grading ROI data from the API.
type GradingROIResponse struct {
	Card    GradingROICard   `json:"card"`
	ROIData []GradingROIData `json:"roi_data"`
}

// GradingROICard is the card identity within grading ROI data.
type GradingROICard struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	SetName string `json:"set_name"`
}

// ForecastData holds a price forecast with confidence and date.
type ForecastData struct {
	PredictedPrice float64 `json:"predicted_price"`
	Confidence     float64 `json:"confidence"`
	ForecastDate   string  `json:"forecast_date"`
}

// SuggestionsResponse is returned from GET /suggestions.
type SuggestionsResponse struct {
	Cards          SuggestionGroup `json:"cards"`
	Sealed         SuggestionGroup `json:"sealed"`
	GeneratedAt    string          `json:"generated_at"`
	SuggestionDate string          `json:"suggestion_date"`
}

// SuggestionGroup holds hottest cards and selling candidates for a product type.
type SuggestionGroup struct {
	HottestCards    []SuggestionItem `json:"hottest_cards"`
	ConsiderSelling []SuggestionItem `json:"consider_selling"`
}

// SuggestionItem is a single card recommendation with reasoning and metrics.
type SuggestionItem struct {
	Rank                int                  `json:"rank"`
	IsManual            bool                 `json:"is_manual"`
	Card                SuggestionCard       `json:"card"`
	ConfidenceScore     float64              `json:"confidence_score"`
	Reasoning           string               `json:"reasoning"`
	StructuredReasoning *StructuredReasoning `json:"structured_reasoning"`
	Metrics             *SuggestionMetrics   `json:"metrics"`
	Sentiment           *SuggestionSentiment `json:"sentiment"`
}

// SuggestionCard is the card identity embedded in a suggestion item.
type SuggestionCard struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	SetName       string  `json:"set_name"`
	Number        string  `json:"number"`
	ImageURL      string  `json:"image_url"`
	ImageURLSmall string  `json:"image_url_small"`
	ImageURLLarge string  `json:"image_url_large"`
	Slug          string  `json:"slug"`
	CurrentPrice  float64 `json:"current_price"`
}

// StructuredReasoning holds a machine-readable breakdown of a suggestion's rationale.
type StructuredReasoning struct {
	Summary    string   `json:"summary"`
	Verdict    string   `json:"verdict"`
	KeyInsight string   `json:"key_insight"`
	Signals    []Signal `json:"signals"`
}

// Signal is a single contributing factor in a suggestion's structured reasoning.
type Signal struct {
	Factor    string `json:"factor"`
	Direction string `json:"direction"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Metric    string `json:"metric"`
}

// SuggestionMetrics holds quantitative market metrics for a suggestion item.
type SuggestionMetrics struct {
	Sentiment14d  float64            `json:"sentiment_14d"`
	Mentions14d   int                `json:"mentions_14d"`
	PriceChange7d float64            `json:"price_change_7d"`
	CurrentPrice  float64            `json:"current_price"`
	RawNMPrice    float64            `json:"raw_nm_price"`
	PSA10Price    float64            `json:"psa_10_price"`
	Factors       map[string]float64 `json:"factors"`
	BuyScore      float64            `json:"buy_score"`
	SellScore     float64            `json:"sell_score"`
}

// SuggestionSentiment holds the social sentiment snapshot for a suggestion item.
type SuggestionSentiment struct {
	Score        float64 `json:"score"`
	Trend        float64 `json:"trend"`
	MentionCount int     `json:"mention_count"`
}

// CardLookupResponse is returned from GET /enterprise/cards/lookup.
type CardLookupResponse struct {
	Card       CardLookupCard       `json:"card"`
	MarketData CardLookupMarketData `json:"market_data"`
}

// CardLookupCard is the card identity from enterprise lookup.
type CardLookupCard struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	SetName            string `json:"set_name"`
	Number             string `json:"number"`
	Rarity             string `json:"rarity"`
	Language           string `json:"language"`
	Era                string `json:"era"`
	Year               string `json:"year"`
	Artist             string `json:"artist"`
	ImageURL           string `json:"image_url"`
	Slug               string `json:"slug"`
	PriceChartingID    string `json:"pricecharting_id"`
	TCGPlayerProductID *int   `json:"tcgplayer_product_id"`
}

// CardLookupMarketData is the market data from enterprise lookup.
type CardLookupMarketData struct {
	BestBid      *float64 `json:"best_bid"`
	BestAsk      *float64 `json:"best_ask"`
	Spread       *float64 `json:"spread"`
	LastSale     *float64 `json:"last_sale"`
	LastSaleDate *string  `json:"last_sale_date"`
	LowPrice     *float64 `json:"low_price"`
	MidPrice     *float64 `json:"mid_price"`
	HighPrice    *float64 `json:"high_price"`
	ActiveBids   int      `json:"active_bids"`
	ActiveAsks   int      `json:"active_asks"`
	Volume24h    int      `json:"24h_volume"`
	Change24h    *float64 `json:"24h_change"`
	Change7d     *float64 `json:"7d_change"`
	Change30d    *float64 `json:"30d_change"`
}
