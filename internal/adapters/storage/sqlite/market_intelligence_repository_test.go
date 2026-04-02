package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/stretchr/testify/require"
)

func newIntelRepo(t *testing.T) (*MarketIntelligenceRepository, func()) {
	t.Helper()
	db := setupTestDB(t)
	repo := NewMarketIntelligenceRepository(db.DB)
	return repo, func() { db.Close() }
}

func TestMarketIntelligence_StoreAndGetByCard(t *testing.T) {
	repo, cleanup := newIntelRepo(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second).UTC()

	intel := &intelligence.MarketIntelligence{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
		DHCardID:   "dh-001",
		Sentiment: &intelligence.Sentiment{
			Score:        0.85,
			MentionCount: 42,
			Trend:        "rising",
		},
		Forecast: &intelligence.Forecast{
			PredictedPriceCents: 150000,
			Confidence:          0.72,
			ForecastDate:        now.Add(30 * 24 * time.Hour),
		},
		GradingROI: []intelligence.GradeROI{
			{Grade: "PSA 10", AvgSaleCents: 200000, ROI: 1.5},
			{Grade: "PSA 9", AvgSaleCents: 80000, ROI: 0.8},
		},
		RecentSales: []intelligence.Sale{
			{SoldAt: now.Add(-24 * time.Hour), GradingCompany: "PSA", Grade: "10", PriceCents: 195000, Platform: "eBay"},
		},
		Population: []intelligence.PopulationEntry{
			{GradingCompany: "PSA", Grade: "10", Count: 122},
			{GradingCompany: "PSA", Grade: "9", Count: 456},
		},
		Insights: &intelligence.Insights{
			Headline: "Strong upward trend",
			Detail:   "PSA 10 population is low relative to demand.",
		},
		FetchedAt: now,
	}

	err := repo.Store(ctx, intel)
	require.NoError(t, err)

	got, err := repo.GetByCard(ctx, "Charizard", "Base Set", "4")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.Equal(t, "Charizard", got.CardName)
	require.Equal(t, "Base Set", got.SetName)
	require.Equal(t, "4", got.CardNumber)
	require.Equal(t, "dh-001", got.DHCardID)

	// Sentiment
	require.NotNil(t, got.Sentiment)
	require.InDelta(t, 0.85, got.Sentiment.Score, 0.001)
	require.Equal(t, 42, got.Sentiment.MentionCount)
	require.Equal(t, "rising", got.Sentiment.Trend)

	// Forecast
	require.NotNil(t, got.Forecast)
	require.Equal(t, int64(150000), got.Forecast.PredictedPriceCents)
	require.InDelta(t, 0.72, got.Forecast.Confidence, 0.001)

	// Grading ROI
	require.Len(t, got.GradingROI, 2)
	require.Equal(t, "PSA 10", got.GradingROI[0].Grade)
	require.Equal(t, int64(200000), got.GradingROI[0].AvgSaleCents)

	// Recent Sales
	require.Len(t, got.RecentSales, 1)
	require.Equal(t, "eBay", got.RecentSales[0].Platform)
	require.Equal(t, int64(195000), got.RecentSales[0].PriceCents)

	// Population
	require.Len(t, got.Population, 2)
	require.Equal(t, 122, got.Population[0].Count)

	// Insights
	require.NotNil(t, got.Insights)
	require.Equal(t, "Strong upward trend", got.Insights.Headline)
	require.Equal(t, "PSA 10 population is low relative to demand.", got.Insights.Detail)
}

func TestMarketIntelligence_StoreAndGetByCard_NullableNil(t *testing.T) {
	repo, cleanup := newIntelRepo(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second).UTC()

	intel := &intelligence.MarketIntelligence{
		CardName:   "Pikachu",
		SetName:    "Jungle",
		CardNumber: "60",
		DHCardID:   "dh-002",
		FetchedAt:  now,
		// All nullable fields left nil
	}

	err := repo.Store(ctx, intel)
	require.NoError(t, err)

	got, err := repo.GetByCard(ctx, "Pikachu", "Jungle", "60")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Nil(t, got.Sentiment)
	require.Nil(t, got.Forecast)
	require.Nil(t, got.Insights)
	require.Empty(t, got.GradingROI)
	require.Empty(t, got.RecentSales)
	require.Empty(t, got.Population)
}

