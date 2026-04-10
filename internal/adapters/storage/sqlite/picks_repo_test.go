package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/picks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPick(date time.Time, rank int, cardName string) picks.Pick {
	return picks.Pick{
		Date:              date,
		CardName:          cardName,
		SetName:           "Base Set",
		Grade:             "PSA 9",
		Direction:         picks.DirectionBuy,
		Confidence:        picks.ConfidenceHigh,
		BuyThesis:         "Undervalued card",
		TargetBuyPrice:    50000,
		ExpectedSellPrice: 80000,
		Signals: []picks.Signal{
			{Factor: "market_trend", Direction: picks.SignalBullish, Title: "Rising demand", Detail: "Pop count low"},
			{Factor: "price_history", Direction: picks.SignalNeutral, Title: "Stable pricing", Detail: "No major swings"},
		},
		Rank:   rank,
		Source: picks.SourceAI,
	}
}

func TestPicksSavePicks(t *testing.T) {
	ctx := context.Background()
	date := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	t.Run("inserts batch and round-trips through GetPicksByDate", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		input := []picks.Pick{
			newTestPick(date, 1, "pk-Charizard"),
			newTestPick(date, 2, "pk-Blastoise"),
			newTestPick(date, 3, "pk-Venusaur"),
		}
		// Give each a different direction/confidence for variety
		input[1].Direction = picks.DirectionWatch
		input[1].Confidence = picks.ConfidenceMedium
		input[2].Direction = picks.DirectionAvoid
		input[2].Confidence = picks.ConfidenceLow
		input[2].Signals = nil // test nil signals

		err := repo.SavePicks(ctx, input)
		require.NoError(t, err)

		got, err := repo.GetPicksByDate(ctx, date)
		require.NoError(t, err)
		require.Len(t, got, 3)

		// Verify ordering by rank and IDs assigned
		for i, p := range got {
			assert.Equal(t, i+1, p.Rank)
			assert.NotZero(t, p.ID)
		}

		// Verify first pick round-tripped correctly
		assert.Equal(t, "pk-Charizard", got[0].CardName)
		assert.Equal(t, "Base Set", got[0].SetName)
		assert.Equal(t, "PSA 9", got[0].Grade)
		assert.Equal(t, picks.DirectionBuy, got[0].Direction)
		assert.Equal(t, picks.ConfidenceHigh, got[0].Confidence)
		assert.Equal(t, "Undervalued card", got[0].BuyThesis)
		assert.Equal(t, 50000, got[0].TargetBuyPrice)
		assert.Equal(t, 80000, got[0].ExpectedSellPrice)
		assert.Equal(t, picks.SourceAI, got[0].Source)

		// Verify JSON signal marshaling round-trip
		require.Len(t, got[0].Signals, 2)
		assert.Equal(t, "market_trend", got[0].Signals[0].Factor)
		assert.Equal(t, picks.SignalBullish, got[0].Signals[0].Direction)
		assert.Equal(t, "Rising demand", got[0].Signals[0].Title)
		assert.Equal(t, "Pop count low", got[0].Signals[0].Detail)

		// Verify nil signals stored/read correctly (empty list)
		assert.Empty(t, got[2].Signals)
	})

	t.Run("empty slice is a no-op", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		err := repo.SavePicks(ctx, []picks.Pick{})
		require.NoError(t, err)

		got, err := repo.GetPicksByDate(ctx, date)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestPicksGetPicksByDate(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty for date with no picks", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		got, err := repo.GetPicksByDate(ctx, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("only returns picks for requested date", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		day1 := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
		day2 := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)

		err := repo.SavePicks(ctx, []picks.Pick{
			newTestPick(day1, 1, "pk-Day1Card"),
			newTestPick(day2, 1, "pk-Day2Card"),
		})
		require.NoError(t, err)

		got, err := repo.GetPicksByDate(ctx, day1)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "pk-Day1Card", got[0].CardName)
	})
}

