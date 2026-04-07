# Capital Exposure Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the budget/limit-based capital exposure card with a velocity-based design showing outstanding capital, 30-day recovery rate, weeks-to-cover, and recovery trend.

**Architecture:** Modify the `CapitalSummary` Go struct to drop budget-centric fields and add recovery velocity fields. Rewrite the repository query to compute 30-day and prior-30-day recovery from `campaign_sales`. Update the frontend card, hero bar, advisor tools, LLM prompts, scoring module, and activation checklist to use the new fields.

**Tech Stack:** Go 1.26, SQLite, React/TypeScript, Tailwind CSS

---

### Task 1: Update Go domain type `CapitalSummary`

**Files:**
- Modify: `internal/domain/campaigns/types.go:235-246`

- [ ] **Step 1: Replace the CapitalSummary struct**

Replace the existing struct at lines 235-246 with:

```go
// CapitalSummary provides a snapshot of current capital exposure with recovery velocity.
type CapitalSummary struct {
	OutstandingCents       int     `json:"outstandingCents"`           // Unpaid purchases minus payments
	RecoveryRate30dCents   int     `json:"recoveryRate30dCents"`       // Sale revenue in last 30 days
	RecoveryRate30dPriorCents int  `json:"recoveryRate30dPriorCents"`  // Sale revenue in days 31-60
	WeeksToCover           float64 `json:"weeksToCover"`               // outstanding / weekly recovery rate (99 = no data)
	RecoveryTrend          string  `json:"recoveryTrend"`              // "improving", "declining", "stable"
	AlertLevel             string  `json:"alertLevel"`                 // "ok", "warning", "critical"
	RefundedCents          int     `json:"refundedCents"`              // Total refunds
	PaidCents              int     `json:"paidCents"`                  // Total paid
	UnpaidInvoiceCount     int     `json:"unpaidInvoiceCount"`
}
```

- [ ] **Step 2: Verify the build compiles (expect errors from consumers)**

Run: `go build ./internal/domain/campaigns/...`
Expected: SUCCESS (this package itself compiles; downstream consumers will fail until updated)

- [ ] **Step 3: Commit**

```bash
git add internal/domain/campaigns/types.go
git commit -m "refactor: replace budget-centric CapitalSummary with velocity-based fields"
```

---

### Task 2: Rewrite `GetCapitalSummary` repository query

**Files:**
- Modify: `internal/adapters/storage/sqlite/finance_repository.go:126-207`
- Test: `internal/adapters/storage/sqlite/finance_repository_test.go`

- [ ] **Step 1: Rewrite the existing tests to match new behavior**

Replace `TestGetCapitalSummary_AlertLevels` and `TestGetCapitalSummary_EmptyState` in `finance_repository_test.go` with:

```go
func TestGetCapitalSummary_WeeksToCover(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	tests := []struct {
		name           string
		outstandingSetup func(t *testing.T, repo *CampaignsRepository)
		salesSetup     func(t *testing.T, repo *CampaignsRepository)
		wantAlertLevel string
		wantTrend      string
		checkWeeks     func(t *testing.T, weeks float64)
	}{
		{
			name: "ok when weeks to cover under 6",
			outstandingSetup: func(t *testing.T, repo *CampaignsRepository) {
				// $500 outstanding (purchase $500, no payments)
				c := &campaigns.Campaign{ID: "camp-ok", Name: "OK", Phase: campaigns.PhaseActive, CreatedAt: now, UpdatedAt: now}
				require.NoError(t, repo.CreateCampaign(ctx, c))
				p := &campaigns.Purchase{
					ID: "ok-p", CampaignID: "camp-ok", CardName: "Charizard", CertNumber: "OK001",
					GradeValue: 9, BuyCostCents: 50000, PSASourcingFeeCents: 0,
					PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
					CreatedAt: now, UpdatedAt: now,
				}
				require.NoError(t, repo.CreatePurchase(ctx, p))
			},
			salesSetup: func(t *testing.T, repo *CampaignsRepository) {
				// $2000/month recovery → ~$465/week → ~1.1 weeks to cover $500
				for i := 0; i < 4; i++ {
					saleDate := time.Now().AddDate(0, 0, -i*7).Format("2006-01-02")
					s := &campaigns.Sale{
						ID: fmt.Sprintf("ok-s%d", i), PurchaseID: "ok-p",
						SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 50000,
						SaleFeeCents: 0, SaleDate: saleDate, DaysToSell: 10,
						NetProfitCents: 10000, CreatedAt: now, UpdatedAt: now,
					}
					// Only first sale can reference ok-p; use different purchase IDs
					// Actually we need separate purchases for separate sales
				}
			},
			wantAlertLevel: "ok",
			wantTrend:      "stable",
			checkWeeks: func(t *testing.T, weeks float64) {
				assert.Less(t, weeks, 6.0)
			},
		},
	}

	// (Placeholder structure — actual detailed tests below)
	_ = tests
}
```

