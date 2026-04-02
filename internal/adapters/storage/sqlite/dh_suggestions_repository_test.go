package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/stretchr/testify/require"
)

func newSuggestionsRepo(t *testing.T) (*DHSuggestionsRepository, func()) {
	t.Helper()
	db := setupTestDB(t)
	repo := NewDHSuggestionsRepository(db.DB)
	return repo, func() { db.Close() }
}

func makeSuggestion(date, typ, category string, rank int, cardName, setName string) intelligence.Suggestion {
	return intelligence.Suggestion{
		SuggestionDate:    date,
		Type:              typ,
		Category:          category,
		Rank:              rank,
		IsManual:          false,
		DHCardID:          "dh-" + cardName,
		CardName:          cardName,
		SetName:           setName,
		CardNumber:        "1",
		ImageURL:          "https://example.com/img.jpg",
		CurrentPriceCents: 5000,
		ConfidenceScore:   0.85,
		Reasoning:         "Strong momentum",
		SentimentScore:    0.7,
		SentimentTrend:    0.1,
		SentimentMentions: 25,
		FetchedAt:         time.Now().Truncate(time.Second).UTC(),
	}
}

func TestDHSuggestions_StoreAndGetByDate(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	suggestions := []intelligence.Suggestion{
		makeSuggestion("2026-04-01", "cards", "hottest_cards", 1, "Charizard", "Base Set"),
		makeSuggestion("2026-04-01", "cards", "hottest_cards", 2, "Blastoise", "Base Set"),
		makeSuggestion("2026-04-01", "cards", "consider_selling", 1, "Pikachu", "Jungle"),
	}

	err := repo.StoreSuggestions(ctx, suggestions)
	require.NoError(t, err)

	got, err := repo.GetByDate(ctx, "2026-04-01")
	require.NoError(t, err)
	require.Len(t, got, 3)

	// Verify ordering: type, category, rank
	require.Equal(t, "consider_selling", got[0].Category)
	require.Equal(t, "hottest_cards", got[1].Category)
	require.Equal(t, 1, got[1].Rank)
	require.Equal(t, 2, got[2].Rank)
}

func TestDHSuggestions_StoreSuggestions_ReplacesExisting(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Store initial set
	initial := []intelligence.Suggestion{
		makeSuggestion("2026-04-01", "cards", "hottest_cards", 1, "Charizard", "Base Set"),
		makeSuggestion("2026-04-01", "cards", "hottest_cards", 2, "Blastoise", "Base Set"),
	}
	require.NoError(t, repo.StoreSuggestions(ctx, initial))

	got, err := repo.GetByDate(ctx, "2026-04-01")
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Replace with a different set
	replacement := []intelligence.Suggestion{
		makeSuggestion("2026-04-01", "cards", "hottest_cards", 1, "Venusaur", "Base Set"),
	}
	require.NoError(t, repo.StoreSuggestions(ctx, replacement))

	got, err = repo.GetByDate(ctx, "2026-04-01")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Venusaur", got[0].CardName)
}

func TestDHSuggestions_StoreSuggestions_EmptySlice(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	err := repo.StoreSuggestions(ctx, nil)
	require.NoError(t, err)

	err = repo.StoreSuggestions(ctx, []intelligence.Suggestion{})
	require.NoError(t, err)
}

func TestDHSuggestions_GetByDate_NotFound(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	got, err := repo.GetByDate(ctx, "2099-01-01")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDHSuggestions_GetLatest(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Store suggestions across two dates
	older := []intelligence.Suggestion{
		makeSuggestion("2026-03-31", "cards", "hottest_cards", 1, "Pikachu", "Jungle"),
	}
	newer := []intelligence.Suggestion{
		makeSuggestion("2026-04-01", "cards", "hottest_cards", 1, "Charizard", "Base Set"),
		makeSuggestion("2026-04-01", "cards", "consider_selling", 1, "Blastoise", "Base Set"),
	}

	require.NoError(t, repo.StoreSuggestions(ctx, older))
	require.NoError(t, repo.StoreSuggestions(ctx, newer))

	got, err := repo.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "2026-04-01", got[0].SuggestionDate)
}

