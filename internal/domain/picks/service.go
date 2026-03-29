package picks

import "context"

type Service interface {
	GenerateDailyPicks(ctx context.Context) error
	GetLatestPicks(ctx context.Context) ([]Pick, error)
	GetPickHistory(ctx context.Context, days int) ([]Pick, error)
	AddToWatchlist(ctx context.Context, item WatchlistItem) error
	RemoveFromWatchlist(ctx context.Context, id int) error
	GetWatchlist(ctx context.Context) ([]WatchlistItem, error)
}