Actually, let me write cleaner, more targeted tests:

```go
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
		// Two purchases: total $1000 outstanding
		for i, cert := range []string{"SELL001", "SELL002"} {
			p := &campaigns.Purchase{
				ID: fmt.Sprintf("sell-p%d", i), CampaignID: "camp-sell", CardName: "Pikachu",
				CertNumber: cert, GradeValue: 10, BuyCostCents: 50000, PSASourcingFeeCents: 0,
				PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
				CreatedAt: now, UpdatedAt: now,
			}
			require.NoError(t, repo.CreatePurchase(ctx, p))
		}

		// Sale within last 30 days: $800
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
		// weeklyRate = 80000 / 4.3 ≈ 18604; outstanding = 100000; weeks ≈ 5.4
		assert.Greater(t, summary.WeeksToCover, 4.0)
		assert.Less(t, summary.WeeksToCover, 7.0)
	})

	t.Run("improving trend when 30d exceeds prior 30d by more than 10%", func(t *testing.T) {
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

		// Recent sale: $500 (within last 30 days)
		recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
		s1 := &campaigns.Sale{
			ID: "trend-s1", PurchaseID: "trend-p1", SaleChannel: campaigns.SaleChannelEbay,
			SalePriceCents: 50000, SaleFeeCents: 6175, SaleDate: recentDate,
			DaysToSell: 5, NetProfitCents: 0, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateSale(ctx, s1))

		// Need a second purchase+sale for prior period
		p2 := &campaigns.Purchase{
			ID: "trend-p2", CampaignID: "camp-trend", CardName: "Mewtwo",
			CertNumber: "TR002", GradeValue: 10, BuyCostCents: 50000, PSASourcingFeeCents: 0,
			PurchaseDate: "2026-01-10", InvoiceDate: "2026-01-10",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreatePurchase(ctx, p2))

		// Prior period sale: $200 (between 31-60 days ago)
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

		// High outstanding ($50000), low recovery ($1000/30d)
		// weeklyRate = 1000/4.3 ≈ 232; weeks = 50000/232 ≈ 215 → critical
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
		assert.Equal(t, "critical", summary.AlertLevel) // >$10K outstanding, no recovery
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/storage/sqlite/ -run TestGetCapitalSummary -v`
Expected: FAIL (struct fields don't match)

- [ ] **Step 3: Rewrite `GetCapitalSummary` implementation**

Replace `GetCapitalSummary` in `finance_repository.go` (lines 126-207) with:

```go
func (r *CampaignsRepository) GetCapitalSummary(ctx context.Context) (*campaigns.CapitalSummary, error) {
	// Outstanding: invoiced non-refunded purchases minus payments
	var outstanding, refunded int
	err := r.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN was_refunded = 0 AND invoice_date != '' THEN buy_cost_cents + psa_sourcing_fee_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN was_refunded = 1 THEN buy_cost_cents + psa_sourcing_fee_cents ELSE 0 END), 0)
		FROM campaign_purchases WHERE invoice_date != ''`,
	).Scan(&outstanding, &refunded)
	if err != nil {
		return nil, err
	}

	// Paid total + unpaid count from invoices
	var paidTotal, unpaidCount int
	err = r.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN status IN ('paid', 'partial') THEN paid_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status != 'paid' THEN 1 ELSE 0 END), 0)
		FROM invoices`,
	).Scan(&paidTotal, &unpaidCount)
	if err != nil {
		return nil, err
	}

	outstanding -= paidTotal
	if outstanding < 0 {
		outstanding = 0
	}

	// Recovery velocity: 30-day and prior 30-day sale revenue
	var recovery30d, recoveryPrior30d int
	err = r.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN sale_date >= date('now', '-30 days') THEN sale_price_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN sale_date >= date('now', '-60 days') AND sale_date < date('now', '-30 days') THEN sale_price_cents ELSE 0 END), 0)
		FROM campaign_sales`,
	).Scan(&recovery30d, &recoveryPrior30d)
	if err != nil {
		return nil, err
	}

	// Derive weeks to cover
	weeksToCover := 99.0
	if recovery30d > 0 {
		weeklyRate := float64(recovery30d) / 4.3
		weeksToCover = float64(outstanding) / weeklyRate
	}

	// Derive trend (>10% delta = directional)
	trend := "stable"
	if recovery30d > 0 && recoveryPrior30d > 0 {
		ratio := float64(recovery30d) / float64(recoveryPrior30d)
		if ratio > 1.10 {
			trend = "improving"
		} else if ratio < 0.90 {
			trend = "declining"
		}
	}

	// Alert level based on weeks to cover
	alertLevel := "ok"
	if recovery30d > 0 {
		if weeksToCover > 12 {
			alertLevel = "critical"
		} else if weeksToCover >= 6 {
			alertLevel = "warning"
		}
	} else {
		// Fallback: no recovery data, use outstanding thresholds
		if outstanding > 1000000 { // >$10K
			alertLevel = "critical"
		} else if outstanding > 500000 { // >$5K
			alertLevel = "warning"
		}
	}

	return &campaigns.CapitalSummary{
		OutstandingCents:          outstanding,
		RecoveryRate30dCents:      recovery30d,
		RecoveryRate30dPriorCents: recoveryPrior30d,
		WeeksToCover:              weeksToCover,
		RecoveryTrend:             trend,
		AlertLevel:                alertLevel,
		RefundedCents:             refunded,
		PaidCents:                 paidTotal,
		UnpaidInvoiceCount:        unpaidCount,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/storage/sqlite/ -run TestGetCapitalSummary -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/sqlite/finance_repository.go internal/adapters/storage/sqlite/finance_repository_test.go
git commit -m "feat: rewrite GetCapitalSummary with recovery velocity computation"
```

---

### Task 3: Fix downstream Go consumers of old CapitalSummary fields

**Files:**
- Modify: `internal/testutil/mocks/campaign_service.go:361`
- Modify: `internal/testutil/mocks/campaign_repository.go:595-596`
- Modify: `internal/domain/campaigns/service_arbitrage.go:112-165`
- Modify: `internal/domain/campaigns/analytics_types.go:165` (WeeklyReviewSummary)
- Modify: `internal/domain/campaigns/service_portfolio.go:295-297`
- Modify: `internal/domain/advisor/scoring.go:47,187-188`
- Modify: `internal/domain/advisor/scoring_test.go:73-75`
- Modify: `internal/domain/scoring/factors.go:106-116` (ComputeCapitalPressure)
- Modify: `internal/adapters/httpserver/handlers/campaigns_finance_test.go:27,58-59`

- [ ] **Step 1: Update mocks default return values**

In `internal/testutil/mocks/campaign_service.go`, change line 361:
```go
return &campaigns.CapitalSummary{}, nil
```

In `internal/testutil/mocks/campaign_repository.go`, change line 596:
```go
return &campaigns.CapitalSummary{}, nil
```

- [ ] **Step 2: Update activation checklist in `service_arbitrage.go`**

The activation checklist uses `ExposurePct` and `CapitalBudgetCents`. Replace the capital exposure check (lines 114-122) with a weeks-to-cover check:

```go
	exposureCheckOK := capital.WeeksToCover < 12
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:    "Capital Exposure",
		Passed:  exposureCheckOK,
		Message: fmt.Sprintf("Weeks to cover: %.1f (threshold: 12)", capital.WeeksToCover),
	})
	if !exposureCheckOK {
		checklist.AllPassed = false
	}
