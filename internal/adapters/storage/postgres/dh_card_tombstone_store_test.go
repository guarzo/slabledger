package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDHCardTombstoneStore(t *testing.T) {
	db := setupTestDB(t)
	store := NewDHCardTombstoneStore(db.DB)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `TRUNCATE TABLE dh_card_tombstones`)
	require.NoError(t, err)

	ts, err := store.IsTombstoned(ctx, 100)
	require.NoError(t, err)
	require.False(t, ts)

	n, err := store.RecordFailure(ctx, 100, "404")
	require.NoError(t, err)
	require.Equal(t, 1, n)
	ts, _ = store.IsTombstoned(ctx, 100)
	require.False(t, ts)

	_, _ = store.RecordFailure(ctx, 100, "404")
	n, _ = store.RecordFailure(ctx, 100, "404")
	require.Equal(t, 3, n)
	ts, _ = store.IsTombstoned(ctx, 100)
	require.True(t, ts)

	c, err := store.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, c)

	require.NoError(t, store.Clear(ctx, 100))
	ts, _ = store.IsTombstoned(ctx, 100)
	require.False(t, ts)

	_, _ = store.RecordFailure(ctx, 200, "x")
	_, _ = store.RecordFailure(ctx, 200, "x")
	_, _ = store.RecordFailure(ctx, 200, "x")
	cleared, err := store.ClearAll(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, cleared)
}
