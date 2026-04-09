package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- UpdatePurchasePriceOverride ----------

func TestPurchasesPricing_UpdatePurchasePriceOverride(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pp-camp-1", "Override Campaign")

	t.Run("set override", func(t *testing.T) {
		p := newTestPurchase("pp-camp-1", "PP000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchasePriceOverride(ctx, p.ID, 12500, "manual")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 12500, got.OverridePriceCents)
		assert.Equal(t, campaigns.OverrideSourceManual, got.OverrideSource)
		assert.NotEmpty(t, got.OverrideSetAt)
	})

	t.Run("clear with zero", func(t *testing.T) {
		p := newTestPurchase("pp-camp-1", "PP000002")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchasePriceOverride(ctx, p.ID, 5000, "manual"))

		err := repo.UpdatePurchasePriceOverride(ctx, p.ID, 0, "manual")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, got.OverridePriceCents)
		assert.Equal(t, campaigns.OverrideSourceNone, got.OverrideSource)
		assert.Empty(t, got.OverrideSetAt)
	})

	t.Run("clear with negative", func(t *testing.T) {
		p := newTestPurchase("pp-camp-1", "PP000003")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchasePriceOverride(ctx, p.ID, 9000, "cost_markup"))

		err := repo.UpdatePurchasePriceOverride(ctx, p.ID, -1, "cost_markup")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, got.OverridePriceCents)
		assert.Equal(t, campaigns.OverrideSourceNone, got.OverrideSource)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchasePriceOverride(ctx, "pp-nonexistent", 100, "manual")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

// ---------- UpdateReviewedPrice ----------

func TestPurchasesPricing_UpdateReviewedPrice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pp-camp-2", "Reviewed Campaign")

	t.Run("set reviewed price", func(t *testing.T) {
		p := newTestPurchase("pp-camp-2", "PP100001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdateReviewedPrice(ctx, p.ID, 15000, "manual")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 15000, got.ReviewedPriceCents)
		assert.Equal(t, campaigns.ReviewSourceManual, got.ReviewSource)
		assert.NotEmpty(t, got.ReviewedAt, "reviewed_at should be set")
	})

	t.Run("clear with zero", func(t *testing.T) {
		p := newTestPurchase("pp-camp-2", "PP100002")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdateReviewedPrice(ctx, p.ID, 10000, "manual"))

		err := repo.UpdateReviewedPrice(ctx, p.ID, 0, "manual")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, got.ReviewedPriceCents)
		assert.Equal(t, campaigns.ReviewSource(""), got.ReviewSource)
		assert.Empty(t, got.ReviewedAt, "reviewed_at should be cleared")
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdateReviewedPrice(ctx, "pp-nonexistent", 5000, "manual")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

// ---------- UpdatePurchaseAISuggestion / ClearPurchaseAISuggestion ----------

func TestPurchasesPricing_AISuggestion(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pp-camp-3", "AI Suggestion Campaign")

	t.Run("set suggestion", func(t *testing.T) {
		p := newTestPurchase("pp-camp-3", "PP200001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseAISuggestion(ctx, p.ID, 20000)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 20000, got.AISuggestedPriceCents)
		assert.NotEmpty(t, got.AISuggestedAt)
	})

	t.Run("set with zero clears", func(t *testing.T) {
		p := newTestPurchase("pp-camp-3", "PP200002")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p.ID, 15000))

		err := repo.UpdatePurchaseAISuggestion(ctx, p.ID, 0)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, got.AISuggestedPriceCents)
		assert.Empty(t, got.AISuggestedAt)
	})

	t.Run("set with negative clears", func(t *testing.T) {
		p := newTestPurchase("pp-camp-3", "PP200003")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p.ID, 8000))

		err := repo.UpdatePurchaseAISuggestion(ctx, p.ID, -5)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, got.AISuggestedPriceCents)
	})

	t.Run("clear suggestion", func(t *testing.T) {
		p := newTestPurchase("pp-camp-3", "PP200004")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p.ID, 30000))

		err := repo.ClearPurchaseAISuggestion(ctx, p.ID)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, got.AISuggestedPriceCents)
		assert.Empty(t, got.AISuggestedAt)
	})

	t.Run("update not found", func(t *testing.T) {
		err := repo.UpdatePurchaseAISuggestion(ctx, "pp-nonexistent", 1000)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})

	t.Run("clear not found", func(t *testing.T) {
		err := repo.ClearPurchaseAISuggestion(ctx, "pp-nonexistent")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

// ---------- AcceptAISuggestion ----------

func TestPurchasesPricing_AcceptAISuggestion(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pp-camp-4", "Accept AI Campaign")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("pp-camp-4", "PP300001")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p.ID, 25000))

		err := repo.AcceptAISuggestion(ctx, p.ID, 25000)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 25000, got.OverridePriceCents)
		assert.Equal(t, campaigns.OverrideSourceAIAccepted, got.OverrideSource)
		assert.NotEmpty(t, got.OverrideSetAt)
		// AI suggestion should be cleared after accept
		assert.Equal(t, 0, got.AISuggestedPriceCents)
		assert.Empty(t, got.AISuggestedAt)
	})

	t.Run("optimistic lock failure — price changed", func(t *testing.T) {
		p := newTestPurchase("pp-camp-4", "PP300002")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p.ID, 18000))

		// Try to accept with a different price (stale)
		err := repo.AcceptAISuggestion(ctx, p.ID, 19000)
		assert.ErrorIs(t, err, campaigns.ErrNoAISuggestion)

		// Original suggestion should remain untouched
		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 18000, got.AISuggestedPriceCents)
	})

	t.Run("no suggestion set", func(t *testing.T) {
		p := newTestPurchase("pp-camp-4", "PP300003")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.AcceptAISuggestion(ctx, p.ID, 10000)
		assert.ErrorIs(t, err, campaigns.ErrNoAISuggestion)
	})

	t.Run("zero price rejected", func(t *testing.T) {
		p := newTestPurchase("pp-camp-4", "PP300004")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p.ID, 5000))

		err := repo.AcceptAISuggestion(ctx, p.ID, 0)
		assert.ErrorIs(t, err, campaigns.ErrNoAISuggestion)
	})

	t.Run("negative price rejected", func(t *testing.T) {
		err := repo.AcceptAISuggestion(ctx, "pp-anything", -100)
		assert.ErrorIs(t, err, campaigns.ErrNoAISuggestion)
	})

	t.Run("nonexistent purchase", func(t *testing.T) {
		err := repo.AcceptAISuggestion(ctx, "pp-nonexistent", 5000)
		assert.ErrorIs(t, err, campaigns.ErrNoAISuggestion)
	})
}

