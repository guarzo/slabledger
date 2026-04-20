# Phase 6a — App Fitness on Fly/Supabase Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the user-visible inventory slowness, harden the app against transient Supabase outages at startup, and emit enough metrics to see DB pool and Go runtime health in the Fly-managed Grafana.

**Architecture:** Three independent changes, each tested before shipping, each small enough to revert alone. Inventory fix batches an N+1 comp-summary lookup into a single SQL round trip. Migration safety adds an exponential-backoff retry around the initial DB ping (not around migrations themselves — those stay fail-fast). Observability adds `prometheus/client_golang` on a separate `:9091` port, registered with Go runtime collectors and a custom `sql.DBStats` collector, scraped automatically by Fly's built-in Prometheus.

**Tech Stack:** Go 1.26, `jackc/pgx/v5/stdlib`, `golang-migrate/migrate/v4`, `prometheus/client_golang` (new), `observability.Logger` (structured slog wrapper in this repo), Supabase Postgres, Fly.io (fly-metrics.net for Grafana).

**Spec:** `docs/specs/2026-04-20-phase6a-app-fitness-on-fly-design.md`

---

## File Structure

### New files
- `internal/adapters/storage/postgres/retry.go` — pure helper for exponential-backoff retry of a `func() error`. Isolated for testability.
- `internal/adapters/storage/postgres/retry_test.go` — unit tests with injected sleep.
- `internal/adapters/storage/postgres/metrics.go` — custom Prometheus collector wrapping `sql.DBStats`.
- `internal/adapters/storage/postgres/metrics_test.go` — registers collector against a test registry, asserts metric names/values.
- `internal/adapters/storage/postgres/cl_sales_store_test.go` — new test file (none exists yet for this store) scoped to the new batch method.

### Modified files
- `internal/domain/inventory/comp_summary.go` — add `CompKey` type and `GetCompSummariesByKeys` to `CompSummaryProvider`.
- `internal/adapters/storage/postgres/cl_sales_store.go` — implement `GetCompSummariesByKeys`; factor out the single-key SQL so the batch path and the existing path share helpers.
- `internal/domain/inventory/comp_enrichment.go` — replace the per-key loop with one batch call.
- `internal/domain/inventory/service_analytics.go` — wrap `GetInventoryAging` and `GetGlobalInventoryAging` with phase-timing log lines.
- `internal/adapters/storage/postgres/db.go` — call the retry helper around the initial `PingContext`.
- `cmd/slabledger/main.go` — register Go runtime collectors + the DBStats collector; start a `:9091` metrics HTTP server; wire its shutdown into the existing lifecycle.
- `go.mod` / `go.sum` — new dependency.
- `fly.toml` — add `[metrics]` block.

---

## Section A — Inventory slowness

The prime suspect (from the spec + code read) is `enrichCompSummaries` issuing three serial queries per unique `(gemRateID, grade)` group: `lookupCondition`, the aggregation, and `fetchRecentPricesAndDates`. For ~100 groups that is ~300 serial round trips to Supabase at ~15ms each. The batch rewrite collapses that to three queries total.

### Task 1: Add phase-timing log lines to inventory aging

**Why:** Ship telemetry before the fix so we can confirm (in prod logs) that the fix actually moves the needle, and so any remaining slowness is visible.

**Files:**
- Modify: `internal/domain/inventory/service_analytics.go:194-213` (GetInventoryAging)
- Modify: `internal/domain/inventory/service_analytics.go:215-258` (GetGlobalInventoryAging)

- [ ] **Step 1: Add `"time"` to the imports in `service_analytics.go`**

