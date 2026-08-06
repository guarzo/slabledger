package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/stretchr/testify/require"
)

func TestPSAPortalCatalogStore(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	store := NewPSAPortalCatalogStore(db.DB)
	_, err := db.ExecContext(ctx, `DELETE FROM psa_portal_catalog`)
	require.NoError(t, err)

	t.Run("empty store returns zero values, no error", func(t *testing.T) {
		lists, fetchedAt, err := store.SpecLists(ctx)
		require.NoError(t, err)
		require.Empty(t, lists)
		require.True(t, fetchedAt.IsZero())

		subjects, fetchedAt, err := store.Subjects(ctx, psacampaign.PokemonCategoryID)
		require.NoError(t, err)
		require.Empty(t, subjects)
		require.True(t, fetchedAt.IsZero())
	})

	t.Run("refuses to save an empty spec-list catalog", func(t *testing.T) {
		err := store.SaveSpecLists(ctx, nil)
		require.Error(t, err)
	})

	t.Run("refuses to save an empty subject catalog", func(t *testing.T) {
		err := store.SaveSubjects(ctx, psacampaign.PokemonCategoryID, []psacampaign.SubjectRef{})
		require.Error(t, err)
	})

	t.Run("save and read back spec lists", func(t *testing.T) {
		// Synthetic fixture values — not real portal UUIDs.
		want := []psacampaign.SpecListRef{
			{ID: "fixture-uuid-japanese-pokemon", Name: "Japanese Pokemon", Status: "ENABLED"},
			{ID: "fixture-uuid-english-pokemon", Name: "English Pokemon", Status: "ENABLED"},
		}
		require.NoError(t, store.SaveSpecLists(ctx, want))

		got, fetchedAt, err := store.SpecLists(ctx)
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.WithinDuration(t, time.Now(), fetchedAt, 5*time.Second)
	})

	t.Run("second save overwrites (upsert on kind+key)", func(t *testing.T) {
		newer := []psacampaign.SpecListRef{
			{ID: "fixture-uuid-riftbound", Name: "Riftbound", Status: "ENABLED"},
		}
		require.NoError(t, store.SaveSpecLists(ctx, newer))

		got, _, err := store.SpecLists(ctx)
		require.NoError(t, err)
		require.Equal(t, newer, got)

		var count int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT count(*) FROM psa_portal_catalog WHERE kind = 'spec_lists'`).Scan(&count))
		require.Equal(t, 1, count)
	})

	t.Run("save and read back subjects, keyed by category id", func(t *testing.T) {
		// Synthetic fixture subject ids — arbitrary, not from any real capture.
		want := []psacampaign.SubjectRef{
			{ID: 990001, Name: "Fixture Charizard"},
			{ID: 990002, Name: "Fixture Pikachu"},
		}
		require.NoError(t, store.SaveSubjects(ctx, psacampaign.PokemonCategoryID, want))

		got, fetchedAt, err := store.Subjects(ctx, psacampaign.PokemonCategoryID)
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.WithinDuration(t, time.Now(), fetchedAt, 5*time.Second)

		// A different category id has its own row and stays empty.
		otherCategory, _, err := store.Subjects(ctx, psacampaign.PokemonCategoryID+1)
		require.NoError(t, err)
		require.Empty(t, otherCategory)
	})
}
