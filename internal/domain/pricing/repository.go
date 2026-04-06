package pricing

import (
	"context"
	"time"
)

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

// PricingDiagnostics summarizes pricing data quality across the inventory.
type PricingDiagnostics struct {
	TotalMappedCards int              `json:"totalMappedCards"`
	UnmappedCards    int              `json:"unmappedCards"`
	RecentFailures   []FailureSummary `json:"recentFailures"`
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

// RefreshCandidate identifies a card needing a price refresh.
// Derived from campaign inventory (unsold purchases) rather than price_history.
type RefreshCandidate struct {
	CardName        string
	CardNumber      string
	SetName         string
	PSAListingTitle string
}

// RefreshCandidateProvider lists cards whose prices should be refreshed.
type RefreshCandidateProvider interface {
	GetRefreshCandidates(ctx context.Context, limit int) ([]RefreshCandidate, error)
}