The existing import block imports `context`, `encoding/json`, `fmt`, `log/slog`, `math`, and two project packages. Add `"time"` to the stdlib group. Resulting import block:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/timeutil"
)
```

- [ ] **Step 2: Replace `GetInventoryAging` body with timing instrumentation**

Replace `service_analytics.go:194-213` with:

```go
func (s *service) GetInventoryAging(ctx context.Context, campaignID string) (*InventoryResult, error) {
	start := time.Now()
	var phasePurchases, phaseEnrich, phaseFlags, phaseComps time.Duration

	t0 := time.Now()
	unsold, err := s.purchases.ListUnsoldPurchases(ctx, campaignID)
	phasePurchases = time.Since(t0)
	if err != nil {
		return nil, err
	}

	t0 = time.Now()
	items := make([]AgingItem, 0, len(unsold))
	for i := range unsold {
		items = append(items, s.enrichAgingItem(ctx, &unsold[i], ""))
	}
	phaseEnrich = time.Since(t0)

	result := &InventoryResult{Items: items}

	t0 = time.Now()
	if err := s.applyOpenFlags(ctx, items); err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "applyOpenFlags failed", observability.Err(err))
		}
		result.Warnings = append(result.Warnings, "Price flag data unavailable")
	}
	phaseFlags = time.Since(t0)

	t0 = time.Now()
	s.enrichCompSummaries(ctx, items)
	phaseComps = time.Since(t0)

	if s.logger != nil {
		s.logger.Info(ctx, "inventory aging completed",
			observability.String("campaignId", campaignID),
			observability.Int("itemCount", len(items)),
			observability.Float64("totalMs", float64(time.Since(start).Milliseconds())),
			observability.Float64("purchasesMs", float64(phasePurchases.Milliseconds())),
			observability.Float64("enrichMs", float64(phaseEnrich.Milliseconds())),
			observability.Float64("openFlagsMs", float64(phaseFlags.Milliseconds())),
			observability.Float64("compSummariesMs", float64(phaseComps.Milliseconds())),
		)
	}

	return result, nil
}
```

- [ ] **Step 3: Do the same for `GetGlobalInventoryAging`**

Replace `service_analytics.go:215-258` with:

```go
func (s *service) GetGlobalInventoryAging(ctx context.Context) (*InventoryResult, error) {
	start := time.Now()
	var phasePurchases, phaseCampaigns, phaseEnrich, phaseFlags, phaseComps, phaseCracks, phaseSignals time.Duration

	t0 := time.Now()
	purchases, err := s.purchases.ListAllUnsoldPurchases(ctx)
	phasePurchases = time.Since(t0)
	if err != nil {
		return nil, fmt.Errorf("list unsold purchases: %w", err)
	}

	t0 = time.Now()
	campaignList, err := s.campaigns.ListCampaigns(ctx, false)
	phaseCampaigns = time.Since(t0)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	campaignNames := make(map[string]string, len(campaignList))
	for _, c := range campaignList {
		campaignNames[c.ID] = c.Name
	}

	t0 = time.Now()
	items := make([]AgingItem, 0, len(purchases))
	for i := range purchases {
		items = append(items, s.enrichAgingItem(ctx, &purchases[i], campaignNames[purchases[i].CampaignID]))
	}
	phaseEnrich = time.Since(t0)

	result := &InventoryResult{Items: items}

	t0 = time.Now()
	if err := s.applyOpenFlags(ctx, items); err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "applyOpenFlags failed", observability.Err(err))
		}
		result.Warnings = append(result.Warnings, "Price flag data unavailable")
	}
	phaseFlags = time.Since(t0)

	t0 = time.Now()
	s.enrichCompSummaries(ctx, items)
	phaseComps = time.Since(t0)

	t0 = time.Now()
	crackSet := s.buildCrackCandidateSet(ctx)
	phaseCracks = time.Since(t0)

	t0 = time.Now()
	for i := range items {
		isCrack := crackSet[items[i].Purchase.ID]
		sig := ComputeInventorySignals(&items[i], isCrack)
		if sig.HasAnySignal() {
			items[i].Signals = &sig
		}
	}
	phaseSignals = time.Since(t0)

	if s.logger != nil {
		s.logger.Info(ctx, "global inventory aging completed",
			observability.Int("itemCount", len(items)),
			observability.Float64("totalMs", float64(time.Since(start).Milliseconds())),
			observability.Float64("purchasesMs", float64(phasePurchases.Milliseconds())),
			observability.Float64("campaignsMs", float64(phaseCampaigns.Milliseconds())),
			observability.Float64("enrichMs", float64(phaseEnrich.Milliseconds())),
			observability.Float64("openFlagsMs", float64(phaseFlags.Milliseconds())),
			observability.Float64("compSummariesMs", float64(phaseComps.Milliseconds())),
			observability.Float64("cracksMs", float64(phaseCracks.Milliseconds())),
			observability.Float64("signalsMs", float64(phaseSignals.Milliseconds())),
		)
	}

	return result, nil
}
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 5: Verify existing tests still pass**

Run: `go test ./internal/domain/inventory/...`
Expected: `ok  ...`, no regressions.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/inventory/service_analytics.go
git commit -m "feat(inventory): add phase-timing logs to aging queries

So we can see where the inventory page spends time in prod logs."
```

---

### Task 2: Add `CompKey` type and `GetCompSummariesByKeys` to the interface

**Files:**
- Modify: `internal/domain/inventory/comp_summary.go`

- [ ] **Step 1: Add `CompKey` and batch method to the interface**

Replace `comp_summary.go:31-39` (the existing `CompSummaryProvider` interface block) with:

```go
// CompKey identifies a unique card variant + condition for comp lookup.
// The CertNumber is used (not grade) because the existing pipeline resolves
// condition from cl_card_mappings by cert.
type CompKey struct {
	GemRateID  string
	CertNumber string
}

