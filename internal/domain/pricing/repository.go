package pricing

import (
	"context"
	"time"
)

// PriceRepository defines the persistence interface for price data.
// This interface is implemented by adapters (e.g., SQLite) and used by domain services.
type PriceRepository interface {
	// Price persistence
	StorePrice(ctx context.Context, entry *PriceEntry) error
	GetLatestPrice(ctx context.Context, card Card, grade string, source string) (*PriceEntry, error)

	// GetLatestPricesBySource retrieves the latest price entry per grade for a given
	// card/source combination. Only entries with updated_at within maxAge are returned.
	// The returned map is keyed by grade string (e.g. "PSA 10", "Raw").
	// When cardNumber is non-empty, results are filtered to that specific card variant.
	GetLatestPricesBySource(ctx context.Context, cardName, setName, cardNumber, source string, maxAge time.Duration) (map[string]PriceEntry, error)

	// DeletePricesByCard removes all price history entries for a specific card identity.
	// Used to clean up stale entries when card resolution changes (e.g., name correction).
	// When cardNumber is non-empty, only the specific variant is deleted.
	DeletePricesByCard(ctx context.Context, cardName, setName, cardNumber string) (int64, error)

	// Stale price detection for selective refresh
	// Note: Staleness thresholds are defined in the stale_prices VIEW based on price value:
	// - High value (>$100): stale after 12 hours
	// - Medium value ($50-$100): stale after 24 hours
	// - Low value (<$50): stale after 48 hours
	GetStalePrices(ctx context.Context, source string, limit int) ([]StalePrice, error)
}

// APITracker defines the interface for tracking API calls and rate limiting.
type APITracker interface {
	RecordAPICall(ctx context.Context, call *APICallRecord) error
	GetAPIUsage(ctx context.Context, provider string) (*APIUsageStats, error)
	UpdateRateLimit(ctx context.Context, provider string, blockedUntil time.Time) error
	IsProviderBlocked(ctx context.Context, provider string) (bool, time.Time, error)
}

// AccessTracker defines the interface for tracking card access patterns.
type AccessTracker interface {
	RecordCardAccess(ctx context.Context, cardName, setName, accessType string) error
	// CleanupOldAccessLogs removes access logs older than the specified retention period.
	// This is a maintenance operation to prevent unbounded growth of the access log table.
	CleanupOldAccessLogs(ctx context.Context, retentionDays int) (int64, error)
}

// HealthChecker defines the interface for health checks.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// PriceEntry represents a single price observation
type PriceEntry struct {
	CardName   string
	SetName    string
	CardNumber string
	Grade      string

	PriceCents int64
	Confidence float64
	Source     string

	// Fusion metadata
	FusionSourceCount     int
	FusionOutliersRemoved int
	FusionMethod          string

	PriceDate time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// StalePrice identifies prices needing refresh
type StalePrice struct {
	CardName        string
	CardNumber      string
	SetName         string
	Grade           string
	Source          string
	DaysOld         float64
	HoursOld        float64
	LastPrice       int64
	Priority        int
	PSAListingTitle string // From campaign_purchases (empty if no purchase match)
}

// APICallRecord tracks individual API calls
type APICallRecord struct {
	Provider   string
	Endpoint   string
	StatusCode int
	Error      string
	LatencyMS  int64
	Timestamp  time.Time
}

// APIUsageStats summarizes API usage
type APIUsageStats struct {
	Provider      string
	TotalCalls    int64
	ErrorCalls    int64
	RateLimitHits int64
	AvgLatencyMS  float64
	LastCallAt    time.Time
	CallsLastHour int64
	CallsLast5Min int64
	BlockedUntil  *time.Time
}

// DiscoveryFailure records a card that failed source discovery (e.g., CardHedger card-match).
type DiscoveryFailure struct {
	CardName      string
	SetName       string
	CardNumber    string
	Provider      string // "cardhedger"
	FailureReason string // "no_match", "low_confidence", "api_error"
	Query         string // The query string attempted
	Attempts      int
	LastAttempted time.Time
	CreatedAt     time.Time
}

// DiscoveryFailureTracker persists and queries discovery failures for diagnostics.
type DiscoveryFailureTracker interface {
	RecordDiscoveryFailure(ctx context.Context, f *DiscoveryFailure) error
	ClearDiscoveryFailure(ctx context.Context, cardName, setName, cardNumber, provider string) error
	ListDiscoveryFailures(ctx context.Context, provider string, limit int) ([]DiscoveryFailure, error)
	CountDiscoveryFailures(ctx context.Context, provider string) (int, error)
}

// PricingDiagnostics summarizes pricing data quality across the inventory.
type PricingDiagnostics struct {
	TotalCards        int              `json:"totalCards"`
	FullFusionCards   int              `json:"fullFusionCards"`
	PartialCards      int              `json:"partialCards"`
	PCOnlyCards       int              `json:"pcOnlyCards"`
	SourceCoverage    map[string]int   `json:"sourceCoverage"`
	PCOnlyCardList    []DiagnosticCard `json:"pcOnlyCardList"`
	DiscoveryFailures int              `json:"discoveryFailures"`
	RecentFailures    []FailureSummary `json:"recentFailures"`
}

// DiagnosticCard identifies a card with limited source coverage.
type DiagnosticCard struct {
	CardName   string    `json:"cardName"`
	SetName    string    `json:"setName"`
	CardNumber string    `json:"cardNumber"`
	Sources    []string  `json:"sources"`
	PriceUsd   float64   `json:"priceUsd"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// FailureSummary aggregates recent API failures by provider and error type.
type FailureSummary struct {
	Provider  string    `json:"provider"`
	ErrorType string    `json:"errorType"`
	Count     int       `json:"count"`
	LastSeen  time.Time `json:"lastSeen"`
}

// PricingDiagnosticsProvider queries pricing data quality metrics.
type PricingDiagnosticsProvider interface {
	GetPricingDiagnostics(ctx context.Context) (*PricingDiagnostics, error)
}