```

Replace the daily exposure check (lines 157-166) — remove the `CapitalBudgetCents` reference. The daily exposure check doesn't need a budget anymore; just show the total daily spend as informational:

```go
	dailyExpOK := true // informational only without a budget
	exposureMsg := fmt.Sprintf("Total daily exposure with activation: $%d/day", totalDailyExposure/100)
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:    "Daily Exposure",
		Passed:  dailyExpOK,
		Message: exposureMsg,
	})
```

Update constant `CapitalExposureThresholdPct` — remove it (no longer used). Keep `DailyExposureDivisor` removal too if unused. Add a new constant if desired, or just use `12` inline since it matches the alert threshold.

- [ ] **Step 3: Update WeeklyReviewSummary and service_portfolio.go**

In `internal/domain/campaigns/analytics_types.go`, change the field:
```go
// Replace:  CapitalExposurePct   float64           `json:"capitalExposurePct"`
// With:
WeeksToCover float64 `json:"weeksToCover"`
```

In `internal/domain/campaigns/service_portfolio.go`, update the mapping (lines 294-298):
```go
	// Capital exposure
	capital, err := s.repo.GetCapitalSummary(ctx)
	if err == nil && capital != nil {
		summary.WeeksToCover = capital.WeeksToCover
	}
```

- [ ] **Step 4: Update scoring module**

In `internal/domain/advisor/scoring.go`, rename the `LiquidationFactorData` field (line 47):
```go
// Replace:  CapitalExposurePct *float64
// With:
WeeksToCover *float64
```

Update `liquidationFactors` (lines 187-189) to use weeks-to-cover for capital pressure. `ComputeCapitalPressure` in `scoring/factors.go` currently takes a percentage. Adapt it to work with weeks-to-cover instead:

In `internal/domain/scoring/factors.go`, replace `ComputeCapitalPressure` (lines 106-120):
```go
func ComputeCapitalPressure(weeksToCover, confidence float64, source string) Factor {
	var v float64
	switch {
	case weeksToCover > 20:
		v = -1.0
	case weeksToCover > 12:
		v = -0.6
	case weeksToCover > 6:
		v = -0.3
	default:
		v = 0.0
	}
	return Factor{
		Name:       FactorCapitalPressure,
		Value:      v,
		Confidence: confidence,
		Source:     source,
	}
}
```

Update `internal/domain/advisor/scoring.go` lines 187-189:
```go
	addOrGap(&factors, &gaps, d.WeeksToCover != nil, func() scoring.Factor {
		return scoring.ComputeCapitalPressure(*d.WeeksToCover, 1.0, "capital")
	}, scoring.FactorCapitalPressure, gapNoMarketData)
