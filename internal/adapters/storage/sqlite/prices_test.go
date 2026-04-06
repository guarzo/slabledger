package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	logger := mocks.NewMockLogger()

	db, err := Open(":memory:", logger)
	require.NoError(t, err)

	err = RunMigrations(db, "migrations")
	require.NoError(t, err)

	return db
}

func TestPriceRepository_StorePrice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	entry := &pricing.PriceEntry{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4/102",
		Grade:      "PSA 10",
		PriceCents: 100000,
		Confidence: 0.95,
		Source:     "doubleholo",
		PriceDate:  time.Now(),
	}

	err := repo.StorePrice(ctx, entry)
	require.NoError(t, err)

	// Verify stored
	retrieved, err := repo.GetLatestPrice(ctx, pricing.Card{
		Name: entry.CardName,
		Set:  entry.SetName,
	}, entry.Grade, entry.Source)

	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, entry.CardName, retrieved.CardName)
	require.Equal(t, entry.PriceCents, retrieved.PriceCents)
}

func TestPriceRepository_StorePrice_Upsert(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	priceDate := time.Now().Truncate(time.Second)

	// Store initial price
	entry1 := &pricing.PriceEntry{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4/102",
		Grade:      "PSA 10",
		PriceCents: 100000,
		Confidence: 0.95,
		Source:     "doubleholo",
		PriceDate:  priceDate,
	}
	err := repo.StorePrice(ctx, entry1)
	require.NoError(t, err)

	// Store updated price with same key
	entry2 := &pricing.PriceEntry{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4/102",
		Grade:      "PSA 10",
		PriceCents: 105000, // Updated price
		Confidence: 0.96,
		Source:     "doubleholo",
		PriceDate:  priceDate, // Same date - should upsert
	}
	err = repo.StorePrice(ctx, entry2)
	require.NoError(t, err)

	// Verify latest price is the updated one
	retrieved, err := repo.GetLatestPrice(ctx, pricing.Card{
		Name: "Charizard",
		Set:  "Base Set",
	}, "PSA 10", "doubleholo")

	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, int64(105000), retrieved.PriceCents)
	require.Equal(t, 0.96, retrieved.Confidence)
}

func TestPriceRepository_GetLatestPrice_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	retrieved, err := repo.GetLatestPrice(ctx, pricing.Card{
		Name: "NonExistent",
		Set:  "Test Set",
	}, "PSA 10", "doubleholo")

	require.NoError(t, err)
	require.Nil(t, retrieved, "should return nil for non-existent price")
}

func TestPriceRepository_GetStalePrices(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Store a price with old timestamp
	entry := &pricing.PriceEntry{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4/102",
		Grade:      "PSA 10",
		PriceCents: 100000,
		Source:     "doubleholo",
		PriceDate:  time.Now().Add(-48 * time.Hour),
	}
	err := repo.StorePrice(ctx, entry)
	require.NoError(t, err)

	// Manually update timestamp to be old (simulate stale data)
	_, err = db.Exec(`UPDATE price_history SET updated_at = DATETIME('now', '-48 hours') WHERE card_name = ?`, entry.CardName)
	require.NoError(t, err)

	// Get stale prices
	stalePrices, err := repo.GetStalePrices(ctx, "", 100)
	require.NoError(t, err)
	require.Greater(t, len(stalePrices), 0, "should find stale prices")

	found := false
	for _, sp := range stalePrices {
		if sp.CardName == "Charizard" {
			found = true
			require.Greater(t, sp.HoursOld, 24.0, "should be older than 24 hours")
		}
	}
	require.True(t, found, "should find Charizard in stale prices")
}

