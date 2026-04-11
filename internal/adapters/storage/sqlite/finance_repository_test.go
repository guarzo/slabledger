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

func TestGetCapitalRawData_RecoveryVelocity(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	t.Run("no sales returns raw zero recovery data", func(t *testing.T) {
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

		raw, err := repo.GetCapitalRawData(ctx)
		require.NoError(t, err)
		assert.Equal(t, 50000, raw.OutstandingCents)
		assert.Equal(t, 0, raw.RecoveryRate30dCents)
		assert.Equal(t, 0, raw.RecoveryRate30dPriorCents)

		// Verify derived fields via domain logic
		summary := campaigns.ComputeCapitalSummary(raw)
		assert.Equal(t, 99.0, summary.WeeksToCover)
		assert.Equal(t, campaigns.TrendStable, summary.RecoveryTrend)
	})

	t.Run("recent sales compute recovery rate", func(t *testing.T) {
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

		raw, err := repo.GetCapitalRawData(ctx)
		require.NoError(t, err)
		assert.Equal(t, 80000, raw.RecoveryRate30dCents)
		assert.Equal(t, 0, raw.RecoveryRate30dPriorCents)

		// Verify derived fields via domain logic
		summary := campaigns.ComputeCapitalSummary(raw)
		assert.Greater(t, summary.WeeksToCover, 4.0)
		assert.Less(t, summary.WeeksToCover, 7.0)
		assert.Equal(t, campaigns.TrendImproving, summary.RecoveryTrend,
			"recovery from zero prior should be improving")
	})

	t.Run("raw data has both 30d and prior 30d recovery", func(t *testing.T) {
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

		raw, err := repo.GetCapitalRawData(ctx)
		require.NoError(t, err)
		assert.Equal(t, 50000, raw.RecoveryRate30dCents)
		assert.Equal(t, 20000, raw.RecoveryRate30dPriorCents)

		// Verify derived fields via domain logic
		summary := campaigns.ComputeCapitalSummary(raw)
		assert.Equal(t, campaigns.TrendImproving, summary.RecoveryTrend)
	})

	t.Run("declining recovery data from repo", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)

		createTestCampaign(t, db, "camp-decline", "Declining")
		for i, cert := range []string{"DEC001", "DEC002"} {
			p := &campaigns.Purchase{
				ID: fmt.Sprintf("dec-p%d", i), CampaignID: "camp-decline", CardName: "Slowpoke",
				CertNumber: cert, GradeValue: 8, BuyCostCents: 30000, PSASourcingFeeCents: 0,
				PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
				CreatedAt: now, UpdatedAt: now,
			}
			require.NoError(t, repo.CreatePurchase(ctx, p))
		}

		recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
		s1 := &campaigns.Sale{
			ID: "dec-s1", PurchaseID: "dec-p0", SaleChannel: campaigns.SaleChannelEbay,
			SalePriceCents: 10000, SaleFeeCents: 1235, SaleDate: recentDate,
			DaysToSell: 5, NetProfitCents: 0, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateSale(ctx, s1))

		priorDate := time.Now().AddDate(0, 0, -45).Format("2006-01-02")
		s2 := &campaigns.Sale{
			ID: "dec-s2", PurchaseID: "dec-p1", SaleChannel: campaigns.SaleChannelEbay,
			SalePriceCents: 20000, SaleFeeCents: 2470, SaleDate: priorDate,
			DaysToSell: 5, NetProfitCents: 0, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateSale(ctx, s2))

		raw, err := repo.GetCapitalRawData(ctx)
		require.NoError(t, err)
		assert.Equal(t, 10000, raw.RecoveryRate30dCents)
		assert.Equal(t, 20000, raw.RecoveryRate30dPriorCents)

		// Verify derived fields via domain logic
		summary := campaigns.ComputeCapitalSummary(raw)
		assert.Equal(t, campaigns.TrendDeclining, summary.RecoveryTrend)
	})

	t.Run("warning-level raw data from repo", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		repo := NewCampaignsRepository(db.DB)

		createTestCampaign(t, db, "camp-warning", "Warning")
		// Outstanding ~200000, recovery ~100000/30d → weekly ~23256 → weeks ~8.6
		p := &campaigns.Purchase{
			ID: "warn-p1", CampaignID: "camp-warning", CardName: "Snorlax",
			CertNumber: "WRN001", GradeValue: 9, BuyCostCents: 200000, PSASourcingFeeCents: 0,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p))

		recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
		s := &campaigns.Sale{
			ID: "warn-s1", PurchaseID: "warn-p1", SaleChannel: campaigns.SaleChannelEbay,
			SalePriceCents: 100000, SaleFeeCents: 12350, SaleDate: recentDate,
			DaysToSell: 5, NetProfitCents: 0, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateSale(ctx, s))

		raw, err := repo.GetCapitalRawData(ctx)
		require.NoError(t, err)

		// Verify derived fields via domain logic
		summary := campaigns.ComputeCapitalSummary(raw)
		assert.GreaterOrEqual(t, summary.WeeksToCover, 6.0)
		assert.Less(t, summary.WeeksToCover, 12.0)
		assert.Equal(t, campaigns.AlertWarning, summary.AlertLevel)
	})

	t.Run("critical-level raw data from repo", func(t *testing.T) {
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

		raw, err := repo.GetCapitalRawData(ctx)
		require.NoError(t, err)

		// Verify derived fields via domain logic
		summary := campaigns.ComputeCapitalSummary(raw)
		assert.Greater(t, summary.WeeksToCover, 12.0)
		assert.Equal(t, campaigns.AlertCritical, summary.AlertLevel)
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

		raw, err := repo.GetCapitalRawData(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1100000, raw.OutstandingCents)
		assert.Equal(t, 0, raw.RecoveryRate30dCents)

		// Verify derived fields via domain logic
		summary := campaigns.ComputeCapitalSummary(raw)
		assert.Equal(t, campaigns.AlertCritical, summary.AlertLevel)
	})
}