```

Update test in `internal/domain/advisor/scoring_test.go` line 75:
```go
				WeeksToCover: ptrFloat(8.0),
```

- [ ] **Step 5: Update HTTP handler test**

In `internal/adapters/httpserver/handlers/campaigns_finance_test.go`, update the test for `HandleCapitalSummary`:

Change the mock return (line 27):
```go
return &campaigns.CapitalSummary{OutstandingCents: 50000, WeeksToCover: 5.0, AlertLevel: "ok", RecoveryTrend: "stable"}, nil
```

Change the assertion (lines 53-59) — instead of checking `CapitalBudgetCents`, check `OutstandingCents`:
```go
			if tt.wantStatus == http.StatusOK {
				var result campaigns.CapitalSummary
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result.OutstandingCents != 50000 {
					t.Errorf("expected OutstandingCents=50000, got %d", result.OutstandingCents)
				}
			} else {
				decodeErrorResponse(t, rec)
			}
```

Also update the `wantBudgetCents` field in the test struct — rename it to `wantOutstandingCents` or remove the field and inline the assertion.

- [ ] **Step 6: Run full backend tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/testutil/mocks/campaign_service.go internal/testutil/mocks/campaign_repository.go \
  internal/domain/campaigns/service_arbitrage.go internal/domain/campaigns/analytics_types.go \
  internal/domain/campaigns/service_portfolio.go internal/domain/advisor/scoring.go \
  internal/domain/advisor/scoring_test.go internal/domain/scoring/factors.go \
  internal/adapters/httpserver/handlers/campaigns_finance_test.go
git commit -m "fix: update all Go consumers of CapitalSummary to use velocity fields"
```

