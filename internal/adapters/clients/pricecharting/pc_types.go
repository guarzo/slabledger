package pricecharting

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// ErrMissingRequiredField creates an error for missing required API fields
func ErrMissingRequiredField(field string) error {
	return fmt.Errorf("missing required field: %s", field)
}

// FlexibleInt is a custom type that can unmarshal from both string and int
type FlexibleInt struct {
	Value int
	IsSet bool
}

// UnmarshalJSON implements json.Unmarshaler to handle both string and int
func (fi *FlexibleInt) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as int first
	var intVal int
	if err := json.Unmarshal(data, &intVal); err == nil {
		fi.Value = intVal
		fi.IsSet = true
		return nil
	}

	// Try to unmarshal as string
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		if strVal == "" {
			fi.IsSet = false
			return nil
		}
		// Parse string to int
		parsed, err := strconv.Atoi(strVal)
		if err != nil {
			// If parsing fails, treat as 0
			fi.Value = 0
			fi.IsSet = true
			return nil
		}
		fi.Value = parsed
		fi.IsSet = true
		return nil
	}

	// If both fail, it might be null
	fi.IsSet = false
	return nil
}

// ToIntPtr converts FlexibleInt to *int for compatibility
func (fi *FlexibleInt) ToIntPtr() *int {
	if !fi.IsSet {
		return nil
	}
	val := fi.Value
	return &val
}

// PriceChartingAPIResponse represents the exact structure returned by PriceCharting API
// This provides type safety and eliminates the need for map[string]any type assertions
type PriceChartingAPIResponse struct {
	ID          string `json:"id"`
	ProductName string `json:"product-name"`
	ConsoleName string `json:"console-name"`
	Status      string `json:"status"`

	// Core prices (API returns integers in CENTS - no conversion needed!)
	// Field mapping based on PriceCharting Condition Table:
	// Condition 1 (loose) = Ungraded, Condition 5 (graded) = Grade 9,
	// Condition 6 (box-only) = Grade 9.5, Condition 7 (manual-only) = Grade 10
	LoosePrice   *int `json:"loose-price"`       // Ungraded/Raw price in cents (Condition 1)
	GradedPrice  *int `json:"graded-price"`      // Grade 9 price in cents (Condition 5)
	PSA8Price    *int `json:"psa-8-price"`       // PSA 8 price in cents
	BoxOnlyPrice *int `json:"box-only-price"`    // Grade 9.5 price in cents (Condition 6)
	ManualPrice  *int `json:"manual-only-price"` // Grade 10 price in cents (Condition 7)
	BGS10Price   *int `json:"bgs-10-price"`      // BGS 10 price in cents (Condition 8)

	// Additional price fields (API returns integers in CENTS)
	NewPrice     *int `json:"new-price"`    // Sealed product in cents
	CIBPrice     *int `json:"cib-price"`    // Complete In Box in cents
	ManualPrice2 *int `json:"manual-price"` // Separate manual field in cents
	BoxPrice     *int `json:"box-price"`    // Box only in cents

	// UPC (can be null)
	UPC *string `json:"upc"`

	// Sales data fields
	SalesVolume  *FlexibleInt `json:"sales-volume"`
	LastSoldDate *string      `json:"last-sold-date"`

	// Optional marketplace fields
	ActiveListings      *int     `json:"active-listings,omitempty"`
	LowestListing       *int     `json:"lowest-listing,omitempty"`        // Price in cents
	AverageListingPrice *int     `json:"average-listing-price,omitempty"` // Price in cents
	ListingVelocity     *float64 `json:"listing-velocity,omitempty"`      // Rate (not a price)
	CompetitionLevel    *string  `json:"competition-level,omitempty"`

	// Sales data array (if provided)
	SalesData []APISaleData `json:"sales-data,omitempty"`

	// Historical price data
	PriceHistory []APIPricePoint `json:"price-history,omitempty"`
}

// APISaleData represents the exact structure of a sale data point from the API
// This replaces map[string]interface{} with a typed struct for better type safety
type APISaleData struct {
	SalePrice *float64 `json:"sale-price"` // Price in dollars
	SaleDate  *string  `json:"sale-date"`  // Date of sale
	Grade     *string  `json:"grade"`      // Grading (e.g., "PSA 10")
	Source    *string  `json:"source"`     // Source (e.g., "eBay", "PWCC")
}

// APIPricePoint represents the exact structure of a historical price point from the API
// This replaces map[string]interface{} with a typed struct for better type safety
type APIPricePoint struct {
	Date        *string  `json:"date"`         // Date in YYYY-MM-DD format
	PSA10Price  *float64 `json:"psa10-price"`  // PSA 10 price in dollars
	Grade9Price *float64 `json:"grade9-price"` // Grade 9 price in dollars
	RawPrice    *float64 `json:"raw-price"`    // Raw/ungraded price in dollars
	Volume      *int     `json:"volume"`       // Sales volume for this date
	Timestamp   *int64   `json:"timestamp"`    // Unix timestamp
}