// CompSummaryProvider computes comp analytics for a card variant at a specific grade.
type CompSummaryProvider interface {
	// GetCompSummary returns aggregated comp data for a gemRateID filtered by grade.
	// certNumber resolves the CL condition (grade) from the card mapping table so comps
	// are grade-specific (e.g., PSA 10 only, not mixed with PSA 9).
	// CompsAboveCL and CompsAboveCost are left at 0 — the caller derives them per-purchase
	// from PriceCentsList since different purchases may have different CL values and costs.
	GetCompSummary(ctx context.Context, gemRateID, certNumber string) (*CompSummary, error)

	// GetCompSummariesByKeys is the batch form of GetCompSummary. It returns one
	// *CompSummary per input key (same semantics: CompsAboveCL/CompsAboveCost left at 0;
	// PriceCentsList populated so the caller can derive those per-purchase). Missing or
	// no-data keys are absent from the returned map rather than mapping to nil.
	GetCompSummariesByKeys(ctx context.Context, keys []CompKey) (map[CompKey]*CompSummary, error)
}
```

- [ ] **Step 2: Verify build fails — CLSalesStore no longer satisfies the interface**

Run: `go build ./...`
Expected: `internal/adapters/storage/postgres/cl_sales_store.go:337:5: cannot use (*CLSalesStore)(nil) (value of type *CLSalesStore) as inventory.CompSummaryProvider value in variable declaration: *CLSalesStore does not implement inventory.CompSummaryProvider (missing method GetCompSummariesByKeys)`

This failure is expected — it proves the interface change is load-bearing. The next task implements the method.

- [ ] **Step 3: Do not commit yet** — the tree does not compile. Task 3 will finish the change and commit together.

---

### Task 3: Implement `GetCompSummariesByKeys` on `CLSalesStore` with test

**Files:**
- Modify: `internal/adapters/storage/postgres/cl_sales_store.go`
- Create: `internal/adapters/storage/postgres/cl_sales_store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/storage/postgres/cl_sales_store_test.go`:

```go
package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLSalesStore_GetCompSummariesByKeys(t *testing.T) {
	db := setupTestDB(t)
	store := NewCLSalesStore(db.DB)
	ctx := context.Background()

	// Seed two card variants (different gemRateIDs) with cl_card_mappings + comps.
	// gem-a / cert-a → PSA 10, 5 sales over last 30 days
	// gem-b / cert-b → PSA 9,  3 sales over last 30 days
	_, err := db.ExecContext(ctx,
		`INSERT INTO cl_card_mappings (slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition)
		 VALUES ('cert-a', 'coll-a', 'gem-a', 'PSA 10'),
		        ('cert-b', 'coll-b', 'gem-b', 'PSA 9')`)
	require.NoError(t, err)

	// Insert sales comps. Use recent dates so they fall inside the 90-day window.
	now := "2026-04-20"
	for _, row := range [][]any{
		{"gem-a", "item-a-1", now, 10000, "ebay", "PSA 10"},
		{"gem-a", "item-a-2", now, 12000, "ebay", "PSA 10"},
		{"gem-a", "item-a-3", now, 11000, "ebay", "PSA 10"},
		{"gem-a", "item-a-4", now, 13000, "ebay", "PSA 10"},
		{"gem-a", "item-a-5", now, 9000, "ebay", "PSA 10"},
		{"gem-b", "item-b-1", now, 4000, "ebay", "PSA 9"},
		{"gem-b", "item-b-2", now, 5000, "ebay", "PSA 9"},
		{"gem-b", "item-b-3", now, 4500, "ebay", "PSA 9"},
	} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO cl_sales_comps
			 (gem_rate_id, item_id, sale_date, price_cents, platform, condition)
			 VALUES ($1, $2, $3, $4, $5, $6)`, row...)
		require.NoError(t, err)
	}

	keys := []inventory.CompKey{
		{GemRateID: "gem-a", CertNumber: "cert-a"},
		{GemRateID: "gem-b", CertNumber: "cert-b"},
		{GemRateID: "gem-missing", CertNumber: "cert-missing"}, // absent in mappings → skipped
	}
	got, err := store.GetCompSummariesByKeys(ctx, keys)
	require.NoError(t, err)

	require.Len(t, got, 2)

	a := got[inventory.CompKey{GemRateID: "gem-a", CertNumber: "cert-a"}]
	require.NotNil(t, a)
	assert.Equal(t, 5, a.TotalComps)
	assert.Equal(t, 5, a.RecentComps)
	assert.Equal(t, 11000, a.MedianCents)
	assert.Equal(t, 13000, a.HighestCents)
	assert.Equal(t, 9000, a.LowestCents)
	assert.Len(t, a.PriceCentsList, 5)

	b := got[inventory.CompKey{GemRateID: "gem-b", CertNumber: "cert-b"}]
	require.NotNil(t, b)
	assert.Equal(t, 3, b.TotalComps)
	assert.Equal(t, 3, b.RecentComps)
	assert.Equal(t, 4500, b.MedianCents)
}

func TestCLSalesStore_GetCompSummariesByKeys_EmptyInput(t *testing.T) {
	db := setupTestDB(t)
	store := NewCLSalesStore(db.DB)
	got, err := store.GetCompSummariesByKeys(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/adapters/storage/postgres/ -run TestCLSalesStore_GetCompSummariesByKeys -v`
Expected: compile error (method doesn't exist on `*CLSalesStore`) OR test failure. Either way, not green.

- [ ] **Step 3: Implement `GetCompSummariesByKeys`**

Add to `internal/adapters/storage/postgres/cl_sales_store.go` (anywhere after `GetCompSummary`, before the `var _ inventory.CompSummaryProvider = ...` line):

```go
// GetCompSummariesByKeys is the batch form of GetCompSummary. It issues three
// SQL queries total regardless of the number of keys: (1) resolve conditions
// from cl_card_mappings, (2) aggregate counts/extrema, (3) fetch recent price
// lists for median/trend. Keys whose cert has no mapping, or whose variant has
// no recent comps, are absent from the returned map.
func (s *CLSalesStore) GetCompSummariesByKeys(ctx context.Context, keys []inventory.CompKey) (map[inventory.CompKey]*inventory.CompSummary, error) {
	out := make(map[inventory.CompKey]*inventory.CompSummary)
	if len(keys) == 0 {
		return out, nil
	}

	// 1. Batch-resolve conditions for all cert numbers.
	certs := make([]string, 0, len(keys))
	for _, k := range keys {
		if k.CertNumber != "" {
			certs = append(certs, k.CertNumber)
		}
	}
	conditions, err := s.lookupConditionsBatch(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("batch lookup conditions: %w", err)
	}

	// Reduce keys to unique (gemRateID, condition) pairs we actually want to query,
	// and remember the mapping back to input keys.
	type pair struct{ gemRateID, condition string }
	pairToKeys := make(map[pair][]inventory.CompKey)
	gemIDs := make([]string, 0, len(keys))
	conds := make([]string, 0, len(keys))
	for _, k := range keys {
		cond, ok := conditions[k.CertNumber]
		if !ok || cond == "" {
			continue
		}
		p := pair{gemRateID: k.GemRateID, condition: cond}
		if _, seen := pairToKeys[p]; !seen {
			gemIDs = append(gemIDs, k.GemRateID)
			conds = append(conds, cond)
		}
		pairToKeys[p] = append(pairToKeys[p], k)
	}
	if len(pairToKeys) == 0 {
		return out, nil
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -90).Format("2006-01-02")
	midCutoff := now.AddDate(0, 0, -45).Format("2006-01-02")

	// 2. Aggregation: one row per (gem_rate_id, condition) pair.
	aggQuery := `
		SELECT gem_rate_id, condition,
			COUNT(*) AS total_comps,
			COUNT(CASE WHEN sale_date >= $3 THEN 1 END) AS recent_comps,
			MAX(CASE WHEN sale_date >= $3 THEN price_cents END) AS high_cents,
			MIN(CASE WHEN sale_date >= $3 THEN price_cents END) AS low_cents,
			MAX(sale_date) AS last_sale_date
		FROM cl_sales_comps
		WHERE (gem_rate_id, condition) IN (SELECT UNNEST($1::text[]), UNNEST($2::text[]))
		GROUP BY gem_rate_id, condition`
	rows, err := s.db.QueryContext(ctx, aggQuery, gemIDs, conds, cutoff)
	if err != nil {
		return nil, fmt.Errorf("aggregate: %w", err)
	}
	aggs := make(map[pair]*inventory.CompSummary)
	for rows.Next() {
		var p pair
		var totalComps, recentComps int
		var highCents, lowCents sql.NullInt64
		var lastSaleDate sql.NullString
		if err := rows.Scan(&p.gemRateID, &p.condition, &totalComps, &recentComps, &highCents, &lowCents, &lastSaleDate); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan aggregate: %w", err)
		}
		if totalComps == 0 || recentComps == 0 {
			continue
		}
		sum := &inventory.CompSummary{
			GemRateID:    p.gemRateID,
			TotalComps:   totalComps,
			RecentComps:  recentComps,
			HighestCents: int(highCents.Int64),
			LowestCents:  int(lowCents.Int64),
			LastSaleDate: lastSaleDate.String,
		}
		aggs[p] = sum
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate aggregate: %w", err)
	}
	_ = rows.Close()

	if len(aggs) == 0 {
		return out, nil
	}

	// 3. Recent prices + sale dates for every matched pair (for median + trend).
	priceQuery := `
		SELECT gem_rate_id, condition, price_cents, sale_date, platform
		FROM cl_sales_comps
		WHERE (gem_rate_id, condition) IN (SELECT UNNEST($1::text[]), UNNEST($2::text[]))
		  AND sale_date >= $3
		ORDER BY gem_rate_id, condition, sale_date`
	priceRows, err := s.db.QueryContext(ctx, priceQuery, gemIDs, conds, cutoff)
	if err != nil {
		return nil, fmt.Errorf("recent prices: %w", err)
	}
	type priceRow struct {
		priceCents int
		saleDate   string
		platform   string
	}
	byPair := make(map[pair][]priceRow)
	for priceRows.Next() {
		var p pair
		var pr priceRow
		if err := priceRows.Scan(&p.gemRateID, &p.condition, &pr.priceCents, &pr.saleDate, &pr.platform); err != nil {
			_ = priceRows.Close()
			return nil, fmt.Errorf("scan recent prices: %w", err)
		}
		byPair[p] = append(byPair[p], pr)
	}
	if err := priceRows.Err(); err != nil {
		_ = priceRows.Close()
		return nil, fmt.Errorf("iterate recent prices: %w", err)
	}
	_ = priceRows.Close()

	// Compute median, trend, platform breakdown, attach PriceCentsList.
	for p, sum := range aggs {
		prs := byPair[p]
		if len(prs) == 0 {
			continue
		}
		prices := make([]int, len(prs))
		dates := make([]string, len(prs))
		for i, pr := range prs {
			prices[i] = pr.priceCents
			dates[i] = pr.saleDate
		}
		sum.MedianCents = medianInt(slices.Clone(prices))
		sum.Trend90d = computeTrend(prices, dates, midCutoff)
		sum.ByPlatform = platformBreakdownFromRows(prs)
		sum.PriceCentsList = prices
		if len(prs) > 0 {
			sum.LastSaleCents = prs[len(prs)-1].priceCents
		}

		// Fan out to every input key that resolved to this (gemRateID, condition).
		for _, k := range pairToKeys[p] {
			// Return independent copies so callers can mutate CompsAboveCL/CompsAboveCost.
			cs := *sum
			out[k] = &cs
		}
	}
	return out, nil
}

// lookupConditionsBatch resolves CL conditions for a slice of cert numbers in one query.
// Returns a map keyed by cert number; missing certs are absent from the map.
func (s *CLSalesStore) lookupConditionsBatch(ctx context.Context, certs []string) (map[string]string, error) {
	out := make(map[string]string, len(certs))
	if len(certs) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT slab_serial, cl_condition FROM cl_card_mappings WHERE slab_serial = ANY($1::text[])`,
		certs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cert string
		var cond sql.NullString
		if err := rows.Scan(&cert, &cond); err != nil {
			return nil, err
		}
		out[cert] = cond.String
	}
	return out, rows.Err()
}

