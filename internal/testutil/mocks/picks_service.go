package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/picks"
)

type MockPicksService struct {
	GenerateDailyPicksFn  func(ctx context.Context) error
	GetLatestPicksFn      func(ctx context.Context) ([]picks.Pick, error)
	GetPickHistoryFn      func(ctx context.Context, days int) ([]picks.Pick, error)
	AddToWatchlistFn      func(ctx context.Context, item picks.WatchlistItem) error
	RemoveFromWatchlistFn func(ctx context.Context, id int) error
	GetWatchlistFn        func(ctx context.Context) ([]picks.WatchlistItem, error)
}

var _ picks.Service = (*MockPicksService)(nil)

func (m *MockPicksService) GenerateDailyPicks(ctx context.Context) error {
	if m.GenerateDailyPicksFn != nil {
		return m.GenerateDailyPicksFn(ctx)
	}
	return nil
}

func (m *MockPicksService) GetLatestPicks(ctx context.Context) ([]picks.Pick, error) {
	if m.GetLatestPicksFn != nil {
		return m.GetLatestPicksFn(ctx)
	}
	return []picks.Pick{}, nil
}

func (m *MockPicksService) GetPickHistory(ctx context.Context, days int) ([]picks.Pick, error) {
	if m.GetPickHistoryFn != nil {
		return m.GetPickHistoryFn(ctx, days)
	}
	return []picks.Pick{}, nil
}

func (m *MockPicksService) AddToWatchlist(ctx context.Context, item picks.WatchlistItem) error {
	if m.AddToWatchlistFn != nil {
		return m.AddToWatchlistFn(ctx, item)
	}
	return nil
}

func (m *MockPicksService) RemoveFromWatchlist(ctx context.Context, id int) error {
	if m.RemoveFromWatchlistFn != nil {
		return m.RemoveFromWatchlistFn(ctx, id)
	}
	return nil
}

func (m *MockPicksService) GetWatchlist(ctx context.Context) ([]picks.WatchlistItem, error) {
	if m.GetWatchlistFn != nil {
		return m.GetWatchlistFn(ctx)
	}
	return []picks.WatchlistItem{}, nil
}
