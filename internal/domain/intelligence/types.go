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

	FetchedAt time.Time
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

// Insights holds AI-generated market insights.
type Insights struct {
	Headline string
	Detail   string
}

// Suggestion represents a daily DH buy/sell pick.
type Suggestion struct {
	SuggestionDate      string
	Type                string // "cards" or "sealed"
	Category            string // "hottest_cards" or "consider_selling"
	Rank                int
	IsManual            bool
	DHCardID            string
	CardName            string
	SetName             string
	CardNumber          string
	ImageURL            string
	CurrentPriceCents   int64
	ConfidenceScore     float64
	Reasoning           string
	StructuredReasoning string // JSON
	Metrics             string // JSON
	SentimentScore      float64
	SentimentTrend      float64
	SentimentMentions   int
	FetchedAt           time.Time
}