func TestPriceRepository_GetStalePrices_FilterBySource(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Store prices from different sources
	entries := []*pricing.PriceEntry{
		{
			CardName:   "Charizard",
			SetName:    "Base Set",
			Grade:      "PSA 10",
			PriceCents: 100000,
			Source:     "ebay",
			PriceDate:  time.Now().Add(-48 * time.Hour),
		},
		{
			CardName:   "Blastoise",
			SetName:    "Base Set",
			Grade:      "PSA 10",
			PriceCents: 80000,
			Source:     "doubleholo",
			PriceDate:  time.Now().Add(-48 * time.Hour),
		},
	}

	for _, entry := range entries {
		err := repo.StorePrice(ctx, entry)
		require.NoError(t, err)
	}

	// Make both stale
	_, err := db.Exec(`UPDATE price_history SET updated_at = DATETIME('now', '-48 hours')`)
	require.NoError(t, err)

	// Get stale prices filtered by source
	stalePrices, err := repo.GetStalePrices(ctx, "ebay", 100)
	require.NoError(t, err)

	// Should only have ebay entries
	for _, sp := range stalePrices {
		require.Equal(t, "ebay", sp.Source)
	}
}

func TestPriceRepository_RecordAPICall(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	call := &pricing.APICallRecord{
		Provider:   "doubleholo",
		Endpoint:   "/api/cards",
		StatusCode: 200,
		LatencyMS:  150,
		Timestamp:  time.Now(),
	}

	err := repo.RecordAPICall(ctx, call)
	require.NoError(t, err)

	// Verify recorded
	stats, err := repo.GetAPIUsage(ctx, "doubleholo")
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Equal(t, int64(1), stats.TotalCalls)
}

func TestPriceRepository_RecordAPICall_RateLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Record successful call
	call1 := &pricing.APICallRecord{
		Provider:   "doubleholo",
		Endpoint:   "/api/cards",
		StatusCode: 200,
		LatencyMS:  150,
		Timestamp:  time.Now(),
	}
	err := repo.RecordAPICall(ctx, call1)
	require.NoError(t, err)

	// Record rate limited call
	call2 := &pricing.APICallRecord{
		Provider:   "doubleholo",
		Endpoint:   "/api/cards",
		StatusCode: 429,
		Error:      "rate limited",
		LatencyMS:  50,
		Timestamp:  time.Now(),
	}
	err = repo.RecordAPICall(ctx, call2)
	require.NoError(t, err)

	// Verify stats
	stats, err := repo.GetAPIUsage(ctx, "doubleholo")
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.TotalCalls)
	require.Equal(t, int64(1), stats.RateLimitHits)
}

func TestPriceRepository_IsProviderBlocked(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Should not be blocked initially
	blocked, _, err := repo.IsProviderBlocked(ctx, "doubleholo")
	require.NoError(t, err)
	require.False(t, blocked)

	// Block provider
	blockedUntil := time.Now().Add(1 * time.Hour)
	err = repo.UpdateRateLimit(ctx, "doubleholo", blockedUntil)
	require.NoError(t, err)

	// Should be blocked now
	blocked, until, err := repo.IsProviderBlocked(ctx, "doubleholo")
	require.NoError(t, err)
	require.True(t, blocked)
	require.True(t, until.After(time.Now()))
}

func TestPriceRepository_IsProviderBlocked_Expired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Block provider with past timestamp (already expired)
	blockedUntil := time.Now().Add(-1 * time.Hour)
	err := repo.UpdateRateLimit(ctx, "doubleholo", blockedUntil)
	require.NoError(t, err)

	// Should not be blocked (expired)
	blocked, _, err := repo.IsProviderBlocked(ctx, "doubleholo")
	require.NoError(t, err)
	require.False(t, blocked)
}

