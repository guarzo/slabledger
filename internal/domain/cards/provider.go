package cards

import (
	"context"
	"time"
)

// SearchCriteria defines search parameters for card queries
type SearchCriteria struct {
	// Query is the raw search text (e.g., "Charizard Base Set")
	Query string

	// CardName is the extracted card name (e.g., "Charizard")
	CardName string

	// SetName is the extracted set name (e.g., "Base Set")
	SetName string

	// CardNumber is the extracted card number (e.g., "4" or "4/102")
	CardNumber string

	// Limit is the maximum number of results to return
	Limit int
}

// CardProvider defines what the domain needs from card data sources
type CardProvider interface {
	// GetCards fetches all cards from a set
	GetCards(ctx context.Context, setID string) ([]Card, error)

	// GetSet fetches set metadata
	GetSet(ctx context.Context, setID string) (*Set, error)

	// ListAllSets fetches all available sets
	ListAllSets(ctx context.Context) ([]Set, error)

	// SearchCards searches for cards matching the given criteria.
	// Implementations should limit results and avoid full catalogue scans.
	// Returns cards sorted by relevance (most relevant first) and total match count.
	SearchCards(ctx context.Context, criteria SearchCriteria) ([]Card, int, error)

	// Available returns true if provider is ready
	Available() bool
}

// Card is the domain model with full data from the card provider
type Card struct {
	ID       string
	Name     string
	Number   string
	Set      string // Set ID
	SetName  string // Set display name
	Rarity   string
	Language string // Language of the card (e.g., "English", "Japanese")
	ImageURL string

	// Raw PSA listing title (optional). Reserved for secondary source matching
	// as a fallback query when normalized queries return no candidates.
	PSAListingTitle string

	// Pricing data from various sources
	MarketPrice Money // Simple embedded price for basic use cases
}

// Set represents a Pokemon card set
type Set struct {
	ID          string
	Name        string
	Series      string
	TotalCards  int
	ReleaseDate string
}

// Money represents a monetary value
type Money struct {
	Cents    int64  // Always use cents to avoid float precision issues
	Currency string // "USD", "EUR", etc.
}

// CacheStats represents the state of a card data cache.
type CacheStats struct {
	Enabled         bool           `json:"enabled"`
	TotalSets       int            `json:"totalSets,omitempty"`
	FinalizedSets   int            `json:"finalizedSets,omitempty"`
	DiscoveredSets  int            `json:"discoveredSets,omitempty"`
	LastUpdated     time.Time      `json:"lastUpdated,omitempty"`
	RegistryVersion string         `json:"registryVersion,omitempty"`
	Sets            []CacheSetInfo `json:"sets,omitempty"`
}

// CacheSetInfo contains cache status for a single card set.
type CacheSetInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Series      string    `json:"series,omitempty"`
	ReleaseDate string    `json:"releaseDate"`
	TotalCards  int       `json:"totalCards"`
	Status      string    `json:"status"`
	FetchedAt   time.Time `json:"fetchedAt,omitempty"`
}

// NewSetIDsProvider filters a list of set IDs to only those not yet finalized.
// Implemented by the set registry manager in the tcgdex adapter.
type NewSetIDsProvider interface {
	GetNewSetIDs(ctx context.Context, allSetIDs []string) ([]string, error)
}