---

### Task 4: Update advisor tools and LLM prompts

**Files:**
- Modify: `internal/adapters/advisortool/tools_portfolio.go:37-48,93-125,127-156`
- Modify: `internal/domain/advisor/prompts.go` (5 edits)
- Modify: `internal/adapters/advisortool/executor_test.go` (if it references old fields)

- [ ] **Step 1: Update `get_capital_summary` tool description**

In `tools_portfolio.go` line 40, replace the description:
```go
Description: "Get capital exposure: outstanding balance, 30-day recovery rate, weeks to cover, recovery trend, and alert level.",
```

- [ ] **Step 2: Update `dashboardSummary.Capital` struct and mapping**

Replace the `Capital` struct in `dashboardSummary` (lines 106-112):
```go
	Capital struct {
		BalanceCents         int     `json:"balanceCents"`
		RecoveryRate30dCents int     `json:"recoveryRate30dCents"`
		WeeksToCover         float64 `json:"weeksToCover"`
		RecoveryTrend        string  `json:"recoveryTrend"`
		AlertLevel           string  `json:"alertLevel"`
	} `json:"capital"`
```

Update the mapping (lines 150-155):
```go
		ds.Capital.BalanceCents = cs.OutstandingCents
		ds.Capital.RecoveryRate30dCents = cs.RecoveryRate30dCents
		ds.Capital.WeeksToCover = cs.WeeksToCover
		ds.Capital.RecoveryTrend = cs.RecoveryTrend
		ds.Capital.AlertLevel = cs.AlertLevel
```

- [ ] **Step 3: Update `get_dashboard_summary` tool description**

In `tools_portfolio.go` line 130, replace:
```go
Description: "Get a compact portfolio overview: weekly performance, capital velocity, campaign statuses, and channel velocity. Start here before drilling into specific tools.",
```

- [ ] **Step 4: Update LLM prompts (5 edits)**

In `internal/domain/advisor/prompts.go`:

**Edit 1** — `baseSystemPrompt` line 22, replace:
```
- There is no credit limit. Outstanding balance and projected exposure matter for capital allocation — how much cash is tied up in PSA inventory
```
with:
```
- Outstanding balance, recovery rate, and weeks-to-cover drive capital allocation — how much cash is tied up and how fast it cycles back
```

**Edit 2** — `digestUserPrompt` line 66, replace:
```
2. Cash flow (outstanding balance, projected exposure, payment status)
```
with:
```
2. Cash flow (outstanding balance, recovery rate, weeks to cover, payment status)
```

**Edit 3** — `digestUserPrompt` line 74, replace:
```
3. **Cash Flow** — outstanding balance, projected exposure, unpaid invoices, days to next invoice
```
with:
```
3. **Cash Flow** — outstanding balance, 30d recovery rate, weeks to cover, recovery trend, unpaid invoices
```

**Edit 4** — `liquidationSystemPrompt` lines 139-141, replace:
```
4. **Capital pressure adjustment** — if outstanding balance is high relative to
   projected revenue, lower the bar for all liquidation actions. Cards you would
   normally hold become sells when capital is tied up unproductively.
```
with:
```
4. **Capital pressure adjustment** — if weeks-to-cover exceeds 12 (critical), lower the bar for all liquidation actions. The higher the weeks-to-cover, the more aggressively capital should be freed. Cards you would normally hold become sells when capital is tied up unproductively.
```

**Edit 5** — `liquidationUserPrompt` line 174, replace:
```
1. **Capital Position** — outstanding balance, projected exposure, capital tied up in stale inventory
```
with:
```
1. **Capital Position** — outstanding balance, recovery rate, weeks to cover, recovery trend, capital tied up in stale inventory
```

- [ ] **Step 5: Update executor test if needed**

Check `internal/adapters/advisortool/executor_test.go` for references to old `dashboardSummary` fields (`budgetCents`, `exposurePct`, `daysToInvoice`) and update them.

