package intelligence

import "time"

// MarketIntelligence holds DH Tier 3 market data for a card.
type MarketIntelligence struct {
	CardName   string
	SetName    string
	CardNumber string
	DHCardID   string

	Sentiment   *Sentiment
	Forecast    *Forecast
	GradingROI  []GradeROI
	RecentSales []Sale
	Population  []PopulationEntry
	Insights    *Insights
	Velocity    *Velocity
	Trend       *Trend

	FetchedAt time.Time
}

// Velocity captures sell-through momentum across trailing windows. Source:
// DH `/cards/{id}/velocity` or the `velocity` block of `batch_analytics`.
// SampleSize < 5 means the percentages are noise — consumers should
// downweight accordingly.
type Velocity struct {
	SellThrough30dPct float64
	SellThrough60dPct float64
	SellThrough90dPct float64
	SampleSize        int
	LastFetch         time.Time
}

// Trend captures sales-count volume across trailing windows. Source: DH
// `/cards/{id}/trend` or the `trend` block of `batch_analytics`.
type Trend struct {
	Volume7d  int
	Volume30d int
	Volume90d int
}

// Sentiment represents community sentiment for a card.
type Sentiment struct {
	Score        float64 // 0-1
	MentionCount int
	Trend        string // "rising", "falling", "stable"
}

// Forecast represents a price prediction.
type Forecast struct {
	PredictedPriceCents int64
	Confidence          float64
	ForecastDate        time.Time
}

// GradeROI represents return on investment data for a specific grade.
type GradeROI struct {
	Grade        string
	AvgSaleCents int64
	ROI          float64
}

// Sale represents a recent sale transaction.
type Sale struct {
	SoldAt         time.Time
	GradingCompany string
	Grade          string
	PriceCents     int64
	Platform       string
}

// PopulationEntry represents graded population data.
type PopulationEntry struct {
	GradingCompany string
	Grade          string
	Count          int
}

// SeedCandidate identifies a card for which we want to fetch initial market
// intelligence. The DH refresh scheduler uses this to seed rows into
// market_intelligence — the existing GetStale loop only refreshes rows that
// already exist, so without seeding the table stays empty forever.
type SeedCandidate struct {
	DHCardID   string
	CardName   string
	SetName    string
	CardNumber string
}

// Insights holds AI-generated market insights.
type Insights struct {
	Headline string
	Detail   string
}

// Suggestion represents a daily DH buy/sell pick.
//
// Served as-is by GET /api/dh/suggestions and
// GET /api/dh/suggestions/inventory-alerts, so the tags are the API contract.
// StructuredReasoning and Metrics hold pre-encoded JSON and are strings on the
// wire, not objects.
type Suggestion struct {
	SuggestionDate      string    `json:"suggestionDate"`
	Type                string    `json:"type"`     // "cards" or "sealed"
	Category            string    `json:"category"` // "hottest_cards" or "consider_selling"
	Rank                int       `json:"rank"`
	IsManual            bool      `json:"isManual"`
	DHCardID            string    `json:"dhCardId"`
	CardName            string    `json:"cardName"`
	SetName             string    `json:"setName"`
	CardNumber          string    `json:"cardNumber"`
	ImageURL            string    `json:"imageUrl"`
	CurrentPriceCents   int64     `json:"currentPriceCents"`
	ConfidenceScore     float64   `json:"confidenceScore"`
	Reasoning           string    `json:"reasoning"`
	StructuredReasoning string    `json:"structuredReasoning"` // JSON
	Metrics             string    `json:"metrics"`             // JSON
	SentimentScore      float64   `json:"sentimentScore"`
	SentimentTrend      float64   `json:"sentimentTrend"`
	SentimentMentions   int       `json:"sentimentMentions"`
	FetchedAt           time.Time `json:"fetchedAt"`
}
