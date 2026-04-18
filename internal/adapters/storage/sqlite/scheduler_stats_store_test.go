package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerStatsStore_UpsertAndRead(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	store := NewSchedulerStatsStore(db.DB)
	ctx := context.Background()

	// Empty state returns nil, nil — callers treat as "hasn't run."
	got, err := store.Get(ctx, "card_ladder_refresh")
	require.NoError(t, err)
	require.Nil(t, got)

	// Insert once.
	first := SchedulerRunStats{
		Name:       "card_ladder_refresh",
		LastRunAt:  time.Date(2026, 4, 18, 4, 1, 0, 0, time.UTC),
		DurationMs: 12345,
		StatsJSON:  `{"updated":196,"resolved":4}`,
	}
	require.NoError(t, store.Save(ctx, first))

	got, err = store.Get(ctx, "card_ladder_refresh")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, first.Name, got.Name)
	require.Equal(t, first.DurationMs, got.DurationMs)
	require.Equal(t, first.StatsJSON, got.StatsJSON)
	require.True(t, got.LastRunAt.Equal(first.LastRunAt))

	// Upsert overwrites the same name rather than inserting a second row.
	second := SchedulerRunStats{
		Name:       "card_ladder_refresh",
		LastRunAt:  time.Date(2026, 4, 18, 19, 27, 0, 0, time.UTC),
		DurationMs: 99999,
		StatsJSON:  `{"updated":84,"resolved":23}`,
	}
	require.NoError(t, store.Save(ctx, second))

	got, err = store.Get(ctx, "card_ladder_refresh")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, second.DurationMs, got.DurationMs)
	require.Equal(t, second.StatsJSON, got.StatsJSON)

	// A second scheduler key doesn't collide.
	require.NoError(t, store.Save(ctx, SchedulerRunStats{
		Name:       "market_movers_refresh",
		LastRunAt:  time.Date(2026, 4, 18, 5, 0, 0, 0, time.UTC),
		DurationMs: 1,
		StatsJSON:  `{}`,
	}))
	mm, err := store.Get(ctx, "market_movers_refresh")
	require.NoError(t, err)
	require.NotNil(t, mm)
	require.Equal(t, int64(1), mm.DurationMs)
}