func TestPicksGetPicksRange(t *testing.T) {
	ctx := context.Background()

	t.Run("returns picks within date range ordered by date desc then rank", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		day1 := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
		day2 := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
		day3 := time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)

		err := repo.SavePicks(ctx, []picks.Pick{
			newTestPick(day1, 1, "pk-D1R1"),
			newTestPick(day1, 2, "pk-D1R2"),
			newTestPick(day2, 1, "pk-D2R1"),
			newTestPick(day3, 1, "pk-D3R1"),
		})
		require.NoError(t, err)

		// Range covering day1 and day2 only
		got, err := repo.GetPicksRange(ctx, day1, day2)
		require.NoError(t, err)
		require.Len(t, got, 3)

		// Ordered by pick_date DESC, then rank ASC
		assert.Equal(t, "pk-D2R1", got[0].CardName) // day2, rank 1
		assert.Equal(t, "pk-D1R1", got[1].CardName) // day1, rank 1
		assert.Equal(t, "pk-D1R2", got[2].CardName) // day1, rank 2
	})

	t.Run("returns empty for range with no picks", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		from := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)

		got, err := repo.GetPicksRange(ctx, from, to)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestPicksGetLatestPickDate(t *testing.T) {
	ctx := context.Background()

	t.Run("returns zero time when no picks exist", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		got, err := repo.GetLatestPickDate(ctx)
		require.NoError(t, err)
		assert.True(t, got.IsZero())
	})

	t.Run("returns latest date when picks exist", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		day1 := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
		day2 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

		err := repo.SavePicks(ctx, []picks.Pick{
			newTestPick(day1, 1, "pk-Older"),
			newTestPick(day2, 1, "pk-Newer"),
		})
		require.NoError(t, err)

		got, err := repo.GetLatestPickDate(ctx)
		require.NoError(t, err)
		assert.Equal(t, "2026-03-15", got.Format("2006-01-02"))
	})
}

func TestPicksExistForDate(t *testing.T) {
	ctx := context.Background()
	date := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	t.Run("false when no picks exist", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		exists, err := repo.PicksExistForDate(ctx, date)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("true when picks exist for date", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		err := repo.SavePicks(ctx, []picks.Pick{newTestPick(date, 1, "pk-Exists")})
		require.NoError(t, err)

		exists, err := repo.PicksExistForDate(ctx, date)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("false for different date", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		err := repo.SavePicks(ctx, []picks.Pick{newTestPick(date, 1, "pk-WrongDay")})
		require.NoError(t, err)

		otherDate := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
		exists, err := repo.PicksExistForDate(ctx, otherDate)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestPicksSaveWatchlistItem(t *testing.T) {
	ctx := context.Background()

	t.Run("inserts new watchlist item", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		item := picks.WatchlistItem{
			CardName: "pk-WatchCard",
			SetName:  "Base Set",
			Grade:    "PSA 10",
			Source:   picks.WatchlistManual,
		}
		err := repo.SaveWatchlistItem(ctx, item)
		require.NoError(t, err)

		// Verify via GetActiveWatchlist
		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "pk-WatchCard", items[0].CardName)
		assert.Equal(t, "Base Set", items[0].SetName)
		assert.Equal(t, "PSA 10", items[0].Grade)
		assert.Equal(t, picks.WatchlistManual, items[0].Source)
		assert.True(t, items[0].Active)
		assert.Nil(t, items[0].LatestAssessment)
	})

	t.Run("duplicate active item returns ErrWatchlistDuplicate", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		item := picks.WatchlistItem{
			CardName: "pk-DupCard",
			SetName:  "Jungle",
			Grade:    "PSA 8",
			Source:   picks.WatchlistManual,
		}
		err := repo.SaveWatchlistItem(ctx, item)
		require.NoError(t, err)

		// Same card/set/grade should fail with duplicate error
		err = repo.SaveWatchlistItem(ctx, item)
		require.Error(t, err)
		assert.ErrorIs(t, err, picks.ErrWatchlistDuplicate)
	})

	t.Run("can re-add after soft-delete", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		item := picks.WatchlistItem{
			CardName: "pk-ReaddCard",
			SetName:  "Fossil",
			Grade:    "PSA 7",
			Source:   picks.WatchlistAutoFromPick,
		}
		err := repo.SaveWatchlistItem(ctx, item)
		require.NoError(t, err)

		// Get the ID
		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)

		// Soft-delete
		err = repo.DeleteWatchlistItem(ctx, items[0].ID)
		require.NoError(t, err)

		// Re-add should succeed (unique index is partial: WHERE active = 1)
		err = repo.SaveWatchlistItem(ctx, item)
		require.NoError(t, err)

		activeItems, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		assert.Len(t, activeItems, 1)
	})
}