// Validate checks if the API response has required fields
func (r *PriceChartingAPIResponse) Validate() error {
	if r.ID == "" {
		return ErrMissingRequiredField("id")
	}
	if r.ProductName == "" {
		return ErrMissingRequiredField("product-name")
	}
	// Note: Prices can be nil or 0, so we don't validate them
	return nil
}

// Result normalized to cents (integers) to avoid float issues.
type PCMatch struct {
	ID           string
	ProductName  string
	ConsoleName  string // "console-name" (PriceCharting category)
	LooseCents   int    // "loose-price" (ungraded)
	PSA8Cents    int    // "psa-8-price" (PSA 8)
	Grade9Cents  int    // "graded-price" (Grade 9)
	Grade95Cents int    // "box-only-price" (Grade 9.5)
	PSA10Cents   int    // "manual-only-price" (PSA 10)
	BGS10Cents   int    // "bgs-10-price" (BGS 10)

	// Additional price fields
	NewPriceCents    int // "new-price" (Sealed product price)
	CIBPriceCents    int // "cib-price" (Complete In Box)
	ManualPriceCents int // "manual-price" (Manual only - separate field)
	BoxPriceCents    int // "box-price" (Box only - separate field)

	// Sales data extracted from API (if available)
	RecentSales  []SaleData // Recent eBay sales tracked by PriceCharting
	SalesVolume  int        // "sales-volume" - Number of recent sales
	SalesCount   int        // Total number of sales (calculated)
	LastSoldDate string     // "last-sold-date" - Date of last sale
	AvgSalePrice int        // Average sale price in cents (calculated)

	// Sales velocity for liquidity scoring
	Sales30d int // Sales in last 30 days (calculated from RecentSales)
	Sales90d int // Sales in last 90 days (calculated from RecentSales)

	// Conservative exit prices (p25 percentiles) for margin of safety calculation
	ConservativePSA10USD float64 // p25 percentile of recent PSA 10 sales
	ConservativePSA9USD  float64 // p25 percentile of recent PSA 9 sales
	OptimisticRawUSD     float64 // p90 percentile for raw/ungraded (NM estimate)

	// Sales distribution data (Conservative Exit Prices)
	// Full percentile distributions for risk-adjusted scoring
	PSA10Distribution *pricing.SalesDistribution // Distribution of PSA 10 sales
	PSA9Distribution  *pricing.SalesDistribution // Distribution of PSA 9 sales
	RawDistribution   *pricing.SalesDistribution // Distribution of raw sales

	// Marketplace fields
	ActiveListings      int     // Current marketplace listings
	LowestListing       int     // Lowest available price
	AverageListingPrice int     // Average of all listings
	ListingVelocity     float64 // Sales per day
	CompetitionLevel    string  // LOW, MEDIUM, HIGH
	PriceVolatility     float64 // coefficient of variation

	// UPC and search fields
	UPC       string // Universal Product Code
	QueryUsed string // The actual query that produced this match

	// Last sold data by grade (calculated from RecentSales)
	LastSoldByGrade *pricing.LastSoldByGrade
}

// SaleData represents a single sale tracked by PriceCharting
type SaleData struct {
	PriceCents int
	Date       string
	Grade      string
	Source     string // "eBay", "PWCC", etc.
}

// QueryDeduplicator prevents duplicate queries within a batch
type QueryDeduplicator struct {
	cache map[string]*PCMatch
	mu    sync.RWMutex
}

// NewQueryDeduplicator creates a new query deduplicator
func NewQueryDeduplicator() *QueryDeduplicator {
	return &QueryDeduplicator{
		cache: make(map[string]*PCMatch),
	}
}

// GetCached returns a cached result if available
func (qd *QueryDeduplicator) GetCached(query string) *PCMatch {
	qd.mu.RLock()
	defer qd.mu.RUnlock()
	return qd.cache[query]
}

// Store caches a query result
func (qd *QueryDeduplicator) Store(query string, match *PCMatch) {
	qd.mu.Lock()
	defer qd.mu.Unlock()
	qd.cache[query] = match
}

// Clear clears the deduplicator cache
func (qd *QueryDeduplicator) Clear() {
	qd.mu.Lock()
	defer qd.mu.Unlock()
	qd.cache = make(map[string]*PCMatch)
}

// PriceChartingStats represents API usage statistics
// Replaces map[string]interface{} with typed struct for type safety
type PriceChartingStats struct {
	APIRequests    int64               `json:"api_requests"`
	CachedRequests int64               `json:"cached_requests"`
	TotalRequests  int64               `json:"total_requests"`
	CacheHitRate   string              `json:"cache_hit_rate"` // Formatted as "XX.XX%"
	Reduction      string              `json:"reduction"`      // Formatted as "XX.XX%"
	CircuitBreaker *CircuitBreakerData `json:"circuit_breaker,omitempty"`
}

// CircuitBreakerData represents circuit breaker statistics
type CircuitBreakerData struct {
	State                string `json:"state"`
	Requests             uint32 `json:"requests"`
	Successes            uint32 `json:"successes"`
	Failures             uint32 `json:"failures"`
	ConsecutiveSuccesses uint32 `json:"consecutive_successes"`
	ConsecutiveFailures  uint32 `json:"consecutive_failures"`
}
