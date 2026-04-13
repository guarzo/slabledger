package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertTestUser inserts a minimal user row and returns the row ID.
func insertTestUser(t *testing.T, db *DB) int64 {
	t.Helper()
	result, err := db.Exec(
		`INSERT INTO users (google_id, email, is_admin) VALUES (?, ?, ?)`,
		"google-test-001", "test@example.com", false,
	)
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	return id
}

// newPricingTestDB sets up a test DB and repo backed by the same database.
func newPricingTestDB(t *testing.T) (*DB, *testCampaignsRepository) {
	t.Helper()
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	repo := &testCampaignsRepository{
		CampaignStore:  NewCampaignStore(db.DB, logger),
		PurchaseStore:  NewPurchaseStore(db.DB, logger),
		SaleStore:      NewSaleStore(db.DB, logger),
		AnalyticsStore: NewAnalyticsStore(db.DB, logger),
		FinanceStore:   NewFinanceStore(db.DB, logger),
		PricingStore:   NewPricingStore(db.DB, logger),
		DHStore:        NewDHStore(db.DB, logger),
		SnapshotStore:  NewSnapshotStore(db.DB, logger),
		SellSheetStore: NewSellSheetStore(db.DB, logger),
	}
	return db, repo
}

/* ── UpdateReviewedPrice ──────────────────────────────────────────── */

func TestUpdateReviewedPrice(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		priceCents  int
		source      string
		wantErr     bool
		wantErrType error
		// expected DB state after the call
		wantPriceCents int
		wantSource     string
		wantReviewedAt bool // true → reviewed_at should be non-empty
	}{
		{
			name:           "sets positive price and records reviewed_at",
			priceCents:     95000,
			source:         "manual",
			wantPriceCents: 95000,
			wantSource:     "manual",
			wantReviewedAt: true,
		},
		{
			name:           "zero price clears reviewed_at and source",
			priceCents:     0,
			source:         "manual",
			wantPriceCents: 0,
			wantSource:     "",
			wantReviewedAt: false,
		},
		{
			name:           "negative price treated as zero — clears reviewed_at",
			priceCents:     -1,
			source:         "cl",
			wantPriceCents: 0,
			wantSource:     "",
			wantReviewedAt: false,
		},
		{
			name:        "unknown purchase ID returns ErrPurchaseNotFound",
			priceCents:  50000,
			source:      "manual",
			wantErr:     true,
			wantErrType: inventory.ErrPurchaseNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, repo := newPricingTestDB(t)
			defer db.Close()

			var purchaseID string
			if tt.wantErrType == nil {
				// Create a real purchase to update
				createTestCampaign(t, db, "camp-rev", "Review Test")
				p := newTestPurchase("camp-rev", "REV001")
				require.NoError(t, repo.CreatePurchase(ctx, p))
				purchaseID = p.ID
			} else {
				purchaseID = "nonexistent-purchase"
			}

			err := repo.UpdateReviewedPrice(ctx, purchaseID, tt.priceCents, tt.source)

			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrType)
				return
			}
			require.NoError(t, err)

			var reviewedPriceCents int
			var reviewedAt, reviewSource string
			require.NoError(t, db.QueryRowContext(ctx,
				`SELECT reviewed_price_cents, reviewed_at, review_source FROM campaign_purchases WHERE id = ?`,
				purchaseID,
			).Scan(&reviewedPriceCents, &reviewedAt, &reviewSource))

			assert.Equal(t, tt.wantPriceCents, reviewedPriceCents)
			assert.Equal(t, tt.wantSource, reviewSource)
			if tt.wantReviewedAt {
				assert.NotEmpty(t, reviewedAt, "reviewed_at should be set")
			} else {
				assert.Empty(t, reviewedAt, "reviewed_at should be cleared")
			}
		})
	}
}

/* ── GetReviewStats ───────────────────────────────────────────────── */