- [ ] **Step 6: Run backend tests**

Run: `go test ./internal/adapters/advisortool/... ./internal/domain/advisor/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/advisortool/tools_portfolio.go internal/domain/advisor/prompts.go \
  internal/adapters/advisortool/executor_test.go
git commit -m "feat: update advisor tools and LLM prompts for velocity-based capital"
```

---

### Task 5: Update TypeScript types

**Files:**
- Modify: `web/src/types/campaigns/market.ts:160-170`
- Modify: `web/src/types/campaigns/portfolio.ts:137`
- Modify: `web/tests/pages/CampaignsPage.test.tsx:38-41,71`

- [ ] **Step 1: Update `CapitalSummary` interface**

In `web/src/types/campaigns/market.ts`, replace the `CapitalSummary` interface (lines 160-170):
```typescript
export interface CapitalSummary {
  outstandingCents: number;
  recoveryRate30dCents: number;
  recoveryRate30dPriorCents: number;
  weeksToCover: number;         // 99 = no recovery data
  recoveryTrend: 'improving' | 'declining' | 'stable';
  alertLevel: 'ok' | 'warning' | 'critical';
  unpaidInvoiceCount: number;
  refundedCents: number;
  paidCents: number;
}
```

- [ ] **Step 2: Update `WeeklyReviewSummary` interface**

In `web/src/types/campaigns/portfolio.ts` line 137, replace:
```typescript
// Replace:  capitalExposurePct: number;
// With:
  weeksToCover: number;
```

- [ ] **Step 3: Update test mock data**

In `web/tests/pages/CampaignsPage.test.tsx`, update the `getCapitalSummary` mock (lines 38-44):
```typescript
    getCapitalSummary: vi.fn(() => Promise.resolve({
      outstandingCents: 0,
      recoveryRate30dCents: 0,
      recoveryRate30dPriorCents: 0,
      weeksToCover: 99,
      recoveryTrend: 'stable' as const,
      alertLevel: 'ok' as const,
      refundedCents: 0,
      paidCents: 0,
      unpaidInvoiceCount: 0,
    })),
```

Update the `getWeeklyReview` mock (line 71):
```typescript
// Replace:  capitalExposurePct: 0,
// With:
      weeksToCover: 99,
```

- [ ] **Step 4: Run TypeScript type checks**

Run: `cd web && npx tsc --noEmit`
Expected: Errors in `CapitalExposurePanel.tsx` and `HeroStatsBar.tsx` (expected — fixed in next tasks)

- [ ] **Step 5: Commit**

```bash
git add web/src/types/campaigns/market.ts web/src/types/campaigns/portfolio.ts \
  web/tests/pages/CampaignsPage.test.tsx
git commit -m "refactor: update TypeScript types for velocity-based capital summary"
```

---

### Task 6: Rebuild `CapitalExposurePanel` component

**Files:**
- Modify: `web/src/react/components/portfolio/CapitalExposurePanel.tsx`

- [ ] **Step 1: Rewrite the component**

Replace the entire content of `CapitalExposurePanel.tsx`:

```tsx
import type { CapitalSummary } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';

interface CapitalExposurePanelProps {
  capital?: CapitalSummary;
}

function weeksBadge(capital: CapitalSummary) {
  if (capital.recoveryRate30dCents === 0) {
    return <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-[var(--surface-2)] text-[var(--text-muted)]">No sales data</span>;
  }
  const weeks = capital.weeksToCover;
  const label = weeks > 20 ? '20+ wks' : `~${Math.round(weeks)} wks`;
  const color = capital.alertLevel === 'critical' ? 'bg-[var(--danger)] text-white'
    : capital.alertLevel === 'warning' ? 'bg-[var(--warning)] text-black'
    : 'bg-[var(--success)] text-white';
  return <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${color}`}>{label}</span>;
}

function trendArrow(trend: string) {
  if (trend === 'improving') return <span className="text-[var(--success)]" title="Improving">&#9650;</span>;
  if (trend === 'declining') return <span className="text-[var(--danger)]" title="Declining">&#9660;</span>;
  return <span className="text-[var(--text-muted)]" title="Stable">&#9654;</span>;
}