// platformBreakdownFromRows builds per-platform statistics from a slice of priceRow.
func platformBreakdownFromRows(rows []struct {
	priceCents int
	saleDate   string
	platform   string
}) []inventory.PlatformBreakdown {
	byPlat := make(map[string][]int)
	for _, r := range rows {
		byPlat[r.platform] = append(byPlat[r.platform], r.priceCents)
	}
	out := make([]inventory.PlatformBreakdown, 0, len(byPlat))
	for plat, prices := range byPlat {
		sorted := slices.Clone(prices)
		med := medianInt(sorted)
		high, low := prices[0], prices[0]
		for _, p := range prices {
			if p > high {
				high = p
			}
			if p < low {
				low = p
			}
		}
		out = append(out, inventory.PlatformBreakdown{
			Platform:    plat,
			SaleCount:   len(prices),
			MedianCents: med,
			HighCents:   high,
			LowCents:    low,
		})
	}
	return out
}
```

Note: if a helper named `platformBreakdownFromRows` already exists or if the existing single-key `GetCompSummary` has a near-identical helper, prefer re-using it and only create a new helper if the shapes diverge. Same for the `priceRow` struct — if the existing code uses a similar unnamed struct, inline the type in the batch method too; the goal is symmetry with the single-key path.

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/adapters/storage/postgres/ -run TestCLSalesStore_GetCompSummariesByKeys -v`
Expected: `--- PASS: TestCLSalesStore_GetCompSummariesByKeys`, `--- PASS: TestCLSalesStore_GetCompSummariesByKeys_EmptyInput`.