func TestPriceRepository_RecordCardAccess(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	err := repo.RecordCardAccess(ctx, "Charizard", "Base Set", "analysis")
	require.NoError(t, err)

	// Verify recorded
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM card_access_log WHERE card_name = ?`, "Charizard").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestPriceRepository_CleanupOldAccessLogs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Insert some access logs with different timestamps
	_, err := db.Exec(`
		INSERT INTO card_access_log (card_name, set_name, access_type, accessed_at) VALUES
		('Card1', 'Set1', 'analysis', DATETIME('now', '-60 days')),
		('Card2', 'Set2', 'search', DATETIME('now', '-40 days')),
		('Card3', 'Set3', 'analysis', DATETIME('now', '-10 days')),
		('Card4', 'Set4', 'analysis', DATETIME('now', '-1 days'))
	`)
	require.NoError(t, err)

	// Verify all 4 records exist
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM card_access_log`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 4, count)

	// Cleanup logs older than 30 days
	deleted, err := repo.CleanupOldAccessLogs(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted) // Card1 (60 days) and Card2 (40 days) should be deleted

	// Verify only 2 records remain
	err = db.QueryRow(`SELECT COUNT(*) FROM card_access_log`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	// Verify the remaining records are the recent ones
	var names []string
	rows, err := db.Query(`SELECT card_name FROM card_access_log ORDER BY card_name`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.Equal(t, []string{"Card3", "Card4"}, names)
}

func TestPriceRepository_Ping(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	err := repo.Ping(ctx)
	require.NoError(t, err)
}

func TestPriceRepository_GetStalePrices_DedupByCard(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Store multiple grades and sources for the SAME card
	grades := []string{"PSA 10", "PSA 9", "PSA 8", "Raw"}
	sources := []string{"ebay", "doubleholo"}

	for _, grade := range grades {
		for _, source := range sources {
			entry := &pricing.PriceEntry{
				CardName:   "Charizard",
				SetName:    "Base Set",
				CardNumber: "4/102",
				Grade:      grade,
				PriceCents: 100000,
				Source:     source,
				PriceDate:  time.Now().Add(-48 * time.Hour),
			}
			err := repo.StorePrice(ctx, entry)
			require.NoError(t, err)
		}
	}

	// Store a DIFFERENT card with one grade/source
	entry := &pricing.PriceEntry{
		CardName:   "Blastoise",
		SetName:    "Base Set",
		CardNumber: "2/102",
		Grade:      "PSA 10",
		PriceCents: 50000,
		Source:     "doubleholo",
		PriceDate:  time.Now().Add(-48 * time.Hour),
	}
	err := repo.StorePrice(ctx, entry)
	require.NoError(t, err)

	// Make all entries stale
	_, err = db.Exec(`UPDATE price_history SET updated_at = DATETIME('now', '-48 hours')`)
	require.NoError(t, err)

	// Get stale prices — should deduplicate to 2 unique cards, not 8+ rows
	stalePrices, err := repo.GetStalePrices(ctx, "", 100)
	require.NoError(t, err)

	// We should get exactly 2 rows (one per unique card), not 8 (4 grades * 2 sources)
	require.Equal(t, 2, len(stalePrices), "should deduplicate to 2 unique cards")

	// Verify both cards are represented
	cardNames := make(map[string]bool)
	for _, sp := range stalePrices {
		cardNames[sp.CardName] = true
	}
	require.True(t, cardNames["Charizard"], "should include Charizard")
	require.True(t, cardNames["Blastoise"], "should include Blastoise")
}

func TestPriceRepository_GetLatestPricesBySource(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Store multiple grades for the same card/source
	grades := []string{"PSA 10", "PSA 9", "PSA 8", "Raw"}
	priceCents := []int64{100000, 80000, 60000, 20000}
	now := time.Now()

	for i, grade := range grades {
		entry := &pricing.PriceEntry{
			CardName:   "Charizard",
			SetName:    "Base Set",
			CardNumber: "4/102",
			Grade:      grade,
			PriceCents: priceCents[i],
			Confidence: 0.95,
			Source:     "doubleholo",
			PriceDate:  now,
		}
		err := repo.StorePrice(ctx, entry)
		require.NoError(t, err)
	}

	// Also store a price for a different source (should NOT be returned)
	err := repo.StorePrice(ctx, &pricing.PriceEntry{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4/102",
		Grade:      "PSA 10",
		PriceCents: 110000,
		Source:     "ebay",
		PriceDate:  now,
	})
	require.NoError(t, err)

	// Store a price for the same name/set/source but different card_number (should NOT be returned)
	err = repo.StorePrice(ctx, &pricing.PriceEntry{
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
		Grade:      "PSA 10",
		PriceCents: 999999,
		Source:     "doubleholo",
		PriceDate:  now,
	})
	require.NoError(t, err)

	// Retrieve all grades for doubleholo within 48h window — filtered to card_number "4/102"
	result, err := repo.GetLatestPricesBySource(ctx, "Charizard", "Base Set", "4/102", "doubleholo", 48*time.Hour)
	require.NoError(t, err)
	require.Len(t, result, 4, "should return all 4 grades for doubleholo with card_number 4/102")

	// Verify each grade
	for i, grade := range grades {
		entry, ok := result[grade]
		require.True(t, ok, "should have grade %s", grade)
		require.Equal(t, priceCents[i], entry.PriceCents, "grade %s price mismatch", grade)
		require.Equal(t, "Charizard", entry.CardName)
		require.Equal(t, "Base Set", entry.SetName)
		require.Equal(t, "4/102", entry.CardNumber, "grade %s card number should be populated", grade)
		require.Equal(t, "doubleholo", entry.Source)
	}
}

func TestPriceRepository_GetLatestPricesBySource_VariantsDoNotCollide(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()
	now := time.Now()

	// Store PSA 10 price for variant #25
	err := repo.StorePrice(ctx, &pricing.PriceEntry{
		CardName: "Pikachu", SetName: "Celebrations", CardNumber: "25",
		Grade: "PSA 10", PriceCents: 5000, Source: "doubleholo", PriceDate: now,
	})
	require.NoError(t, err)

	// Store PSA 10 price for variant #SWSH039 (same name/set, different number)
	err = repo.StorePrice(ctx, &pricing.PriceEntry{
		CardName: "Pikachu", SetName: "Celebrations", CardNumber: "SWSH039",
		Grade: "PSA 10", PriceCents: 12000, Source: "doubleholo", PriceDate: now,
	})
	require.NoError(t, err)

	// Querying for #25 should return 5000, not 12000
	result25, err := repo.GetLatestPricesBySource(ctx, "Pikachu", "Celebrations", "25", "doubleholo", 48*time.Hour)
	require.NoError(t, err)
	require.Len(t, result25, 1)
	require.Equal(t, int64(5000), result25["PSA 10"].PriceCents, "variant #25 price should be 5000")
	require.Equal(t, "25", result25["PSA 10"].CardNumber)

	// Querying for #SWSH039 should return 12000, not 5000
	resultSW, err := repo.GetLatestPricesBySource(ctx, "Pikachu", "Celebrations", "SWSH039", "doubleholo", 48*time.Hour)
	require.NoError(t, err)
	require.Len(t, resultSW, 1)
	require.Equal(t, int64(12000), resultSW["PSA 10"].PriceCents, "variant #SWSH039 price should be 12000")
	require.Equal(t, "SWSH039", resultSW["PSA 10"].CardNumber)
}

func TestPriceRepository_GetLatestPricesBySource_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// No data stored — should return empty map, not error
	result, err := repo.GetLatestPricesBySource(ctx, "NonExistent", "No Set", "", "doubleholo", 48*time.Hour)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestPriceRepository_GetLatestPricesBySource_StaleFiltered(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Store a price
	err := repo.StorePrice(ctx, &pricing.PriceEntry{
		CardName:   "Charizard",
		SetName:    "Base Set",
		Grade:      "PSA 10",
		PriceCents: 100000,
		Source:     "doubleholo",
		PriceDate:  time.Now(),
	})
	require.NoError(t, err)

	// Make it old
	_, err = db.Exec(`UPDATE price_history SET updated_at = DATETIME('now', '-72 hours') WHERE card_name = 'Charizard'`)
	require.NoError(t, err)

	// Query with a 24h window — should exclude the stale entry
	result, err := repo.GetLatestPricesBySource(ctx, "Charizard", "Base Set", "", "doubleholo", 24*time.Hour)
	require.NoError(t, err)
	require.Empty(t, result, "stale entries should be filtered out")

	// Query with a 96h window — should include the entry
	result, err = repo.GetLatestPricesBySource(ctx, "Charizard", "Base Set", "", "doubleholo", 96*time.Hour)
	require.NoError(t, err)
	require.Len(t, result, 1, "entry within maxAge should be returned")
}

func TestPriceRepository_GetAPIUsage_NoData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Get usage for provider with no recorded calls
	stats, err := repo.GetAPIUsage(ctx, "nonexistent")
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Equal(t, "nonexistent", stats.Provider)
	require.Equal(t, int64(0), stats.TotalCalls)
}