func TestPicksDeleteWatchlistItem(t *testing.T) {
	ctx := context.Background()

	t.Run("soft-deletes active item", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		item := picks.WatchlistItem{
			CardName: "pk-DeleteMe",
			SetName:  "Base Set",
			Grade:    "PSA 9",
			Source:   picks.WatchlistManual,
		}
		err := repo.SaveWatchlistItem(ctx, item)
		require.NoError(t, err)

		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)

		err = repo.DeleteWatchlistItem(ctx, items[0].ID)
		require.NoError(t, err)

		// Should no longer appear in active list
		items, err = repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("not-found returns ErrWatchlistItemNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		err := repo.DeleteWatchlistItem(ctx, 99999)
		require.Error(t, err)
		assert.ErrorIs(t, err, picks.ErrWatchlistItemNotFound)
	})

	t.Run("double-delete returns ErrWatchlistItemNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		item := picks.WatchlistItem{
			CardName: "pk-DoubleDel",
			SetName:  "Team Rocket",
			Grade:    "PSA 10",
			Source:   picks.WatchlistManual,
		}
		err := repo.SaveWatchlistItem(ctx, item)
		require.NoError(t, err)

		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		id := items[0].ID

		err = repo.DeleteWatchlistItem(ctx, id)
		require.NoError(t, err)

		// Second delete should fail — already inactive
		err = repo.DeleteWatchlistItem(ctx, id)
		require.Error(t, err)
		assert.ErrorIs(t, err, picks.ErrWatchlistItemNotFound)
	})
}

func TestPicksGetActiveWatchlist(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty when no items", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("returns items without picks (LEFT JOIN null)", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		err := repo.SaveWatchlistItem(ctx, picks.WatchlistItem{
			CardName: "pk-NoPick",
			SetName:  "Base Set",
			Grade:    "PSA 9",
			Source:   picks.WatchlistManual,
		})
		require.NoError(t, err)

		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "pk-NoPick", items[0].CardName)
		assert.Nil(t, items[0].LatestAssessment)
	})

	t.Run("returns items with linked pick assessment", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		// Create a pick
		date := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
		pick := newTestPick(date, 1, "pk-AssessCard")
		err := repo.SavePicks(ctx, []picks.Pick{pick})
		require.NoError(t, err)

		// Get the pick ID
		saved, err := repo.GetPicksByDate(ctx, date)
		require.NoError(t, err)
		require.Len(t, saved, 1)
		pickID := saved[0].ID

		// Create watchlist item
		err = repo.SaveWatchlistItem(ctx, picks.WatchlistItem{
			CardName: "pk-AssessCard",
			SetName:  "Base Set",
			Grade:    "PSA 9",
			Source:   picks.WatchlistAutoFromPick,
		})
		require.NoError(t, err)

		// Get watchlist item ID
		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		watchlistID := items[0].ID

		// Link pick to watchlist item
		err = repo.UpdateWatchlistAssessment(ctx, watchlistID, pickID)
		require.NoError(t, err)

		// Fetch again — should have LatestAssessment populated
		items, err = repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.NotNil(t, items[0].LatestAssessment)

		assessment := items[0].LatestAssessment
		assert.Equal(t, pickID, assessment.ID)
		assert.Equal(t, "pk-AssessCard", assessment.CardName)
		assert.Equal(t, picks.DirectionBuy, assessment.Direction)
		assert.Equal(t, picks.ConfidenceHigh, assessment.Confidence)
		assert.Equal(t, 50000, assessment.TargetBuyPrice)
		assert.Equal(t, 80000, assessment.ExpectedSellPrice)

		// Verify signals marshaled through the LEFT JOIN path
		require.Len(t, assessment.Signals, 2)
		assert.Equal(t, "market_trend", assessment.Signals[0].Factor)
		assert.Equal(t, picks.SignalBullish, assessment.Signals[0].Direction)
	})

	t.Run("excludes soft-deleted items", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		err := repo.SaveWatchlistItem(ctx, picks.WatchlistItem{
			CardName: "pk-ActiveCard",
			SetName:  "Base Set",
			Grade:    "PSA 10",
			Source:   picks.WatchlistManual,
		})
		require.NoError(t, err)

		err = repo.SaveWatchlistItem(ctx, picks.WatchlistItem{
			CardName: "pk-InactiveCard",
			SetName:  "Jungle",
			Grade:    "PSA 8",
			Source:   picks.WatchlistManual,
		})
		require.NoError(t, err)

		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 2)

		// Delete the second one
		var inactiveID int
		for _, it := range items {
			if it.CardName == "pk-InactiveCard" {
				inactiveID = it.ID
			}
		}
		require.NotZero(t, inactiveID)
		err = repo.DeleteWatchlistItem(ctx, inactiveID)
		require.NoError(t, err)

		items, err = repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "pk-ActiveCard", items[0].CardName)
	})
}

