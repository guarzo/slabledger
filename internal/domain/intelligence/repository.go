package intelligence

import (
	"context"
	"time"
)

// CardKey identifies a card by its name, set, and collector number.
type CardKey struct {
	CardName   string
	SetName    string
	CardNumber string
}

// Repository stores and retrieves market intelligence data.
type Repository interface {
	Store(ctx context.Context, intel *MarketIntelligence) error
	GetByCard(ctx context.Context, cardName, setName, cardNumber string) (*MarketIntelligence, error)
	GetByCards(ctx context.Context, keys []CardKey) (map[CardKey]*MarketIntelligence, error)
	GetByDHCardID(ctx context.Context, dhCardID string) (*MarketIntelligence, error)
	GetStale(ctx context.Context, maxAge time.Duration, limit int) ([]MarketIntelligence, error)
}

// SuggestionsRepository stores and retrieves daily buy/sell suggestions.
type SuggestionsRepository interface {
	StoreSuggestions(ctx context.Context, suggestions []Suggestion) error
	GetByDate(ctx context.Context, date string) ([]Suggestion, error)
	GetLatest(ctx context.Context) ([]Suggestion, error)
	GetCardSuggestions(ctx context.Context, cardName, setName string) ([]Suggestion, error)
}

// TrajectoryRepository stores and retrieves weekly-aggregated price
// trajectories keyed by DH card ID.
type TrajectoryRepository interface {
	// Upsert writes the given buckets for a card, overwriting any existing
	// rows with matching week_start values. refreshedAt is stamped on every
	// written row so we can tell when the aggregate last ran.
	Upsert(ctx context.Context, dhCardID string, buckets []WeeklyBucket) error
	// GetByDHCardID returns all stored buckets for a card in chronological
	// order. Returns an empty slice when nothing is stored.
	GetByDHCardID(ctx context.Context, dhCardID string) ([]WeeklyBucket, error)
	// LatestWeekStart returns the most recent week_start stored for the card.
	// Returns zero time when nothing is stored — caller treats that as
	// "backfill from scratch".
	LatestWeekStart(ctx context.Context, dhCardID string) (time.Time, error)
}