If the test errors out with "setupTestDB skipped" (no local Postgres), this is expected locally — the test runs green in a devcontainer with Postgres running. The CI workflow does not provide Postgres, so this test will be skipped there too; that matches the existing `campaign_store_test.go` behavior.

- [ ] **Step 5: Verify full build and lint**

Run: `go build ./... && golangci-lint run ./...`
Expected: no output, 0 issues.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/inventory/comp_summary.go \
        internal/adapters/storage/postgres/cl_sales_store.go \
        internal/adapters/storage/postgres/cl_sales_store_test.go
git commit -m "feat(inventory): add GetCompSummariesByKeys batch comp lookup

Collapses the per-variant N+1 (condition lookup + aggregate +
recent prices, x N unique keys) into three SQL queries total."
```

---

### Task 4: Use the batch method from `enrichCompSummaries`

**Files:**
- Modify: `internal/domain/inventory/comp_enrichment.go`

- [ ] **Step 1: Replace the body of `enrichCompSummaries` with batch call**

Replace all of `comp_enrichment.go:9-72` with:

```go
// enrichCompSummaries attaches CompSummary to aging items that have a gemRateID.
// Uses the batch CompSummaryProvider method so the whole set costs three SQL
// queries regardless of how many unique variants are in the inventory page.
func (s *service) enrichCompSummaries(ctx context.Context, items []AgingItem) {
	if s.compProv == nil {
		return
	}

	// compCacheKey groups purchases by card variant + grade so that different grades
	// of the same card get separate comp summaries.
	type compCacheKey struct {
		gemRateID  string
		gradeValue float64
	}

	// Collect unique (gemRateID, grade) pairs, picking one representative cert each.
	// The batch method resolves condition from that representative cert.
	seen := make(map[compCacheKey]string)
	for i := range items {
		p := &items[i].Purchase
		if p.GemRateID == "" {
			continue
		}
		key := compCacheKey{gemRateID: p.GemRateID, gradeValue: p.GradeValue}
		if _, ok := seen[key]; !ok {
			seen[key] = p.CertNumber
		}
	}
	if len(seen) == 0 {
		return
	}

	// Build the batch key list in a stable order to make logs reproducible.
	batchKeys := make([]CompKey, 0, len(seen))
	cacheKeyFor := make(map[CompKey]compCacheKey, len(seen))
	for k, cert := range seen {
		bk := CompKey{GemRateID: k.gemRateID, CertNumber: cert}
		batchKeys = append(batchKeys, bk)
		cacheKeyFor[bk] = k
	}

	results, err := s.compProv.GetCompSummariesByKeys(ctx, batchKeys)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "batch comp summary lookup failed", observability.Err(err))
		}
		return
	}

	// Re-index by cache key for O(1) attachment below.
	cache := make(map[compCacheKey]*CompSummary, len(results))
	for bk, summary := range results {
		if summary == nil {
			continue
		}
		cache[cacheKeyFor[bk]] = summary
	}

	// Attach to items — derive CompsAboveCL and CompsAboveCost per-purchase
	// since different purchases of the same card may have different CL values and costs.
	for i := range items {
		p := &items[i].Purchase
		key := compCacheKey{gemRateID: p.GemRateID, gradeValue: p.GradeValue}
		summary, ok := cache[key]
		if !ok {
			continue
		}
		cs := *summary
		cs.CompsAboveCL = CountAboveCost(summary.PriceCentsList, p.CLValueCents)
		cs.CompsAboveCost = CountAboveCost(summary.PriceCentsList, p.BuyCostCents)
		cs.PriceCentsList = nil
		items[i].CompSummary = &cs
	}
}
```

- [ ] **Step 2: Build and run all tests**

Run: `go build ./... && go test ./internal/domain/inventory/... && golangci-lint run ./...`
Expected: all green.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/inventory/comp_enrichment.go
git commit -m "perf(inventory): use batch comp-summary lookup in enrichCompSummaries

Drops inventory page DB time from ~3N round trips to a constant 3."
```

