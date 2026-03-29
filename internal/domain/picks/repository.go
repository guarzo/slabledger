package picks

import (
	"context"
	"time"
)

type Repository interface {
	SavePicks(ctx context.Context, picks []Pick) error
	GetPicksByDate(ctx context.Context, date time.Time) ([]Pick, error)
	GetPicksRange(ctx context.Context, from, to time.Time) ([]Pick, error)
	PicksExistForDate(ctx context.Context, date time.Time) (bool, error)
	SaveWatchlistItem(ctx context.Context, item WatchlistItem) error
	DeleteWatchlistItem(ctx context.Context, id int) error
	GetActiveWatchlist(ctx context.Context) ([]WatchlistItem, error)
	UpdateWatchlistAssessment(ctx context.Context, watchlistID int, pickID int) error
}

type ProfitabilityProvider interface {
	GetProfitablePatterns(ctx context.Context) (ProfitabilityProfile, error)
}

type InventoryProvider interface {
	GetHeldCardNames(ctx context.Context) ([]string, error)
}
