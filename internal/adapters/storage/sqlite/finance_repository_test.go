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

func TestGetCashflowConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("returns migration-seeded defaults", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)

		cfg, err := repo.GetCashflowConfig(ctx)
		require.NoError(t, err)
		// Migration 000001 seeds: credit_limit_cents=5000000, cash_buffer_cents=1000000
		assert.Equal(t, 5000000, cfg.CapitalBudgetCents)
		assert.Equal(t, 1000000, cfg.CashBufferCents)
	})

	t.Run("returns stored config", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)
		now := time.Now().Truncate(time.Second)

		err := repo.UpdateCashflowConfig(ctx, &campaigns.CashflowConfig{
			CapitalBudgetCents: 5000000,
			CashBufferCents:    2000000,
			UpdatedAt:          now,
		})
		require.NoError(t, err)

		cfg, err := repo.GetCashflowConfig(ctx)
		require.NoError(t, err)
		assert.Equal(t, 5000000, cfg.CapitalBudgetCents)
		assert.Equal(t, 2000000, cfg.CashBufferCents)
	})
}

func TestUpdateCashflowConfig(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	t.Run("insert and update round-trip", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)

		// First insert (upsert)
		err := repo.UpdateCashflowConfig(ctx, &campaigns.CashflowConfig{
			CapitalBudgetCents: 3000000,
			CashBufferCents:    500000,
			UpdatedAt:          now,
		})
		require.NoError(t, err)

		cfg, err := repo.GetCashflowConfig(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3000000, cfg.CapitalBudgetCents)
		assert.Equal(t, 500000, cfg.CashBufferCents)

		// Second update (upsert overwrites)
		later := now.Add(time.Hour)
		err = repo.UpdateCashflowConfig(ctx, &campaigns.CashflowConfig{
			CapitalBudgetCents: 8000000,
			CashBufferCents:    1500000,
			UpdatedAt:          later,
		})
		require.NoError(t, err)

		cfg, err = repo.GetCashflowConfig(ctx)
		require.NoError(t, err)
		assert.Equal(t, 8000000, cfg.CapitalBudgetCents)
		assert.Equal(t, 1500000, cfg.CashBufferCents)
	})
}

func TestSumPurchaseCostByInvoiceDate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	createTestCampaign(t, db, "camp-inv", "Invoice Test")

	t.Run("no purchases returns zero", func(t *testing.T) {
		total, err := repo.SumPurchaseCostByInvoiceDate(ctx, "2026-01-15")
		require.NoError(t, err)
		assert.Equal(t, 0, total)
	})

	t.Run("sums non-refunded purchases for matching date", func(t *testing.T) {
		// Two purchases on same invoice date
		p1 := &campaigns.Purchase{
			ID: "inv-p1", CampaignID: "camp-inv", CardName: "Charizard", CertNumber: "INV001",
			GradeValue: 9, BuyCostCents: 50000, PSASourcingFeeCents: 300,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-15",
			CreatedAt: now, UpdatedAt: now,
		}
		p2 := &campaigns.Purchase{
			ID: "inv-p2", CampaignID: "camp-inv", CardName: "Pikachu", CertNumber: "INV002",
			GradeValue: 10, BuyCostCents: 30000, PSASourcingFeeCents: 300,
			PurchaseDate: "2026-01-12", InvoiceDate: "2026-01-15",
			CreatedAt: now, UpdatedAt: now,
		}
		// Purchase on different invoice date
		p3 := &campaigns.Purchase{
			ID: "inv-p3", CampaignID: "camp-inv", CardName: "Blastoise", CertNumber: "INV003",
			GradeValue: 9, BuyCostCents: 40000, PSASourcingFeeCents: 300,
			PurchaseDate: "2026-01-20", InvoiceDate: "2026-02-01",
			CreatedAt: now, UpdatedAt: now,
		}
		// Refunded purchase on same invoice date — should be excluded
		p4 := &campaigns.Purchase{
			ID: "inv-p4", CampaignID: "camp-inv", CardName: "Venusaur", CertNumber: "INV004",
			GradeValue: 8, BuyCostCents: 25000, PSASourcingFeeCents: 300,
			PurchaseDate: "2026-01-14", InvoiceDate: "2026-01-15",
			WasRefunded: true,
			CreatedAt:   now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p1))
		require.NoError(t, repo.CreatePurchase(ctx, p2))
		require.NoError(t, repo.CreatePurchase(ctx, p3))
		require.NoError(t, repo.CreatePurchase(ctx, p4))

		// Sum for 2026-01-15: p1 (50000+300) + p2 (30000+300) = 80600
		total, err := repo.SumPurchaseCostByInvoiceDate(ctx, "2026-01-15")
		require.NoError(t, err)
		assert.Equal(t, 80600, total)

		// Different date: p3 (40000+300) = 40300
		total, err = repo.SumPurchaseCostByInvoiceDate(ctx, "2026-02-01")
		require.NoError(t, err)
		assert.Equal(t, 40300, total)

		// No match
		total, err = repo.SumPurchaseCostByInvoiceDate(ctx, "2099-01-01")
		require.NoError(t, err)
		assert.Equal(t, 0, total)
	})
}

