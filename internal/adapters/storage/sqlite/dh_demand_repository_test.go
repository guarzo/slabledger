package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/domain/demand"
)

func newDemandRepo(t *testing.T) (*DHDemandRepository, func()) {
	t.Helper()
	db := setupTestDB(t)
	repo := NewDHDemandRepository(db.DB)
	return repo, func() { db.Close() }
}

func strPtr(s string) *string           { return &s }
func floatPtr(f float64) *float64       { return &f }
func timePtr(t time.Time) *time.Time    { return &t }

func TestDHDemand_UpsertAndGetCardCache(t *testing.T) {
	repo, cleanup := newDemandRepo(t)
	defer cleanup()
	ctx := context.Background()

	fetched := time.Now().Truncate(time.Second).UTC()
	computed := fetched.Add(-1 * time.Hour)

	initial := demand.CardCache{
		CardID:              "card-123",
		Window:              "30d",
		DemandScore:         floatPtr(0.72),
		DemandDataQuality:   strPtr("proxy"),
		DemandJSON:          strPtr(`{"demand_score":0.72,"views":412}`),
		VelocityJSON:        strPtr(`{"median_days_to_sell":9.8}`),
		AnalyticsComputedAt: timePtr(computed),
		DemandComputedAt:    timePtr(computed),
		FetchedAt:           fetched,
	}

	require.NoError(t, repo.UpsertCardCache(ctx, initial))

	got, err := repo.GetCardCache(ctx, "card-123", "30d")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "card-123", got.CardID)
	require.Equal(t, "30d", got.Window)
	require.NotNil(t, got.DemandScore)
	require.InDelta(t, 0.72, *got.DemandScore, 0.0001)
	require.Equal(t, "proxy", *got.DemandDataQuality)
	require.JSONEq(t, `{"demand_score":0.72,"views":412}`, *got.DemandJSON)
	require.JSONEq(t, `{"median_days_to_sell":9.8}`, *got.VelocityJSON)
	require.Nil(t, got.TrendJSON)
	require.Nil(t, got.SaturationJSON)
	require.Nil(t, got.PriceDistributionJSON)
	require.NotNil(t, got.AnalyticsComputedAt)
	require.True(t, got.AnalyticsComputedAt.Equal(computed))
	require.True(t, got.FetchedAt.Equal(fetched))

	// Update path: upsert again with different values + newly populated fields.
	fetched2 := fetched.Add(24 * time.Hour)
	updated := initial
	updated.DemandScore = floatPtr(0.85)
	updated.DemandDataQuality = strPtr("full")
	updated.TrendJSON = strPtr(`{"7d":{"direction":"rising"}}`)
	updated.FetchedAt = fetched2

	require.NoError(t, repo.UpsertCardCache(ctx, updated))

	got2, err := repo.GetCardCache(ctx, "card-123", "30d")
	require.NoError(t, err)
	require.NotNil(t, got2)
	require.InDelta(t, 0.85, *got2.DemandScore, 0.0001)
	require.Equal(t, "full", *got2.DemandDataQuality)
	require.NotNil(t, got2.TrendJSON)
	require.JSONEq(t, `{"7d":{"direction":"rising"}}`, *got2.TrendJSON)
	require.True(t, got2.FetchedAt.Equal(fetched2))
}

func TestDHDemand_GetCardCache_NotFound(t *testing.T) {
	repo, cleanup := newDemandRepo(t)
	defer cleanup()
	ctx := context.Background()

	got, err := repo.GetCardCache(ctx, "missing-card", "30d")
	require.NoError(t, err, "not-found must not produce an error")
	require.Nil(t, got)
}

func TestDHDemand_UpsertCardCache_AllNullableFieldsNil(t *testing.T) {
	repo, cleanup := newDemandRepo(t)
	defer cleanup()
	ctx := context.Background()

	row := demand.CardCache{
		CardID:    "analytics-only",
		Window:    "7d",
		FetchedAt: time.Now().Truncate(time.Second).UTC(),
	}
	require.NoError(t, repo.UpsertCardCache(ctx, row))

	got, err := repo.GetCardCache(ctx, "analytics-only", "7d")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Nil(t, got.DemandScore)
	require.Nil(t, got.DemandDataQuality)
	require.Nil(t, got.DemandJSON)
	require.Nil(t, got.VelocityJSON)
	require.Nil(t, got.TrendJSON)
	require.Nil(t, got.SaturationJSON)
	require.Nil(t, got.PriceDistributionJSON)
	require.Nil(t, got.AnalyticsComputedAt)
	require.Nil(t, got.DemandComputedAt)
}