func TestDHSuggestions_GetLatest_Empty(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	got, err := repo.GetLatest(ctx)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDHSuggestions_GetCardSuggestions(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	suggestions := []intelligence.Suggestion{
		makeSuggestion("2026-04-01", "cards", "hottest_cards", 1, "Charizard", "Base Set"),
		makeSuggestion("2026-04-01", "cards", "hottest_cards", 2, "Blastoise", "Base Set"),
	}
	require.NoError(t, repo.StoreSuggestions(ctx, suggestions))

	// Also store for a different date with Charizard again
	more := []intelligence.Suggestion{
		makeSuggestion("2026-04-02", "cards", "hottest_cards", 1, "Charizard", "Base Set"),
	}
	require.NoError(t, repo.StoreSuggestions(ctx, more))

	got, err := repo.GetCardSuggestions(ctx, "Charizard", "Base Set")
	require.NoError(t, err)
	require.Len(t, got, 2)
	// Most recent date first
	require.Equal(t, "2026-04-02", got[0].SuggestionDate)
	require.Equal(t, "2026-04-01", got[1].SuggestionDate)
}

func TestDHSuggestions_GetCardSuggestions_NotFound(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	got, err := repo.GetCardSuggestions(ctx, "Nonexistent", "Set")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDHSuggestions_RoundTrip_AllFields(t *testing.T) {
	repo, cleanup := newSuggestionsRepo(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second).UTC()

	s := intelligence.Suggestion{
		SuggestionDate:      "2026-04-01",
		Type:                "cards",
		Category:            "hottest_cards",
		Rank:                1,
		IsManual:            true,
		DHCardID:            "dh-charizard",
		CardName:            "Charizard",
		SetName:             "Base Set",
		CardNumber:          "4",
		ImageURL:            "https://example.com/charizard.jpg",
		CurrentPriceCents:   150000,
		ConfidenceScore:     0.92,
		Reasoning:           "Historic high demand",
		StructuredReasoning: `{"factor":"demand","weight":0.8}`,
		Metrics:             `{"views":1000,"saves":50}`,
		SentimentScore:      0.88,
		SentimentTrend:      0.15,
		SentimentMentions:   200,
		FetchedAt:           now,
	}

	require.NoError(t, repo.StoreSuggestions(ctx, []intelligence.Suggestion{s}))

	got, err := repo.GetByDate(ctx, "2026-04-01")
	require.NoError(t, err)
	require.Len(t, got, 1)

	g := got[0]
	require.Equal(t, "2026-04-01", g.SuggestionDate)
	require.Equal(t, "cards", g.Type)
	require.Equal(t, "hottest_cards", g.Category)
	require.Equal(t, 1, g.Rank)
	require.True(t, g.IsManual)
	require.Equal(t, "dh-charizard", g.DHCardID)
	require.Equal(t, "Charizard", g.CardName)
	require.Equal(t, "Base Set", g.SetName)
	require.Equal(t, "4", g.CardNumber)
	require.Equal(t, "https://example.com/charizard.jpg", g.ImageURL)
	require.Equal(t, int64(150000), g.CurrentPriceCents)
	require.InDelta(t, 0.92, g.ConfidenceScore, 0.001)
	require.Equal(t, "Historic high demand", g.Reasoning)
	require.Equal(t, `{"factor":"demand","weight":0.8}`, g.StructuredReasoning)
	require.Equal(t, `{"views":1000,"saves":50}`, g.Metrics)
	require.InDelta(t, 0.88, g.SentimentScore, 0.001)
	require.InDelta(t, 0.15, g.SentimentTrend, 0.001)
	require.Equal(t, 200, g.SentimentMentions)
}