func TestMarketIntelligence_GetByCard_NotFound(t *testing.T) {
	repo, cleanup := newIntelRepo(t)
	defer cleanup()
	ctx := context.Background()

	got, err := repo.GetByCard(ctx, "Nonexistent", "Set", "0")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestMarketIntelligence_GetByDHCardID(t *testing.T) {
	repo, cleanup := newIntelRepo(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second).UTC()
	intel := &intelligence.MarketIntelligence{
		CardName:   "Blastoise",
		SetName:    "Base Set",
		CardNumber: "2",
		DHCardID:   "dh-blast-001",
		FetchedAt:  now,
	}
	require.NoError(t, repo.Store(ctx, intel))

	got, err := repo.GetByDHCardID(ctx, "dh-blast-001")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Blastoise", got.CardName)
}

func TestMarketIntelligence_GetByDHCardID_NotFound(t *testing.T) {
	repo, cleanup := newIntelRepo(t)
	defer cleanup()
	ctx := context.Background()

	got, err := repo.GetByDHCardID(ctx, "nonexistent")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestMarketIntelligence_GetStale(t *testing.T) {
	repo, cleanup := newIntelRepo(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second).UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	// Insert an old record
	require.NoError(t, repo.Store(ctx, &intelligence.MarketIntelligence{
		CardName:   "OldCard",
		SetName:    "OldSet",
		CardNumber: "1",
		DHCardID:   "dh-old",
		FetchedAt:  old,
	}))

	// Insert a recent record
	require.NoError(t, repo.Store(ctx, &intelligence.MarketIntelligence{
		CardName:   "NewCard",
		SetName:    "NewSet",
		CardNumber: "2",
		DHCardID:   "dh-new",
		FetchedAt:  recent,
	}))

	// Only the old record should be stale with 24h threshold
	stale, err := repo.GetStale(ctx, 24*time.Hour, 10)
	require.NoError(t, err)
	require.Len(t, stale, 1)
	require.Equal(t, "OldCard", stale[0].CardName)
}

func TestMarketIntelligence_StoreUpsert(t *testing.T) {
	repo, cleanup := newIntelRepo(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second).UTC()

	// Initial store
	require.NoError(t, repo.Store(ctx, &intelligence.MarketIntelligence{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
		DHCardID:   "dh-001",
		Sentiment:  &intelligence.Sentiment{Score: 0.5, MentionCount: 10, Trend: "stable"},
		FetchedAt:  now,
	}))

	// Update with new values
	require.NoError(t, repo.Store(ctx, &intelligence.MarketIntelligence{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
		DHCardID:   "dh-001-updated",
		Sentiment:  &intelligence.Sentiment{Score: 0.9, MentionCount: 99, Trend: "rising"},
		FetchedAt:  now.Add(time.Hour),
	}))

	got, err := repo.GetByCard(ctx, "Charizard", "Base Set", "4")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "dh-001-updated", got.DHCardID)
	require.InDelta(t, 0.9, got.Sentiment.Score, 0.001)
	require.Equal(t, 99, got.Sentiment.MentionCount)
}

func TestMarketIntelligence_GetStale_LimitRespected(t *testing.T) {
	repo, cleanup := newIntelRepo(t)
	defer cleanup()
	ctx := context.Background()

	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second).UTC()

	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Store(ctx, &intelligence.MarketIntelligence{
			CardName:   "Card" + string(rune('A'+i)),
			SetName:    "Set",
			CardNumber: string(rune('0' + i)),
			DHCardID:   "dh-" + string(rune('0'+i)),
			FetchedAt:  old.Add(time.Duration(i) * time.Minute),
		}))
	}

	stale, err := repo.GetStale(ctx, 24*time.Hour, 3)
	require.NoError(t, err)
	require.Len(t, stale, 3)
}
