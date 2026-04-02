package intelligence

import (
	"context"
	"time"
)

// Repository stores and retrieves market intelligence data.
type Repository interface {
	Store(ctx context.Context, intel *MarketIntelligence) error
	GetByCard(ctx context.Context, cardName, setName, cardNumber string) (*MarketIntelligence, error)
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