func TestDHDemand_ListCardCacheByDemandScore(t *testing.T) {
	repo, cleanup := newDemandRepo(t)
	defer cleanup()
	ctx := context.Background()

	fetched := time.Now().Truncate(time.Second).UTC()

	rows := []demand.CardCache{
		{CardID: "a", Window: "30d", DemandScore: floatPtr(0.2), FetchedAt: fetched},
		{CardID: "b", Window: "30d", DemandScore: floatPtr(0.9), FetchedAt: fetched},
		{CardID: "c", Window: "30d", DemandScore: floatPtr(0.5), FetchedAt: fetched},
		{CardID: "d", Window: "30d", DemandScore: nil, FetchedAt: fetched},                // should be excluded
		{CardID: "e", Window: "7d", DemandScore: floatPtr(0.99), FetchedAt: fetched},      // wrong window
	}
	for _, r := range rows {
		require.NoError(t, repo.UpsertCardCache(ctx, r))
	}

	got, err := repo.ListCardCacheByDemandScore(ctx, "30d", 10)
	require.NoError(t, err)
	require.Len(t, got, 3, "should exclude null-score row and 7d-window row")
	require.Equal(t, "b", got[0].CardID)
	require.Equal(t, "c", got[1].CardID)
	require.Equal(t, "a", got[2].CardID)

	// Limit respected
	limited, err := repo.ListCardCacheByDemandScore(ctx, "30d", 2)
	require.NoError(t, err)
	require.Len(t, limited, 2)
	require.Equal(t, "b", limited[0].CardID)
	require.Equal(t, "c", limited[1].CardID)
}

func TestDHDemand_CardDataQualityStats(t *testing.T) {
	repo, cleanup := newDemandRepo(t)
	defer cleanup()
	ctx := context.Background()

	fetched := time.Now().Truncate(time.Second).UTC()
	rows := []demand.CardCache{
		{CardID: "p1", Window: "30d", DemandDataQuality: strPtr("proxy"), FetchedAt: fetched},
		{CardID: "p2", Window: "30d", DemandDataQuality: strPtr("proxy"), FetchedAt: fetched},
		{CardID: "f1", Window: "30d", DemandDataQuality: strPtr("full"), FetchedAt: fetched},
		{CardID: "n1", Window: "30d", FetchedAt: fetched},
		{CardID: "other-window", Window: "7d", DemandDataQuality: strPtr("proxy"), FetchedAt: fetched},
	}
	for _, r := range rows {
		require.NoError(t, repo.UpsertCardCache(ctx, r))
	}

	stats, err := repo.CardDataQualityStats(ctx, "30d")
	require.NoError(t, err)
	require.Equal(t, 2, stats.ProxyCount)
	require.Equal(t, 1, stats.FullCount)
	require.Equal(t, 1, stats.NullQualityCount)
	require.Equal(t, 4, stats.TotalRows)
}

func TestDHDemand_UpsertAndGetCharacterCache(t *testing.T) {
	repo, cleanup := newDemandRepo(t)
	defer cleanup()
	ctx := context.Background()

	fetched := time.Now().Truncate(time.Second).UTC()
	initial := demand.CharacterCache{
		Character:           "Umbreon",
		Window:              "30d",
		DemandJSON:          strPtr(`{"character":"Umbreon","total_views":843,"by_era":{"sword_shield":{"avg_demand_score":0.82}}}`),
		VelocityJSON:        strPtr(`{"median_days_to_sell":9.8}`),
		SaturationJSON:      strPtr(`{"active_listing_count":42}`),
		DemandComputedAt:    timePtr(fetched.Add(-2 * time.Hour)),
		AnalyticsComputedAt: timePtr(fetched.Add(-1 * time.Hour)),
		FetchedAt:           fetched,
	}

	require.NoError(t, repo.UpsertCharacterCache(ctx, initial))

	got, err := repo.GetCharacterCache(ctx, "Umbreon", "30d")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Umbreon", got.Character)
	require.JSONEq(t, *initial.DemandJSON, *got.DemandJSON)
	require.JSONEq(t, *initial.VelocityJSON, *got.VelocityJSON)
	require.JSONEq(t, *initial.SaturationJSON, *got.SaturationJSON)

	// Update
	fetched2 := fetched.Add(24 * time.Hour)
	updated := initial
	updated.SaturationJSON = strPtr(`{"active_listing_count":75}`)
	updated.FetchedAt = fetched2
	require.NoError(t, repo.UpsertCharacterCache(ctx, updated))

	got2, err := repo.GetCharacterCache(ctx, "Umbreon", "30d")
	require.NoError(t, err)
	require.JSONEq(t, `{"active_listing_count":75}`, *got2.SaturationJSON)
	require.True(t, got2.FetchedAt.Equal(fetched2))
}

func TestDHDemand_GetCharacterCache_NotFound(t *testing.T) {
	repo, cleanup := newDemandRepo(t)
	defer cleanup()
	ctx := context.Background()

	got, err := repo.GetCharacterCache(ctx, "Nobody", "30d")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestDHDemand_ListCharacterCache(t *testing.T) {
	repo, cleanup := newDemandRepo(t)
	defer cleanup()
	ctx := context.Background()

	fetched := time.Now().Truncate(time.Second).UTC()
	rows := []demand.CharacterCache{
		{Character: "Umbreon", Window: "30d", FetchedAt: fetched},
		{Character: "Charizard", Window: "30d", FetchedAt: fetched},
		{Character: "Blastoise", Window: "30d", FetchedAt: fetched},
		{Character: "OtherWindow", Window: "7d", FetchedAt: fetched}, // should be excluded
	}
	for _, r := range rows {
		require.NoError(t, repo.UpsertCharacterCache(ctx, r))
	}

	got, err := repo.ListCharacterCache(ctx, "30d")
	require.NoError(t, err)
	require.Len(t, got, 3)
	// Ordered alphabetically
	require.Equal(t, "Blastoise", got[0].Character)
	require.Equal(t, "Charizard", got[1].Character)
	require.Equal(t, "Umbreon", got[2].Character)
}