export default function CapitalExposurePanel({ capital }: CapitalExposurePanelProps) {
  if (!capital) return null;

  const outstandingColor = capital.outstandingCents === 0 ? 'text-[var(--success)]' : 'text-[var(--text)]';

  return (
    <div className="h-full p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Capital Exposure</h3>

      <div className="flex items-baseline gap-3 mb-2">
        <span className={`text-2xl font-bold ${outstandingColor}`}>{formatCents(capital.outstandingCents)}</span>
        {weeksBadge(capital)}
      </div>

      {capital.recoveryRate30dCents > 0 && (
        <div className="text-xs text-[var(--text-muted)] mb-2">
          {formatCents(capital.recoveryRate30dCents)}/mo recovered {trendArrow(capital.recoveryTrend)}
        </div>
      )}

      {capital.recoveryRate30dCents === 0 && capital.outstandingCents > 0 && (
        <div className="text-xs text-[var(--text-muted)] mb-2">No recovery data yet</div>
      )}

      {(capital.unpaidInvoiceCount > 0 || capital.refundedCents > 0) && (
        <div className="text-xs text-[var(--text-muted)]">
          {capital.unpaidInvoiceCount > 0 && (
            <span>{capital.unpaidInvoiceCount} unpaid invoice{capital.unpaidInvoiceCount !== 1 ? 's' : ''}</span>
          )}
          {capital.refundedCents > 0 && (
            <span>{capital.unpaidInvoiceCount > 0 ? ' | ' : ''}{formatCents(capital.refundedCents)} refunded</span>
          )}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Run type check**

Run: `cd web && npx tsc --noEmit`
Expected: May still fail on `HeroStatsBar.tsx` (fixed next task), but `CapitalExposurePanel.tsx` should be clean

- [ ] **Step 3: Commit**

```bash
git add web/src/react/components/portfolio/CapitalExposurePanel.tsx
git commit -m "feat: rebuild CapitalExposurePanel with velocity-based layout"
```

---

### Task 7: Update `HeroStatsBar` component

**Files:**
- Modify: `web/src/react/components/portfolio/HeroStatsBar.tsx:38-55`

- [ ] **Step 1: Replace the capital section**

In `HeroStatsBar.tsx`, replace the capital section (lines 38-55):

```tsx
          {capital && (
            <>
              <div className="border-l border-[rgba(255,255,255,0.08)] pl-6">
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Wks to Cover</div>
                <div className={`text-base font-semibold ${
                  capital.alertLevel === 'critical' ? 'text-[var(--danger)]'
                    : capital.alertLevel === 'warning' ? 'text-[var(--warning)]'
                    : capital.recoveryRate30dCents === 0 ? 'text-[var(--text-muted)]'
                    : 'text-[var(--success)]'
                }`}>
                  {capital.recoveryRate30dCents === 0 ? '—' : capital.weeksToCover > 20 ? '20+' : `~${Math.round(capital.weeksToCover)}`}
                </div>
              </div>
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Outstanding</div>
                <div className="text-base font-semibold text-[#cbd5e1]">{formatCents(capital.outstandingCents ?? 0)}</div>
              </div>
            </>
          )}
```

- [ ] **Step 2: Run type check and lint**

Run: `cd web && npx tsc --noEmit && npm run lint`
Expected: PASS

- [ ] **Step 3: Run frontend tests**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/react/components/portfolio/HeroStatsBar.tsx
git commit -m "feat: update HeroStatsBar capital stat to show weeks-to-cover"
```

---

### Task 8: Final verification

- [ ] **Step 1: Run full backend tests with race detection**

Run: `go test -race -timeout 10m ./...`
Expected: PASS

- [ ] **Step 2: Run quality checks**

Run: `make check`
Expected: PASS (lint + architecture import check + file size check)

- [ ] **Step 3: Run frontend tests and lint**

Run: `cd web && npm test && npm run lint`
Expected: PASS

- [ ] **Step 4: Build backend**

Run: `go build -o slabledger ./cmd/slabledger`
Expected: SUCCESS

- [ ] **Step 5: Build frontend**

Run: `cd web && npm run build`
Expected: SUCCESS
