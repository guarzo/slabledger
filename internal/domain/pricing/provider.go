package pricing

import (
	"context"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
)

// PriceProvider defines what the domain needs from price sources.
// Implementations live in their own packages (prices/, ebay/, gamestop/)
type PriceProvider interface {
	// GetPrice fetches price data for a card
	GetPrice(ctx context.Context, card Card) (*Price, error)

	// Available returns true if provider is configured and ready
	Available() bool

	// Name returns provider identifier for logging/metrics
	Name() string

	// Close releases resources (for cleanup)
	Close() error

	// LookupCard searches for a card by name and set, returning detailed price match.
	// This is used by lookup endpoints to provide detailed price information.
	// The context parameter enables request cancellation and timeout propagation.
	LookupCard(ctx context.Context, setName string, card domainCards.Card) (*Price, error)

	// GetStats returns provider statistics for monitoring and health checks.
	// Returns nil if statistics are not available.
	// The context parameter enables request cancellation and timeout propagation.
	GetStats(ctx context.Context) *ProviderStats
}

// Card represents the minimal card information needed for price lookups
type Card struct {
	Name   string
	Number string
	Set    string

	// PSAListingTitle is the raw PSA listing title (optional).
	// Reserved for future secondary source matching when normalized queries return no candidates.
	PSAListingTitle string
}

// GradedPrices contains prices for each grade level.
type GradedPrices struct {
	RawCents     int64
	RawNMCents   int64 // JustTCG Near Mint specific (condition-specific, not blended)
	PSA6Cents    int64
	PSA7Cents    int64
	PSA8Cents    int64
	PSA9Cents    int64
	Grade95Cents int64
	PSA10Cents   int64
	BGS10Cents   int64
}

// MarketData contains marketplace activity metrics.
type MarketData struct {
	ActiveListings  int
	LowestListing   int64
	SalesLast30d    int
	SalesLast90d    int
	Volatility      float64
	ListingVelocity float64
	UnitsSold       int
}

// ConservativePrices contains conservative exit price estimates.
type ConservativePrices struct {
	PSA10USD float64
	PSA9USD  float64
	RawUSD   float64
}

// Distributions groups sales distribution data by grade.
type Distributions struct {
	PSA10 *SalesDistribution
	PSA9  *SalesDistribution
	Raw   *SalesDistribution
}

// Price represents pricing information for a card across all grades
type Price struct {
	// Identification
	ID          string
	ProductName string

	// Primary price (PSA 10 for backward compatibility)
	Amount   int64  // Price in cents (PSA 10)
	Currency string // "USD", "EUR", etc.
	Source   Source

	// Per-grade pricing
	Grades GradedPrices

	// Product identification
	UPC string

	// Marketplace data
	Market *MarketData

	// Conservative exit prices
	Conservative *ConservativePrices

	// Sales distributions (percentile data)
	Distributions *Distributions

	// Fusion metadata
	Confidence     float64         // 0.0-1.0 confidence score
	FusionMetadata *FusionMetadata // nil for single-source

	// Last sold data by grade
	LastSoldByGrade *LastSoldByGrade // Recent sales info per grade

	// PriceCharting's raw grade prices (before fusion), used by buildSourcePrices
	// to display the actual PriceCharting price separately from the fused price.
	PCGrades *GradedPrices

	// Per-grade detail data from individual sources (eBay + estimates)
	GradeDetails map[string]*GradeDetail // Keys: "raw", "psa8", "psa9", "psa10"

	// Card-level sales velocity
	Velocity *SalesVelocity

	// Which sources contributed to this price (e.g., ["pricecharting", "doubleholo"])
	Sources []string
}

// FusionMetadata captures multi-source fusion information
type FusionMetadata struct {
	SourceCount   int      // Number of sources used
	OutliersFound int      // Outliers detected and removed
	Method        string   // "weighted_median" or "single_source"
	Sources       []string // ["pricecharting", "ebay"]

	// Per-source results for tracking success/failure rates
	SourceResults []SourceResult
}

// SourceResult tracks the outcome of a price lookup from a specific source
type SourceResult struct {
	Source  string // Source name (e.g., "pricecharting", "doubleholo")
	Success bool   // Whether the lookup succeeded
	Error   string // Error message if failed (empty on success)
}

// GradeSaleInfo contains last sold information for a specific grade
type GradeSaleInfo struct {
	LastSoldPrice float64 // Price in USD
	LastSoldDate  string  // ISO date format (YYYY-MM-DD)
	SaleCount     int     // Number of recent sales
}

// LastSoldByGrade contains last sold data for each grade
type LastSoldByGrade struct {
	PSA10 *GradeSaleInfo
	PSA9  *GradeSaleInfo
	PSA8  *GradeSaleInfo
	PSA7  *GradeSaleInfo
	PSA6  *GradeSaleInfo
	Raw   *GradeSaleInfo
}

// EbayGradeDetail contains eBay sold data for a single grade. All price fields are in cents.
type EbayGradeDetail struct {
	PriceCents   int64   // smartMarketPrice.price (cents)
	Confidence   string  // "high", "medium", "low", or ""
	SalesCount   int     // Number of eBay sales in lookback window
	Trend        string  // "up", "down", "stable", or ""
	MedianCents  int64   // Median sale price (cents)
	MinCents     int64   // Minimum sale price (cents)
	MaxCents     int64   // Maximum sale price (cents)
	Avg7DayCents int64   // 7-day average price in cents (0 if unavailable)
	Volume7Day   float64 // 7-day daily volume (0 if unavailable)
}

// EstimateGradeDetail contains a price estimate for a single grade.
// All price fields are in cents.
type EstimateGradeDetail struct {
	PriceCents int64   // Estimated value (cents)
	LowCents   int64   // Confidence range low (cents)
	HighCents  int64   // Confidence range high (cents)
	Confidence float64 // 0-1 numeric score
}

// GradeDetail combines eBay sold data and estimate data for a single grade.
type GradeDetail struct {
	Ebay     *EbayGradeDetail     // nil if no eBay data for this grade
	Estimate *EstimateGradeDetail // nil if no estimate data for this grade
}

// SalesVelocity contains card-level sales velocity metrics.
type SalesVelocity struct {
	DailyAverage  float64
	WeeklyAverage float64
	MonthlyTotal  int
}

// HintMapping represents a user-provided price hint mapping.
type HintMapping struct {
	CardName        string
	SetName         string
	CollectorNumber string
	Provider        string
	ExternalID      string
}

// PriceHintResolver manages user-provided price hints that override automatic
// external ID resolution. Manual hints are never overwritten by auto-discovery.
type PriceHintResolver interface {
	GetHint(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
	SaveHint(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
	DeleteHint(ctx context.Context, cardName, setName, collectorNumber, provider string) error
	ListHints(ctx context.Context) ([]HintMapping, error)
}

// Source identifies where a price came from
type Source string

// Source name constants — untyped so they work with both Source and string fields.
const (
	SourcePriceCharting = "pricecharting"
	SourceJustTCG       = "justtcg"
	SourceDH            = "doubleholo" // DB provider key — do not change the string value
)
