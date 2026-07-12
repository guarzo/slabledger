package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPSAPortalSnapshotStore(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	store := NewPSAPortalSnapshotStore(db.DB)
	_, err := db.ExecContext(ctx, `DELETE FROM psa_portal_snapshot`)
	require.NoError(t, err)

	t.Run("empty store returns zero values, no error", func(t *testing.T) {
		rows, fetchedAt, err := store.CurrentSnapshot(ctx)
		require.NoError(t, err)
		require.Nil(t, rows)
		require.True(t, fetchedAt.IsZero())

		at, err := store.SnapshotFetchedAt(ctx)
		require.NoError(t, err)
		require.True(t, at.IsZero())
	})

	t.Run("save and read back", func(t *testing.T) {
		want := []map[string]string{
			{"fct_instantoffers_offers_cert_number": "12345678", "marketplace_listings_listing_title": "Charizard"},
			{"fct_instantoffers_offers_cert_number": "87654321"},
		}
		fetched := time.Now().UTC().Truncate(time.Millisecond)
		require.NoError(t, store.SaveSnapshot(ctx, want, fetched))

		got, gotAt, err := store.CurrentSnapshot(ctx)
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.WithinDuration(t, fetched, gotAt, time.Millisecond)

		at, err := store.SnapshotFetchedAt(ctx)
		require.NoError(t, err)
		require.WithinDuration(t, fetched, at, time.Millisecond)
	})

	t.Run("second save overwrites (singleton row)", func(t *testing.T) {
		newer := []map[string]string{{"fct_instantoffers_offers_cert_number": "11111111"}}
		later := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
		require.NoError(t, store.SaveSnapshot(ctx, newer, later))

		got, gotAt, err := store.CurrentSnapshot(ctx)
		require.NoError(t, err)
		require.Equal(t, newer, got)
		require.WithinDuration(t, later, gotAt, time.Millisecond)

		var count int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM psa_portal_snapshot`).Scan(&count))
		require.Equal(t, 1, count)
	})
}