func TestGetCapitalSummary_RecoveryVelocity(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	t.Run("no sales returns 99 weeks and stable trend", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)

		createTestCampaign(t, db, "camp-nosale", "No Sales")
		p := &campaigns.Purchase{
			ID: "ns-p1", CampaignID: "camp-nosale", CardName: "Charizard", CertNumber: "NS001",
			GradeValue: 9, BuyCostCents: 50000, PSASourcingFeeCents: 0,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p))

		summary, err := repo.GetCapitalSummary(ctx)
		require.NoError(t, err)
		assert.Equal(t, 50000, summary.OutstandingCents)
		assert.Equal(t, 0, summary.RecoveryRate30dCents)
		assert.Equal(t, 0, summary.RecoveryRate30dPriorCents)
		assert.Equal(t, 99.0, summary.WeeksToCover)
		assert.Equal(t, "stable", summary.RecoveryTrend)
	})

	t.Run("recent sales compute recovery rate and weeks to cover", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)

		createTestCampaign(t, db, "camp-sell", "Selling")
		for i, cert := range []string{"SELL001", "SELL002"} {
			p := &campaigns.Purchase{
				ID: fmt.Sprintf("sell-p%d", i), CampaignID: "camp-sell", CardName: "Pikachu",
				CertNumber: cert, GradeValue: 10, BuyCostCents: 50000, PSASourcingFeeCents: 0,
				PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
				CreatedAt: now, UpdatedAt: now,
			}
			require.NoError(t, repo.CreatePurchase(ctx, p))
		}

		recentDate := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
		s := &campaigns.Sale{
			ID: "sell-s1", PurchaseID: "sell-p0", SaleChannel: campaigns.SaleChannelEbay,
			SalePriceCents: 80000, SaleFeeCents: 9880, SaleDate: recentDate,
			DaysToSell: 10, NetProfitCents: 20120, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateSale(ctx, s))

		summary, err := repo.GetCapitalSummary(ctx)
		require.NoError(t, err)
		assert.Equal(t, 80000, summary.RecoveryRate30dCents)
		assert.Equal(t, 0, summary.RecoveryRate30dPriorCents)
		assert.Greater(t, summary.WeeksToCover, 4.0)
		assert.Less(t, summary.WeeksToCover, 7.0)
	})

	t.Run("improving trend when 30d exceeds prior 30d by more than 10pct", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)

		createTestCampaign(t, db, "camp-trend", "Trending")
		p := &campaigns.Purchase{
			ID: "trend-p1", CampaignID: "camp-trend", CardName: "Mew",
			CertNumber: "TR001", GradeValue: 10, BuyCostCents: 50000, PSASourcingFeeCents: 0,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p))

		recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
		s1 := &campaigns.Sale{
			ID: "trend-s1", PurchaseID: "trend-p1", SaleChannel: campaigns.SaleChannelEbay,
			SalePriceCents: 50000, SaleFeeCents: 6175, SaleDate: recentDate,
			DaysToSell: 5, NetProfitCents: 0, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateSale(ctx, s1))

		p2 := &campaigns.Purchase{
			ID: "trend-p2", CampaignID: "camp-trend", CardName: "Mewtwo",
			CertNumber: "TR002", GradeValue: 10, BuyCostCents: 50000, PSASourcingFeeCents: 0,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p2))

		priorDate := time.Now().AddDate(0, 0, -45).Format("2006-01-02")
		s2 := &campaigns.Sale{
			ID: "trend-s2", PurchaseID: "trend-p2", SaleChannel: campaigns.SaleChannelEbay,
			SalePriceCents: 20000, SaleFeeCents: 2470, SaleDate: priorDate,
			DaysToSell: 5, NetProfitCents: 0, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateSale(ctx, s2))

		summary, err := repo.GetCapitalSummary(ctx)
		require.NoError(t, err)
		assert.Equal(t, 50000, summary.RecoveryRate30dCents)
		assert.Equal(t, 20000, summary.RecoveryRate30dPriorCents)
		assert.Equal(t, "improving", summary.RecoveryTrend)
	})

	t.Run("alert levels based on weeks to cover", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)

		createTestCampaign(t, db, "camp-alert", "Alert")

		p := &campaigns.Purchase{
			ID: "alert-p1", CampaignID: "camp-alert", CardName: "Lugia",
			CertNumber: "AL001", GradeValue: 10, BuyCostCents: 5000000, PSASourcingFeeCents: 0,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p))

		p2 := &campaigns.Purchase{
			ID: "alert-p2", CampaignID: "camp-alert", CardName: "Ho-Oh",
			CertNumber: "AL002", GradeValue: 10, BuyCostCents: 10000, PSASourcingFeeCents: 0,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p2))

		recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
		s := &campaigns.Sale{
			ID: "alert-s1", PurchaseID: "alert-p2", SaleChannel: campaigns.SaleChannelEbay,
			SalePriceCents: 100000, SaleFeeCents: 12350, SaleDate: recentDate,
			DaysToSell: 5, NetProfitCents: 0, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateSale(ctx, s))

		summary, err := repo.GetCapitalSummary(ctx)
		require.NoError(t, err)
		assert.Greater(t, summary.WeeksToCover, 12.0)
		assert.Equal(t, "critical", summary.AlertLevel)
	})

	t.Run("fallback alert when no recovery and high outstanding", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)

		createTestCampaign(t, db, "camp-fb", "Fallback")
		p := &campaigns.Purchase{
			ID: "fb-p1", CampaignID: "camp-fb", CardName: "Rayquaza",
			CertNumber: "FB001", GradeValue: 10, BuyCostCents: 1100000, PSASourcingFeeCents: 0,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p))

		summary, err := repo.GetCapitalSummary(ctx)
		require.NoError(t, err)
		assert.Equal(t, "critical", summary.AlertLevel)
	})
}

func TestGetCapitalSummary_EmptyState(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	summary, err := repo.GetCapitalSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.OutstandingCents)
	assert.Equal(t, 0, summary.RecoveryRate30dCents)
	assert.Equal(t, 0, summary.RecoveryRate30dPriorCents)
	assert.Equal(t, 99.0, summary.WeeksToCover)
	assert.Equal(t, "stable", summary.RecoveryTrend)
	assert.Equal(t, "ok", summary.AlertLevel)
	assert.Equal(t, 0, summary.RefundedCents)
	assert.Equal(t, 0, summary.PaidCents)
	assert.Equal(t, 0, summary.UnpaidInvoiceCount)
}