---

## Section B — Migration-on-boot safety

### Task 5: Create retry helper with tests

**Files:**
- Create: `internal/adapters/storage/postgres/retry.go`
- Create: `internal/adapters/storage/postgres/retry_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/storage/postgres/retry_test.go`:

```go
package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryWithBackoff_FirstAttemptSucceeds(t *testing.T) {
	calls := 0
	sleep := func(time.Duration) {}
	err := retryWithBackoff(context.Background(), retryConfig{
		MaxAttempts:  5,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        sleep,
	}, func(ctx context.Context) error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryWithBackoff_EventualSuccess(t *testing.T) {
	calls := 0
	sleeps := []time.Duration{}
	sleep := func(d time.Duration) { sleeps = append(sleeps, d) }
	target := errors.New("transient")
	err := retryWithBackoff(context.Background(), retryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        sleep,
	}, func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return target
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, sleeps)
}

func TestRetryWithBackoff_Exhausted(t *testing.T) {
	target := errors.New("always fails")
	err := retryWithBackoff(context.Background(), retryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        func(time.Duration) {},
	}, func(ctx context.Context) error {
		return target
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, target)
}

func TestRetryWithBackoff_CapsDelayAtMax(t *testing.T) {
	sleeps := []time.Duration{}
	_ = retryWithBackoff(context.Background(), retryConfig{
		MaxAttempts:  6,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     300 * time.Millisecond,
		Sleep:        func(d time.Duration) { sleeps = append(sleeps, d) },
	}, func(ctx context.Context) error {
		return errors.New("nope")
	})
	// attempts 1→2: 100ms, 2→3: 200ms, 3→4: 300ms (capped), 4→5: 300ms, 5→6: 300ms
	assert.Equal(t, []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		300 * time.Millisecond,
		300 * time.Millisecond,
	}, sleeps)
}

func TestRetryWithBackoff_RespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := retryWithBackoff(ctx, retryConfig{
		MaxAttempts:  5,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        func(time.Duration) {},
	}, func(ctx context.Context) error {
		calls++
		return errors.New("x")
	})
	require.Error(t, err)
	assert.LessOrEqual(t, calls, 1)
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/adapters/storage/postgres/ -run TestRetryWithBackoff -v`
Expected: compile error — `retryWithBackoff` / `retryConfig` undefined.

- [ ] **Step 3: Implement the retry helper**

Create `internal/adapters/storage/postgres/retry.go`:

```go
package postgres

import (
	"context"
	"time"
)

// retryConfig controls the exponential-backoff loop used by postgres.Open for
// its initial ping. Sleep is injectable for tests.
type retryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Sleep        func(time.Duration)
}

// retryWithBackoff calls fn up to cfg.MaxAttempts times, doubling the delay
// between attempts and capping at cfg.MaxDelay. It returns nil on the first
// successful call, or the last error after exhaustion. Honors ctx cancellation
// between attempts.
func retryWithBackoff(ctx context.Context, cfg retryConfig, fn func(context.Context) error) error {
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	delay := cfg.InitialDelay
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}
		if err := fn(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == cfg.MaxAttempts {
			break
		}
		sleep(delay)
		delay *= 2
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}
	return lastErr
}
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `go test ./internal/adapters/storage/postgres/ -run TestRetryWithBackoff -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/retry.go internal/adapters/storage/postgres/retry_test.go
git commit -m "feat(postgres): add retryWithBackoff helper

Exponential-backoff retry with injectable sleep, used by Open() to
ride out transient Supabase connectivity blips at startup."
```

---

### Task 6: Wire retry into `postgres.Open`

**Files:**
- Modify: `internal/adapters/storage/postgres/db.go`

- [ ] **Step 1: Wrap the Ping in retry**

Replace the block at `db.go:44-50` (the `PingContext` + error handling) with:

```go
	pingCfg := retryConfig{
		MaxAttempts:  10,
		InitialDelay: time.Second,
		MaxDelay:     30 * time.Second,
	}
	pingErr := retryWithBackoff(ctx, pingCfg, func(ctx context.Context) error {
		if err := db.PingContext(ctx); err != nil {
			logger.Warn(ctx, "database ping failed, will retry",
				observability.String("error", err.Error()))
			return err
		}
		return nil
	})
	if pingErr != nil {
		if closeErr := db.Close(); closeErr != nil {
			logger.Warn(ctx, "failed to close database after ping failure", observability.Err(closeErr))
		}
		return nil, apperrors.StorageError("ping database after retries", pingErr)
	}
```

- [ ] **Step 2: Build and test**

Run: `go build ./... && go test ./internal/adapters/storage/postgres/... && golangci-lint run ./...`
Expected: all green.

- [ ] **Step 3: Verify grace_period in fly.toml is sufficient**

Open `fly.toml`. Confirm `[[http_service.checks]]` has `grace_period = '30s'` or longer. If shorter, raise to `'60s'`.

The total retry budget is ~1+2+4+8+16+30+30+30+30 = ~151s worst case, but the DB is usually up within the first few attempts. `30s` grace is acceptable since Fly will still let the machine keep trying as long as it hasn't exceeded the boot timeout. If boots start getting killed, raise to `60s` in a follow-up.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/postgres/db.go
git commit -m "feat(postgres): retry initial ping with exponential backoff

Transient Supabase blips (DNS flicker, pooler restart) no longer
crashloop the Fly machine. Migrations themselves stay fail-fast."
```