func TestPicksUpdateWatchlistAssessment(t *testing.T) {
	ctx := context.Background()

	t.Run("links pick to active watchlist item", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		// Create pick
		date := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
		err := repo.SavePicks(ctx, []picks.Pick{newTestPick(date, 1, "pk-LinkCard")})
		require.NoError(t, err)

		saved, err := repo.GetPicksByDate(ctx, date)
		require.NoError(t, err)
		pickID := saved[0].ID

		// Create watchlist item
		err = repo.SaveWatchlistItem(ctx, picks.WatchlistItem{
			CardName: "pk-LinkCard",
			SetName:  "Base Set",
			Grade:    "PSA 9",
			Source:   picks.WatchlistManual,
		})
		require.NoError(t, err)

		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		watchlistID := items[0].ID

		err = repo.UpdateWatchlistAssessment(ctx, watchlistID, pickID)
		require.NoError(t, err)

		// Verify the link
		items, err = repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.NotNil(t, items[0].LatestAssessment)
		assert.Equal(t, pickID, items[0].LatestAssessment.ID)
	})

	t.Run("non-existent watchlist ID returns ErrWatchlistItemNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		err := repo.UpdateWatchlistAssessment(ctx, 99999, 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, picks.ErrWatchlistItemNotFound)
	})

	t.Run("inactive watchlist item returns ErrWatchlistItemNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		// Create and soft-delete
		err := repo.SaveWatchlistItem(ctx, picks.WatchlistItem{
			CardName: "pk-InactiveLink",
			SetName:  "Fossil",
			Grade:    "PSA 7",
			Source:   picks.WatchlistManual,
		})
		require.NoError(t, err)

		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		watchlistID := items[0].ID

		err = repo.DeleteWatchlistItem(ctx, watchlistID)
		require.NoError(t, err)

		// Try to update inactive item
		err = repo.UpdateWatchlistAssessment(ctx, watchlistID, 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, picks.ErrWatchlistItemNotFound)
	})

	t.Run("updates assessment to newer pick", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewPicksRepository(db.DB)

		date1 := time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC)
		date2 := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)

		err := repo.SavePicks(ctx, []picks.Pick{
			newTestPick(date1, 1, "pk-UpdateCard"),
		})
		require.NoError(t, err)

		// Use different set to avoid unique constraint on ai_picks
		pick2 := newTestPick(date2, 1, "pk-UpdateCard")
		pick2.Source = picks.SourceWatchlistReassessment
		err = repo.SavePicks(ctx, []picks.Pick{pick2})
		require.NoError(t, err)

		picks1, err := repo.GetPicksByDate(ctx, date1)
		require.NoError(t, err)
		picks2, err := repo.GetPicksByDate(ctx, date2)
		require.NoError(t, err)

		err = repo.SaveWatchlistItem(ctx, picks.WatchlistItem{
			CardName: "pk-UpdateCard",
			SetName:  "Base Set",
			Grade:    "PSA 9",
			Source:   picks.WatchlistManual,
		})
		require.NoError(t, err)

		items, err := repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		watchlistID := items[0].ID

		// Link first pick
		err = repo.UpdateWatchlistAssessment(ctx, watchlistID, picks1[0].ID)
		require.NoError(t, err)

		// Update to second pick
		err = repo.UpdateWatchlistAssessment(ctx, watchlistID, picks2[0].ID)
		require.NoError(t, err)

		items, err = repo.GetActiveWatchlist(ctx)
		require.NoError(t, err)
		require.NotNil(t, items[0].LatestAssessment)
		assert.Equal(t, picks2[0].ID, items[0].LatestAssessment.ID)
	})
}