func TestGetReviewStats(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	t.Run("empty campaign returns all zeros", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		createTestCampaign(t, db, "camp-stats", "Stats Test")

		stats, err := repo.GetReviewStats(ctx, "camp-stats")
		require.NoError(t, err)
		assert.Equal(t, 0, stats.Total)
		assert.Equal(t, 0, stats.NeedsReview)
		assert.Equal(t, 0, stats.Reviewed)
		assert.Equal(t, 0, stats.Flagged)
	})

	t.Run("counts unsold purchases correctly", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		createTestCampaign(t, db, "camp-rev2", "Review Stats")

		// p1: not reviewed (no reviewed_at)
		p1 := &inventory.Purchase{
			ID: "rs-p1", CampaignID: "camp-rev2", CardName: "Charizard",
			CertNumber: "RS001", GradeValue: 9, BuyCostCents: 80000,
			PurchaseDate: "2026-01-15", CreatedAt: now, UpdatedAt: now,
		}
		// p2: reviewed
		p2 := &inventory.Purchase{
			ID: "rs-p2", CampaignID: "camp-rev2", CardName: "Pikachu",
			CertNumber: "RS002", GradeValue: 10, BuyCostCents: 50000,
			PurchaseDate: "2026-01-15", CreatedAt: now, UpdatedAt: now,
		}
		// p3: sold — should not appear in stats
		p3 := &inventory.Purchase{
			ID: "rs-p3", CampaignID: "camp-rev2", CardName: "Blastoise",
			CertNumber: "RS003", GradeValue: 9, BuyCostCents: 60000,
			PurchaseDate: "2026-01-15", CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p1))
		require.NoError(t, repo.CreatePurchase(ctx, p2))
		require.NoError(t, repo.CreatePurchase(ctx, p3))

		// Mark p2 as reviewed
		require.NoError(t, repo.UpdateReviewedPrice(ctx, p2.ID, 55000, "manual"))

		// Sell p3
		require.NoError(t, repo.CreateSale(ctx, newTestSale(p3.ID)))

		stats, err := repo.GetReviewStats(ctx, "camp-rev2")
		require.NoError(t, err)
		assert.Equal(t, 2, stats.Total, "only unsold purchases count")
		assert.Equal(t, 1, stats.NeedsReview)
		assert.Equal(t, 1, stats.Reviewed)
		assert.Equal(t, 0, stats.Flagged)
	})
}

/* ── GetGlobalReviewStats ─────────────────────────────────────────── */

func TestGetGlobalReviewStats(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	t.Run("empty DB returns zeros", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()

		stats, err := repo.GetGlobalReviewStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, stats.Total)
		assert.Equal(t, 0, stats.NeedsReview)
		assert.Equal(t, 0, stats.Reviewed)
	})

	t.Run("aggregates across campaigns", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		createTestCampaign(t, db, "camp-g1", "Global 1")
		createTestCampaign(t, db, "camp-g2", "Global 2")

		purchases := []*inventory.Purchase{
			{ID: "g-p1", CampaignID: "camp-g1", CardName: "A", CertNumber: "G001", GradeValue: 9, BuyCostCents: 10000, PurchaseDate: "2026-01-01", CreatedAt: now, UpdatedAt: now},
			{ID: "g-p2", CampaignID: "camp-g1", CardName: "B", CertNumber: "G002", GradeValue: 9, BuyCostCents: 10000, PurchaseDate: "2026-01-01", CreatedAt: now, UpdatedAt: now},
			{ID: "g-p3", CampaignID: "camp-g2", CardName: "C", CertNumber: "G003", GradeValue: 9, BuyCostCents: 10000, PurchaseDate: "2026-01-01", CreatedAt: now, UpdatedAt: now},
		}
		for _, p := range purchases {
			require.NoError(t, repo.CreatePurchase(ctx, p))
		}
		// Review one
		require.NoError(t, repo.UpdateReviewedPrice(ctx, "g-p1", 12000, "manual"))

		stats, err := repo.GetGlobalReviewStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, stats.Total)
		assert.Equal(t, 2, stats.NeedsReview)
		assert.Equal(t, 1, stats.Reviewed)
	})
}

/* ── CreatePriceFlag / HasOpenFlag / ResolvePriceFlag ─────────────── */

func TestPriceFlag_CreateAndHasOpen(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("newly created flag is open", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		createTestCampaign(t, db, "camp-flag", "Flag Test")
		p := newTestPurchase("camp-flag", "FL001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		userID := insertTestUser(t, db)

		flag := &inventory.PriceFlag{
			PurchaseID: p.ID,
			FlaggedBy:  userID,
			FlaggedAt:  now,
			Reason:     inventory.PriceFlagWrongMatch,
		}
		id, err := repo.CreatePriceFlag(ctx, flag)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		open, err := repo.HasOpenFlag(ctx, p.ID)
		require.NoError(t, err)
		assert.True(t, open, "newly created flag should be open")
	})

	t.Run("no flag returns false", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()

		open, err := repo.HasOpenFlag(ctx, "nonexistent-purchase")
		require.NoError(t, err)
		assert.False(t, open)
		_ = db
	})
}