---

## Section C — Baseline observability

### Task 7: Add `prometheus/client_golang` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dep**

Run: `go get github.com/prometheus/client_golang@latest`
Expected: `go: added github.com/prometheus/client_golang vX.Y.Z` (plus transitive deps).

- [ ] **Step 2: Run go mod tidy**

Run: `go mod tidy`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add prometheus/client_golang"
```

---

### Task 8: Build the `sql.DBStats` collector with tests

**Files:**
- Create: `internal/adapters/storage/postgres/metrics.go`
- Create: `internal/adapters/storage/postgres/metrics_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/storage/postgres/metrics_test.go`:

```go
package postgres

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNewDBStatsCollector_RegistersAndCollects(t *testing.T) {
	stats := sql.DBStats{
		MaxOpenConnections: 25,
		OpenConnections:    7,
		InUse:              3,
		Idle:               4,
		WaitCount:          11,
		MaxIdleClosed:      2,
		MaxLifetimeClosed:  1,
	}
	collector := NewDBStatsCollector("test", func() sql.DBStats { return stats })

	reg := prometheus.NewRegistry()
	err := reg.Register(collector)
	assert.NoError(t, err)

	// Expected names match our collector's FQ names.
	count, err := testutil.GatherAndCount(reg,
		"slabledger_db_max_open_connections",
		"slabledger_db_open_connections",
		"slabledger_db_in_use_connections",
		"slabledger_db_idle_connections",
		"slabledger_db_wait_count_total",
		"slabledger_db_max_idle_closed_total",
		"slabledger_db_max_lifetime_closed_total",
	)
	assert.NoError(t, err)
	assert.Equal(t, 7, count)

	// Spot-check one gauge value.
	mf, err := reg.Gather()
	assert.NoError(t, err)
	var found bool
	for _, f := range mf {
		if strings.HasSuffix(f.GetName(), "_in_use_connections") {
			assert.InDelta(t, 3.0, f.Metric[0].Gauge.GetValue(), 0.0001)
			found = true
		}
	}
	assert.True(t, found)
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./internal/adapters/storage/postgres/ -run TestNewDBStatsCollector -v`
Expected: compile error — `NewDBStatsCollector` undefined.

- [ ] **Step 3: Implement the collector**

Create `internal/adapters/storage/postgres/metrics.go`:

```go
package postgres

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

// DBStatsCollector is a prometheus.Collector that emits metrics derived from
// sql.DBStats on every scrape. Using a custom collector (vs. pre-registered
// gauges updated on a timer) means every scrape gets a consistent snapshot.
type DBStatsCollector struct {
	statsFn func() sql.DBStats

	maxOpen       *prometheus.Desc
	open          *prometheus.Desc
	inUse         *prometheus.Desc
	idle          *prometheus.Desc
	waitCount     *prometheus.Desc
	maxIdleClose  *prometheus.Desc
	maxLifeClose  *prometheus.Desc
}

// NewDBStatsCollector returns a collector that calls statsFn on every scrape.
// namespace is prepended to each metric name.
func NewDBStatsCollector(namespace string, statsFn func() sql.DBStats) *DBStatsCollector {
	ns := func(name string) string { return namespace + "_db_" + name }
	return &DBStatsCollector{
		statsFn:      statsFn,
		maxOpen:      prometheus.NewDesc(ns("max_open_connections"), "Maximum number of open connections to the database.", nil, nil),
		open:         prometheus.NewDesc(ns("open_connections"), "The number of established connections both in use and idle.", nil, nil),
		inUse:        prometheus.NewDesc(ns("in_use_connections"), "The number of connections currently in use.", nil, nil),
		idle:         prometheus.NewDesc(ns("idle_connections"), "The number of idle connections.", nil, nil),
		waitCount:    prometheus.NewDesc(ns("wait_count_total"), "The total number of connections waited for.", nil, nil),
		maxIdleClose: prometheus.NewDesc(ns("max_idle_closed_total"), "The total number of connections closed due to SetMaxIdleConns.", nil, nil),
		maxLifeClose: prometheus.NewDesc(ns("max_lifetime_closed_total"), "The total number of connections closed due to SetConnMaxLifetime.", nil, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *DBStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.maxOpen
	ch <- c.open
	ch <- c.inUse
	ch <- c.idle
	ch <- c.waitCount
	ch <- c.maxIdleClose
	ch <- c.maxLifeClose
}

// Collect implements prometheus.Collector.
func (c *DBStatsCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.statsFn()
	ch <- prometheus.MustNewConstMetric(c.maxOpen, prometheus.GaugeValue, float64(s.MaxOpenConnections))
	ch <- prometheus.MustNewConstMetric(c.open, prometheus.GaugeValue, float64(s.OpenConnections))
	ch <- prometheus.MustNewConstMetric(c.inUse, prometheus.GaugeValue, float64(s.InUse))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.Idle))
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(s.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.maxIdleClose, prometheus.CounterValue, float64(s.MaxIdleClosed))
	ch <- prometheus.MustNewConstMetric(c.maxLifeClose, prometheus.CounterValue, float64(s.MaxLifetimeClosed))
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/adapters/storage/postgres/ -run TestNewDBStatsCollector -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/metrics.go internal/adapters/storage/postgres/metrics_test.go
git commit -m "feat(postgres): DBStatsCollector for prometheus scrape

Custom collector so every scrape gets a live sql.DBStats snapshot.
Gauges for connection counts, counters for cumulative events."
```

---

### Task 9: Wire a metrics HTTP server in `main.go`

**Files:**
- Modify: `cmd/slabledger/main.go`

- [ ] **Step 1: Locate the DB-open block in main.go**

Run: `grep -n "postgres.Open\|NewDBTracker\|priceRepo" cmd/slabledger/main.go`
Expected: `160:	db, err := postgres.Open(...)`, `178:	priceRepo := postgres.NewDBTracker(db)`. Confirm that range exists as expected.

- [ ] **Step 2: Register collectors + start metrics server**

Insert this block at `cmd/slabledger/main.go` immediately after `defer func() { if err := db.Close(); ... }()` (around line 170, after the deferred db close and before `migrationsPath := cfg.Database.MigrationsPath`):

```go
	// Prometheus metrics server on :9091 — scraped by Fly's built-in Prometheus.
	// Separate port so app middleware (auth, rate limiter) doesn't interfere.
	metricsReg := prometheus.NewRegistry()
	metricsReg.MustRegister(collectors.NewGoCollector())
	metricsReg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsReg.MustRegister(postgres.NewDBStatsCollector("slabledger", db.Stats))

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(metricsReg, promhttp.HandlerOpts{Registry: metricsReg}))
	metricsSrv := &http.Server{
		Addr:              ":9091",
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info(ctx, "metrics server listening", observability.String("addr", metricsSrv.Addr))
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "metrics server error", observability.Err(err))
		}
	}()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsSrv.Shutdown(shutCtx); err != nil {
			logger.Warn(context.Background(), "metrics server shutdown error", observability.Err(err))
		}
	}()