// ---------- GetPriceOverrideStats ----------

func TestPurchasesPricing_GetPriceOverrideStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	t.Run("empty database", func(t *testing.T) {
		stats, err := repo.GetPriceOverrideStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, stats.TotalUnsold)
		assert.Equal(t, 0, stats.OverrideCount)
	})

	t.Run("aggregates correctly", func(t *testing.T) {
		createTestCampaign(t, db, "pp-camp-5", "Stats Campaign")

		// Purchase 1: manual override
		p1 := newTestPurchase("pp-camp-5", "PP400001")
		require.NoError(t, repo.CreatePurchase(ctx, p1))
		require.NoError(t, repo.UpdatePurchasePriceOverride(ctx, p1.ID, 10000, "manual"))

		// Purchase 2: cost_markup override + AI suggestion
		p2 := newTestPurchase("pp-camp-5", "PP400002")
		require.NoError(t, repo.CreatePurchase(ctx, p2))
		require.NoError(t, repo.UpdatePurchasePriceOverride(ctx, p2.ID, 20000, "cost_markup"))
		require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p2.ID, 22000))

		// Purchase 3: ai_accepted override
		p3 := newTestPurchase("pp-camp-5", "PP400003")
		require.NoError(t, repo.CreatePurchase(ctx, p3))
		require.NoError(t, repo.UpdatePurchasePriceOverride(ctx, p3.ID, 15000, "ai_accepted"))

		// Purchase 4: no override, only AI suggestion
		p4 := newTestPurchase("pp-camp-5", "PP400004")
		require.NoError(t, repo.CreatePurchase(ctx, p4))
		require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p4.ID, 8000))

		// Purchase 5: sold — should be excluded from stats
		p5 := newTestPurchase("pp-camp-5", "PP400005")
		require.NoError(t, repo.CreatePurchase(ctx, p5))
		require.NoError(t, repo.UpdatePurchasePriceOverride(ctx, p5.ID, 50000, "manual"))
		sale := newTestSale(p5.ID)
		require.NoError(t, repo.CreateSale(ctx, sale))

		stats, err := repo.GetPriceOverrideStats(ctx)
		require.NoError(t, err)

		assert.Equal(t, 4, stats.TotalUnsold)
		assert.Equal(t, 3, stats.OverrideCount)          // p1, p2, p3
		assert.Equal(t, 1, stats.ManualCount)            // p1
		assert.Equal(t, 1, stats.CostMarkupCount)        // p2
		assert.Equal(t, 1, stats.AIAcceptedCount)        // p3
		assert.Equal(t, 2, stats.PendingSuggestions)     // p2, p4
		assert.Equal(t, 45000, stats.OverrideTotalCents) // 10000+20000+15000
		assert.InDelta(t, 450.00, stats.OverrideTotalUsd, 0.01)
		assert.Equal(t, 30000, stats.SuggestionTotalCents) // 22000+8000
		assert.InDelta(t, 300.00, stats.SuggestionTotalUsd, 0.01)
	})

	t.Run("excludes closed campaigns", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		_, err := db.Exec(
			`INSERT INTO campaigns (id, name, phase, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"pp-camp-closed", "Closed Campaign", "closed", now, now,
		)
		require.NoError(t, err)

		p := newTestPurchase("pp-camp-closed", "PP400010")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.UpdatePurchasePriceOverride(ctx, p.ID, 99000, "manual"))

		stats, err := repo.GetPriceOverrideStats(ctx)
		require.NoError(t, err)

		// Closed campaign purchase should NOT be counted; counts should match the previous subtest.
		assert.Equal(t, 4, stats.TotalUnsold)
	})
}

// ---------- SetEbayExportFlag / ListEbayFlaggedPurchases / ClearEbayExportFlags ----------

func TestPurchasesPricing_EbayExportFlags(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pp-camp-6", "Ebay Export Campaign")

	t.Run("set flag and list", func(t *testing.T) {
		p := newTestPurchase("pp-camp-6", "PP500001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		flaggedAt := time.Now().Truncate(time.Second)
		err := repo.SetEbayExportFlag(ctx, p.ID, flaggedAt)
		require.NoError(t, err)

		flagged, err := repo.ListEbayFlaggedPurchases(ctx)
		require.NoError(t, err)

		var found bool
		for _, fp := range flagged {
			if fp.ID == p.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "flagged purchase should appear in list")
	})

	t.Run("set flag not found", func(t *testing.T) {
		err := repo.SetEbayExportFlag(ctx, "pp-nonexistent", time.Now())
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})

	t.Run("clear flags", func(t *testing.T) {
		p1 := newTestPurchase("pp-camp-6", "PP500002")
		p2 := newTestPurchase("pp-camp-6", "PP500003")
		require.NoError(t, repo.CreatePurchase(ctx, p1))
		require.NoError(t, repo.CreatePurchase(ctx, p2))

		flaggedAt := time.Now().Truncate(time.Second)
		require.NoError(t, repo.SetEbayExportFlag(ctx, p1.ID, flaggedAt))
		require.NoError(t, repo.SetEbayExportFlag(ctx, p2.ID, flaggedAt))

		err := repo.ClearEbayExportFlags(ctx, []string{p1.ID, p2.ID})
		require.NoError(t, err)

		flagged, err := repo.ListEbayFlaggedPurchases(ctx)
		require.NoError(t, err)
		for _, fp := range flagged {
			assert.NotEqual(t, p1.ID, fp.ID, "cleared purchase should not appear")
			assert.NotEqual(t, p2.ID, fp.ID, "cleared purchase should not appear")
		}
	})

	t.Run("clear empty list is no-op", func(t *testing.T) {
		err := repo.ClearEbayExportFlags(ctx, []string{})
		require.NoError(t, err)
	})

	t.Run("clear nonexistent IDs is silent", func(t *testing.T) {
		err := repo.ClearEbayExportFlags(ctx, []string{"pp-nope-1", "pp-nope-2"})
		require.NoError(t, err)
	})

	t.Run("list excludes sold purchases", func(t *testing.T) {
		p := newTestPurchase("pp-camp-6", "PP500004")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.SetEbayExportFlag(ctx, p.ID, time.Now().Truncate(time.Second)))

		sale := newTestSale(p.ID)
		require.NoError(t, repo.CreateSale(ctx, sale))

		flagged, err := repo.ListEbayFlaggedPurchases(ctx)
		require.NoError(t, err)
		for _, fp := range flagged {
			assert.NotEqual(t, p.ID, fp.ID, "sold purchase should not appear in flagged list")
		}
	})

	t.Run("list excludes non-PSA grader", func(t *testing.T) {
		p := newTestPurchase("pp-camp-6", "PP500005")
		p.Grader = "BGS"
		require.NoError(t, repo.CreatePurchase(ctx, p))
		require.NoError(t, repo.SetEbayExportFlag(ctx, p.ID, time.Now().Truncate(time.Second)))

		flagged, err := repo.ListEbayFlaggedPurchases(ctx)
		require.NoError(t, err)
		for _, fp := range flagged {
			assert.NotEqual(t, p.ID, fp.ID, "non-PSA purchase should not appear in flagged list")
		}
	})

	t.Run("clear large batch is chunked", func(t *testing.T) {
		// Create >500 purchases to exercise the chunking path
		ids := make([]string, 510)
		for i := range ids {
			cert := fmt.Sprintf("PP6%05d", i)
			p := newTestPurchase("pp-camp-6", cert)
			require.NoError(t, repo.CreatePurchase(ctx, p))
			ids[i] = p.ID
			require.NoError(t, repo.SetEbayExportFlag(ctx, p.ID, time.Now().Truncate(time.Second)))
		}

		err := repo.ClearEbayExportFlags(ctx, ids)
		require.NoError(t, err)

		// Spot-check that flags are cleared
		flagged, err := repo.ListEbayFlaggedPurchases(ctx)
		require.NoError(t, err)
		clearedIDs := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			clearedIDs[id] = struct{}{}
		}
		for _, fp := range flagged {
			_, wasCleaned := clearedIDs[fp.ID]
			assert.False(t, wasCleaned, "purchase %s should have been cleared", fp.ID)
		}
	})
}