func TestPriceFlag_Resolve(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("resolving flag closes it", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		createTestCampaign(t, db, "camp-res", "Resolve Test")
		p := newTestPurchase("camp-res", "RES001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		userID := insertTestUser(t, db)
		flag := &inventory.PriceFlag{
			PurchaseID: p.ID,
			FlaggedBy:  userID,
			FlaggedAt:  now,
			Reason:     inventory.PriceFlagStaleData,
		}
		flagID, err := repo.CreatePriceFlag(ctx, flag)
		require.NoError(t, err)

		err = repo.ResolvePriceFlag(ctx, flagID, userID)
		require.NoError(t, err)

		open, err := repo.HasOpenFlag(ctx, p.ID)
		require.NoError(t, err)
		assert.False(t, open, "resolved flag should not be open")
	})

	t.Run("resolving nonexistent flag returns ErrPriceFlagNotFound", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		_ = db

		err := repo.ResolvePriceFlag(ctx, 9999, 1)
		require.ErrorIs(t, err, inventory.ErrPriceFlagNotFound)
	})

	t.Run("resolving already-resolved flag returns ErrPriceFlagNotFound", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		createTestCampaign(t, db, "camp-dblres", "Double Resolve")
		p := newTestPurchase("camp-dblres", "DR001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		userID := insertTestUser(t, db)
		flag := &inventory.PriceFlag{
			PurchaseID: p.ID,
			FlaggedBy:  userID,
			FlaggedAt:  now,
			Reason:     inventory.PriceFlagOther,
		}
		flagID, err := repo.CreatePriceFlag(ctx, flag)
		require.NoError(t, err)

		require.NoError(t, repo.ResolvePriceFlag(ctx, flagID, userID))
		err = repo.ResolvePriceFlag(ctx, flagID, userID)
		require.ErrorIs(t, err, inventory.ErrPriceFlagNotFound)
	})
}

/* ── OpenFlagPurchaseIDs ──────────────────────────────────────────── */

func TestOpenFlagPurchaseIDs(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("no open flags returns empty map", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		_ = db

		result, err := repo.OpenFlagPurchaseIDs(ctx)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns purchase ID to min flag ID mapping", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		createTestCampaign(t, db, "camp-open", "Open Flags")

		p1 := newTestPurchase("camp-open", "OF001")
		p2 := newTestPurchase("camp-open", "OF002")
		require.NoError(t, repo.CreatePurchase(ctx, p1))
		require.NoError(t, repo.CreatePurchase(ctx, p2))

		userID := insertTestUser(t, db)

		id1, err := repo.CreatePriceFlag(ctx, &inventory.PriceFlag{
			PurchaseID: p1.ID, FlaggedBy: userID, FlaggedAt: now, Reason: inventory.PriceFlagWrongMatch,
		})
		require.NoError(t, err)

		_, err = repo.CreatePriceFlag(ctx, &inventory.PriceFlag{
			PurchaseID: p2.ID, FlaggedBy: userID, FlaggedAt: now, Reason: inventory.PriceFlagStaleData,
		})
		require.NoError(t, err)

		result, err := repo.OpenFlagPurchaseIDs(ctx)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, id1, result[p1.ID])
	})

	t.Run("resolved flags are excluded", func(t *testing.T) {
		db, repo := newPricingTestDB(t)
		defer db.Close()
		createTestCampaign(t, db, "camp-excl", "Exclude Resolved")

		p := newTestPurchase("camp-excl", "EX001")
		require.NoError(t, repo.CreatePurchase(ctx, p))
		userID := insertTestUser(t, db)

		flagID, err := repo.CreatePriceFlag(ctx, &inventory.PriceFlag{
			PurchaseID: p.ID, FlaggedBy: userID, FlaggedAt: now, Reason: inventory.PriceFlagOther,
		})
		require.NoError(t, err)
		require.NoError(t, repo.ResolvePriceFlag(ctx, flagID, userID))

		result, err := repo.OpenFlagPurchaseIDs(ctx)
		require.NoError(t, err)
		assert.Empty(t, result, "resolved flag should not appear")
	})
}

/* ── GetReviewStats with flags ───────────────────────────────────── */

func TestGetReviewStats_Flagged(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	db, repo := newPricingTestDB(t)
	defer db.Close()
	createTestCampaign(t, db, "camp-flagstats", "Flagged Stats")

	p1 := newTestPurchase("camp-flagstats", "FS001")
	p2 := newTestPurchase("camp-flagstats", "FS002")
	require.NoError(t, repo.CreatePurchase(ctx, p1))
	require.NoError(t, repo.CreatePurchase(ctx, p2))

	userID := insertTestUser(t, db)
	_, err := repo.CreatePriceFlag(ctx, &inventory.PriceFlag{
		PurchaseID: p1.ID, FlaggedBy: userID, FlaggedAt: now, Reason: inventory.PriceFlagWrongGrade,
	})
	require.NoError(t, err)

	stats, err := repo.GetReviewStats(ctx, "camp-flagstats")
	require.NoError(t, err)
	assert.Equal(t, 2, stats.Total)
	assert.Equal(t, 1, stats.Flagged, "one purchase should be flagged")

	// Global stats should also reflect the flag
	globalStats, err := repo.GetGlobalReviewStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, globalStats.Flagged)
}
