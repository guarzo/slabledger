package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/picks"
)

type MockPicksRepository struct {
	SavePicksFn                 func(ctx context.Context, p []picks.Pick) error
	GetPicksByDateFn            func(ctx context.Context, date time.Time) ([]picks.Pick, error)
	GetPicksRangeFn             func(ctx context.Context, from, to time.Time) ([]picks.Pick, error)
	PicksExistForDateFn         func(ctx context.Context, date time.Time) (bool, error)
	SaveWatchlistItemFn         func(ctx context.Context, item picks.WatchlistItem) error
	DeleteWatchlistItemFn       func(ctx context.Context, id int) error
	GetActiveWatchlistFn        func(ctx context.Context) ([]picks.WatchlistItem, error)
	UpdateWatchlistAssessmentFn func(ctx context.Context, watchlistID int, pickID int) error
}

var _ picks.Repository = (*MockPicksRepository)(nil)

func (m *MockPicksRepository) SavePicks(ctx context.Context, p []picks.Pick) error {
	if m.SavePicksFn != nil {
		return m.SavePicksFn(ctx, p)
	}
	return nil
}

func (m *MockPicksRepository) GetPicksByDate(ctx context.Context, date time.Time) ([]picks.Pick, error) {
	if m.GetPicksByDateFn != nil {
		return m.GetPicksByDateFn(ctx, date)
	}
	return nil, nil
}

func (m *MockPicksRepository) GetPicksRange(ctx context.Context, from, to time.Time) ([]picks.Pick, error) {
	if m.GetPicksRangeFn != nil {
		return m.GetPicksRangeFn(ctx, from, to)
	}
	return nil, nil
}

func (m *MockPicksRepository) PicksExistForDate(ctx context.Context, date time.Time) (bool, error) {
	if m.PicksExistForDateFn != nil {
		return m.PicksExistForDateFn(ctx, date)
	}
	return false, nil
}

func (m *MockPicksRepository) SaveWatchlistItem(ctx context.Context, item picks.WatchlistItem) error {
	if m.SaveWatchlistItemFn != nil {
		return m.SaveWatchlistItemFn(ctx, item)
	}
	return nil
}

func (m *MockPicksRepository) DeleteWatchlistItem(ctx context.Context, id int) error {
	if m.DeleteWatchlistItemFn != nil {
		return m.DeleteWatchlistItemFn(ctx, id)
	}
	return nil
}

func (m *MockPicksRepository) GetActiveWatchlist(ctx context.Context) ([]picks.WatchlistItem, error) {
	if m.GetActiveWatchlistFn != nil {
		return m.GetActiveWatchlistFn(ctx)
	}
	return nil, nil
}

func (m *MockPicksRepository) UpdateWatchlistAssessment(ctx context.Context, watchlistID int, pickID int) error {
	if m.UpdateWatchlistAssessmentFn != nil {
		return m.UpdateWatchlistAssessmentFn(ctx, watchlistID, pickID)
	}
	return nil
}

type MockProfitabilityProvider struct {
	GetProfitablePatternsFn func(ctx context.Context) (picks.ProfitabilityProfile, error)
}

var _ picks.ProfitabilityProvider = (*MockProfitabilityProvider)(nil)

func (m *MockProfitabilityProvider) GetProfitablePatterns(ctx context.Context) (picks.ProfitabilityProfile, error) {
	if m.GetProfitablePatternsFn != nil {
		return m.GetProfitablePatternsFn(ctx)
	}
	return picks.ProfitabilityProfile{}, nil
}

type MockInventoryProvider struct {
	GetHeldCardNamesFn func(ctx context.Context) ([]string, error)
}

var _ picks.InventoryProvider = (*MockInventoryProvider)(nil)

func (m *MockInventoryProvider) GetHeldCardNames(ctx context.Context) ([]string, error) {
	if m.GetHeldCardNamesFn != nil {
		return m.GetHeldCardNamesFn(ctx)
	}
	return nil, nil
}
