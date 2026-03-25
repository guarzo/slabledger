package pokemonprice

import "time"

// CardsResponse represents the API response for card queries.
// The API returns {"data": [...], "metadata": {...}}.
type CardsResponse struct {
	Data     []CardPriceData `json:"data"`
	Metadata Metadata        `json:"metadata"`
}

// Metadata contains pagination and API usage info from the response.
type Metadata struct {
	Total            int  `json:"total"`
	Count            int  `json:"count"`
	Limit            int  `json:"limit"`
	Offset           int  `json:"offset"`
	HasMore          bool `json:"hasMore"`
	APICallsConsumed struct {
		Total     int `json:"total"`
		Breakdown struct {
			Cards   int `json:"cards"`
			History int `json:"history"`
			Ebay    int `json:"ebay"`
		} `json:"breakdown"`
		CostPerCard int `json:"costPerCard"`
	} `json:"apiCallsConsumed"`
}

// CardPriceData represents pricing data for a Pokemon card.
type CardPriceData struct {
	ID          string    `json:"id"`
	TCGPlayerID string    `json:"tcgPlayerId"`
	SetID       int       `json:"setId"`
	SetName     string    `json:"setName"`
	Name        string    `json:"name"`
	CardNumber  string    `json:"cardNumber"`
	Rarity      string    `json:"rarity"`
	Prices      PriceInfo `json:"prices"`
	ImageURL    string    `json:"imageUrl"`
	Ebay        *EbayData `json:"ebay,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// PriceInfo contains TCGPlayer market pricing data (raw/ungraded card).
type PriceInfo struct {
	Market          float64   `json:"market"`
	Low             float64   `json:"low"`
	Sellers         int       `json:"sellers"`
	Listings        int       `json:"listings"`
	PrimaryPrinting string    `json:"primaryPrinting"`
	LastUpdated     time.Time `json:"lastUpdated"`
}

// EbayData contains eBay sold data returned when includeEbay=true.
type EbayData struct {
	UpdatedAt      time.Time                 `json:"updatedAt"`
	SalesByGrade   map[string]GradeSalesData `json:"salesByGrade"`
	SalesVelocity  SalesVelocity             `json:"salesVelocity"`
	TotalSales     int                       `json:"totalSales"`
	TotalValue     float64                   `json:"totalValue"`
	GradesTracked  []string                  `json:"gradesTracked"`
	DateRangeStart string                    `json:"dateRangeStart"`
	DateRangeEnd   string                    `json:"dateRangeEnd"`
}

// GradeSalesData contains sales statistics for a single grade.
type GradeSalesData struct {
	Count                 int              `json:"count"`
	TotalValue            float64          `json:"totalValue"`
	AveragePrice          float64          `json:"averagePrice"`
	MedianPrice           float64          `json:"medianPrice"`
	MinPrice              float64          `json:"minPrice"`
	MaxPrice              float64          `json:"maxPrice"`
	MarketPrice7Day       *float64         `json:"marketPrice7Day"`
	MarketPriceMedian7Day *float64         `json:"marketPriceMedian7Day"`
	DailyVolume7Day       *float64         `json:"dailyVolume7Day"`
	MarketTrend           *string          `json:"marketTrend"`
	SmartMarketPrice      SmartMarketPrice `json:"smartMarketPrice"`
}

// SmartMarketPrice contains the weighted market price with confidence.
type SmartMarketPrice struct {
	Price      float64 `json:"price"`
	Confidence string  `json:"confidence"` // "high", "medium", "low"
	Method     string  `json:"method"`
	DaysUsed   int     `json:"daysUsed"`
}

// SalesVelocity contains sales velocity metrics.
type SalesVelocity struct {
	DailyAverage  float64 `json:"dailyAverage"`
	WeeklyAverage float64 `json:"weeklyAverage"`
	MonthlyTotal  int     `json:"monthlyTotal"`
}
