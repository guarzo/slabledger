package fusion

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// ResponseMeta carries transport-agnostic metadata from a source fetch.
// Adapters populate this from HTTP status codes and rate-limit headers.
type ResponseMeta struct {
	StatusCode     int
	RateLimitReset *time.Time
}

// FetchResult bundles fusion data with optional per-grade details.
// Returning details inline eliminates shared mutable state (destructive-read maps)
// and makes concurrent FetchFusionData calls for the same card safe.
type FetchResult struct {
	GradeData       map[string][]PriceData
	EbayDetails     map[string]*pricing.EbayGradeDetail
	Velocity        *pricing.SalesVelocity
	EstimateDetails map[string]*pricing.EstimateGradeDetail
}

// SecondaryPriceSource defines the interface for secondary price data sources
// that contribute fusion pricing data.
// Each source fetches raw data from its API and converts it to fusion-compatible format.
type SecondaryPriceSource interface {
	// FetchFusionData retrieves price data and returns it in fusion-ready format.
	// Returns a FetchResult containing grade-keyed price data and optional detail data,
	// response metadata (status code, rate limit info), and any error.
	FetchFusionData(ctx context.Context, card pricing.Card) (*FetchResult, *ResponseMeta, error)
	// Available returns true if the source is configured and ready for use.
	Available() bool
	// Name returns the source identifier used for logging and tracking.
	Name() string
}

// CardIDResolver resolves card names to external provider IDs.
// Implementations cache the mapping to avoid repeated search API calls.
type CardIDResolver interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
	// GetExternalIDFresh returns the cached external ID only if it was updated within maxAge.
	// Returns "" if no mapping exists or the mapping is stale.
	GetExternalIDFresh(ctx context.Context, cardName, setName, collectorNumber, provider string, maxAge time.Duration) (string, error)
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
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