func TestGetCapitalRawData_EmptyState(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	raw, err := repo.GetCapitalRawData(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, raw.OutstandingCents)
	assert.Equal(t, 0, raw.RecoveryRate30dCents)
	assert.Equal(t, 0, raw.RecoveryRate30dPriorCents)
	assert.Equal(t, 0, raw.RefundedCents)
	assert.Equal(t, 0, raw.PaidCents)
	assert.Equal(t, 0, raw.UnpaidInvoiceCount)

	// Verify derived fields via domain logic
	summary := campaigns.ComputeCapitalSummary(raw)
	assert.Equal(t, 99.0, summary.WeeksToCover)
	assert.Equal(t, campaigns.TrendStable, summary.RecoveryTrend)
	assert.Equal(t, campaigns.AlertOK, summary.AlertLevel)
}

func TestGetPendingReceiptByInvoiceDate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setup       func(t *testing.T, db *DB, repo *CampaignsRepository)
		inputDates  []string
		wantPending map[string]int
	}{
		{
			name:        "empty input returns empty map",
			setup:       func(t *testing.T, db *DB, repo *CampaignsRepository) {},
			inputDates:  []string{},
			wantPending: map[string]int{},
		},
		{
			name: "date with no purchases returns zero (absent from map)",
			setup: func(t *testing.T, db *DB, repo *CampaignsRepository) {
				createTestCampaign(t, db, "camp-pr1", "PR Test 1")
			},
			inputDates:  []string{"2026-01-01"},
			wantPending: map[string]int{},
		},
		{
			name: "all purchases received returns zero pending (absent from map)",
			setup: func(t *testing.T, db *DB, repo *CampaignsRepository) {
				now := time.Now().Truncate(time.Second)
				createTestCampaign(t, db, "camp-pr2", "PR Test 2")
				receivedAt := now.Format(time.RFC3339)
				p := &campaigns.Purchase{
					ID: "pr-p1", CampaignID: "camp-pr2", CardName: "Charizard", CertNumber: "PR001",
					GradeValue: 9, BuyCostCents: 50000,
					PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-15",
					ReceivedAt: &receivedAt,
					CreatedAt:  now, UpdatedAt: now,
				}
				require.NoError(t, repo.CreatePurchase(ctx, p))
			},
			inputDates:  []string{"2026-01-15"},
			wantPending: map[string]int{},
		},
		{
			name: "partial receipt returns sum of unreceived buy_cost_cents",
			setup: func(t *testing.T, db *DB, repo *CampaignsRepository) {
				now := time.Now().Truncate(time.Second)
				createTestCampaign(t, db, "camp-pr3", "PR Test 3")
				receivedAt := now.Format(time.RFC3339)
				// received: 50000
				p1 := &campaigns.Purchase{
					ID: "pr-p2", CampaignID: "camp-pr3", CardName: "Charizard", CertNumber: "PR002",
					GradeValue: 9, BuyCostCents: 50000,
					PurchaseDate: "2026-02-10", InvoiceDate: "2026-02-15",
					ReceivedAt: &receivedAt,
					CreatedAt:  now, UpdatedAt: now,
				}
				// NOT received: 30000
				p2 := &campaigns.Purchase{
					ID: "pr-p3", CampaignID: "camp-pr3", CardName: "Pikachu", CertNumber: "PR003",
					GradeValue: 10, BuyCostCents: 30000,
					PurchaseDate: "2026-02-12", InvoiceDate: "2026-02-15",
					CreatedAt: now, UpdatedAt: now,
				}
				require.NoError(t, repo.CreatePurchase(ctx, p1))
				require.NoError(t, repo.CreatePurchase(ctx, p2))
			},
			inputDates:  []string{"2026-02-15"},
			wantPending: map[string]int{"2026-02-15": 30000},
		},
		{
			name: "refunded purchases excluded from pending",
			setup: func(t *testing.T, db *DB, repo *CampaignsRepository) {
				now := time.Now().Truncate(time.Second)
				createTestCampaign(t, db, "camp-pr4", "PR Test 4")
				// NOT received but refunded: should not count
				p1 := &campaigns.Purchase{
					ID: "pr-p4", CampaignID: "camp-pr4", CardName: "Venusaur", CertNumber: "PR004",
					GradeValue: 8, BuyCostCents: 25000,
					PurchaseDate: "2026-03-10", InvoiceDate: "2026-03-15",
					WasRefunded: true,
					CreatedAt:   now, UpdatedAt: now,
				}
				// NOT received and NOT refunded: should count
				p2 := &campaigns.Purchase{
					ID: "pr-p5", CampaignID: "camp-pr4", CardName: "Blastoise", CertNumber: "PR005",
					GradeValue: 9, BuyCostCents: 40000,
					PurchaseDate: "2026-03-12", InvoiceDate: "2026-03-15",
					CreatedAt: now, UpdatedAt: now,
				}
				require.NoError(t, repo.CreatePurchase(ctx, p1))
				require.NoError(t, repo.CreatePurchase(ctx, p2))
			},
			inputDates:  []string{"2026-03-15"},
			wantPending: map[string]int{"2026-03-15": 40000},
		},
		{
			name: "multiple invoice dates in a single call",
			setup: func(t *testing.T, db *DB, repo *CampaignsRepository) {
				now := time.Now().Truncate(time.Second)
				createTestCampaign(t, db, "camp-pr5", "PR Test 5")
				// date A: 20000 pending
				p1 := &campaigns.Purchase{
					ID: "pr-p6", CampaignID: "camp-pr5", CardName: "Mewtwo", CertNumber: "PR006",
					GradeValue: 10, BuyCostCents: 20000,
					PurchaseDate: "2026-04-01", InvoiceDate: "2026-04-05",
					CreatedAt: now, UpdatedAt: now,
				}
				// date B: 15000 pending
				p2 := &campaigns.Purchase{
					ID: "pr-p7", CampaignID: "camp-pr5", CardName: "Mew", CertNumber: "PR007",
					GradeValue: 9, BuyCostCents: 15000,
					PurchaseDate: "2026-04-10", InvoiceDate: "2026-04-12",
					CreatedAt: now, UpdatedAt: now,
				}
				require.NoError(t, repo.CreatePurchase(ctx, p1))
				require.NoError(t, repo.CreatePurchase(ctx, p2))
			},
			inputDates:  []string{"2026-04-05", "2026-04-12"},
			wantPending: map[string]int{"2026-04-05": 20000, "2026-04-12": 15000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			repo := NewCampaignsRepository(db.DB)
			tt.setup(t, db, repo)

			got, err := repo.GetPendingReceiptByInvoiceDate(ctx, tt.inputDates)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPending, got)
		})
	}
}
