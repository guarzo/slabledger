package sqlite

import (
	"context"
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

func TestGetCapitalSummary_AlertLevels(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	tests := []struct {
		name           string
		budgetCents    int
		purchaseCents  int // buy_cost_cents (sourcing fee will be added)
		wantAlertLevel string
	}{
		{
			name:           "ok when exposure under 80%",
			budgetCents:    1000000,
			purchaseCents:  500000,
			wantAlertLevel: "ok",
		},
		{
			name:           "warning when exposure >= 80%",
			budgetCents:    1000000,
			purchaseCents:  800000,
			wantAlertLevel: "warning",
		},
		{
			name:           "critical when exposure >= 90%",
			budgetCents:    1000000,
			purchaseCents:  900000,
			wantAlertLevel: "critical",
		},
		{
			name:           "ok when no budget set",
			budgetCents:    0,
			purchaseCents:  500000,
			wantAlertLevel: "ok",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fresh DB per subtest to avoid data bleed
			subDB := setupTestDB(t)
			defer subDB.Close()
			subRepo := NewCampaignsRepository(subDB.DB)

			// Set budget
			err := subRepo.UpdateCashflowConfig(ctx, &campaigns.CashflowConfig{
				CapitalBudgetCents: tt.budgetCents,
				CashBufferCents:    100000,
				UpdatedAt:          now,
			})
			require.NoError(t, err)

			// Create campaign + purchase with invoice date
			campID := "camp-alert-" + time.Now().Format("150405") + string(rune('a'+i))
			c := &campaigns.Campaign{ID: campID, Name: "Alert " + tt.name, Phase: campaigns.PhaseActive, CreatedAt: now, UpdatedAt: now}
			require.NoError(t, subRepo.CreateCampaign(ctx, c))

			purchaseDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
			p := &campaigns.Purchase{
				ID: "alert-p", CampaignID: campID, CardName: "Charizard",
				CertNumber: "ALERT001", GradeValue: 9,
				BuyCostCents: tt.purchaseCents, PSASourcingFeeCents: 0,
				PurchaseDate: purchaseDate, InvoiceDate: purchaseDate,
				CreatedAt: now, UpdatedAt: now,
			}
			require.NoError(t, subRepo.CreatePurchase(ctx, p))

			summary, err := subRepo.GetCapitalSummary(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAlertLevel, summary.AlertLevel)
		})
	}
}

func TestGetCapitalSummary_EmptyState(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	summary, err := repo.GetCapitalSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.OutstandingCents)
	assert.Equal(t, 0, summary.ProjectedExposureCents)
	assert.Equal(t, 0, summary.RefundedCents)
	assert.Equal(t, 0, summary.PaidCents)
	assert.Equal(t, 0, summary.UnpaidInvoiceCount)
	assert.Equal(t, "ok", summary.AlertLevel)
	assert.Equal(t, 30, summary.DaysToNextInvoice)
}