```

- [ ] **Step 3: Add new imports to main.go**

Ensure these imports exist in `cmd/slabledger/main.go`:

```go
import (
	// ... existing imports ...
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)
```

Use `goimports` or manual edit to keep imports grouped by origin. If `errors` or `net/http` are already imported, keep them as-is.

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: no output.

- [ ] **Step 5: Run the app locally and verify `/metrics` responds**

Run in one terminal: `DATABASE_URL=postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable ./slabledger`
In another terminal: `curl -s http://localhost:9091/metrics | head -20`
Expected: Prometheus-format output containing lines starting with `go_` (e.g., `go_goroutines 23`) and `slabledger_db_` (e.g., `slabledger_db_in_use_connections 0`).

Stop the app (Ctrl-C).

- [ ] **Step 6: Commit**

```bash
git add cmd/slabledger/main.go
git commit -m "feat(observability): expose Go runtime + DB stats on :9091

Prometheus metrics endpoint on a separate port for Fly's scraper.
Registers Go runtime, process, and DBStats collectors."
```

---

### Task 10: Add `[metrics]` to fly.toml

**Files:**
- Modify: `fly.toml`

- [ ] **Step 1: Append the metrics block**

Add to the end of `fly.toml`:

```toml
# Prometheus metrics scrape target. Fly's built-in Prometheus scrapes this
# every 15s and publishes results in fly-metrics.net Grafana.
[metrics]
  port = 9091
  path = '/metrics'
```

- [ ] **Step 2: Commit**

```bash
git add fly.toml
git commit -m "chore(fly): configure metrics scrape on :9091"
```

---

## Final verification

### Task 11: Whole-tree check

- [ ] **Step 1: All checks green locally**

Run: `make check && go test -race -timeout 10m ./... && go build ./...`
Expected: all green.

- [ ] **Step 2: Push the branch, open a PR**

Run:
```bash
git push -u origin $(git branch --show-current)
gh pr create --title "phase 6a: app fitness on Fly/Supabase" --body "$(cat <<'EOF'
## Summary
- Inventory page batch comp-summary lookup (3 queries instead of ~3N)
- Exponential-backoff retry on initial Postgres ping — transient Supabase blips no longer crashloop
- Prometheus metrics on :9091 (Go runtime + sql.DBStats), scraped by Fly

Spec: docs/specs/2026-04-20-phase6a-app-fitness-on-fly-design.md
Plan: docs/plans/2026-04-20-phase6a-app-fitness-on-fly.md

## Test plan
- [ ] CI passes
- [ ] After merge + deploy: load /api/campaigns/{id}/inventory in prod, check fly-metrics.net for the edge p95 drop and for go_*/slabledger_db_* series populating
- [ ] Read fly logs for "inventory aging completed" lines — confirm compSummariesMs is small
EOF
)"
```

- [ ] **Step 3: Wait for CI + CodeRabbit, then merge**

Run: `gh pr checks <pr-number> --watch`
Once green: `gh pr merge <pr-number> --squash --delete-branch`

Fly dashboard auto-deploy (now enabled) will deploy to prod. Verify:

- `https://slabledger.dpao.la/api/health` → 200
- Load a campaign's inventory page, observe it feels faster
- fly-metrics.net → Go process dashboard populates
- Search fly-metrics.net for `slabledger_db_in_use_connections` — should have values

- [ ] **Step 4: Optional follow-up**

If the phase-timing logs reveal that compSummaries is no longer the hotspot and something else is (likely `applyOpenFlags` or `buildCrackCandidateSet`), open a follow-up issue but do not fix it in this phase — that is 6d material.
