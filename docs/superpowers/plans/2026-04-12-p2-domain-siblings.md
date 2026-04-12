# P2 — domain/decomposed-siblings Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 15 silent-failure, data-integrity, and logic bugs across `internal/domain/arbitrage/`, `portfolio/`, `tuning/`, `finance/`, `export/`, `dhlisting/`, `csvimport/`, and `mmutil/`.

**Architecture:** All changes are within the named sub-packages. No new interfaces. The sell-sheet duplication fix refactors `export/service_sell_sheet.go` to delegate to the `inventory` package's existing `GenerateSellSheet`/`GenerateCampaignSellSheet` methods via the `InventoryService` interface — no changes to `inventory/service_sell_sheet.go`.

**Tech Stack:** Go 1.26, table-driven tests.

**Worktree:** `.worktrees/plan-p2-domain-siblings`

---

## Setup

```bash
git worktree add .worktrees/plan-p2-domain-siblings -b feature/polish-p2-domain-siblings
cd .worktrees/plan-p2-domain-siblings
go build ./internal/domain/...
go test -race ./internal/domain/...
```

Expected: builds and all tests pass.

---

## Task 1: Persist DH fields after listing — `dhlisting/dh_listing_service.go:193-200` (HIGH)

**Problem:** `UpdatePurchaseDHFields` failure at line 196-198 is logged with `Warn` but `listed` is not decremented. The DH status is listed in DH but not persisted locally — DB diverges from DH truth.

**Files:**
- Modify: `internal/domain/dhlisting/dh_listing_service.go:193-200`

- [ ] **Step 1: Read the current block**

```go
// ~line 193-200
if persistErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
    DHStatus:     inventory.DHStatusListed,
    ChannelsJSON: string(channelsJSON),
}); persistErr != nil {
    s.logger.Warn(ctx, "dh listing: failed to persist listed status",
        observability.String("cert", p.CertNumber), observability.Err(persistErr))
}
```

- [ ] **Step 2: Decrement listed counter on persist failure**

```go
if persistErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
    DHStatus:     inventory.DHStatusListed,
    ChannelsJSON: string(channelsJSON),
}); persistErr != nil {
    s.logger.Error(ctx, "dh listing: failed to persist listed status — decrementing listed count",
        observability.String("cert", p.CertNumber), observability.Err(persistErr))
    listed--
    continue
}
```

- [ ] **Step 3: Write a test**

In `internal/domain/dhlisting/dh_listing_service_test.go` (create if not exists), add:

```go
func TestListPurchases_PersistFailure_DecrementsListedCount(t *testing.T) {
    // Arrange: mock fieldsUpdater that returns error
    mockUpdater := &mockFieldsUpdater{
        updateErr: errors.New("db error"),
    }
    // ... setup service with mockUpdater ...
    // Act: call ListPurchases with a cert that will reach persist step
    // Assert: result.Listed == 0, not 1
}
```

Adapt to match the existing test structure in that file.

- [ ] **Step 4: Build and test**

```bash
go build ./internal/domain/dhlisting/...
go test -race ./internal/domain/dhlisting/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/dhlisting/
git commit -m "fix: decrement listed count when DH field persist fails in ListPurchases"
```

---

## Task 2: Persist DH ID in `inlineMatchAndPush` — `dhlisting/dh_listing_service.go:280-290` (HIGH)

**Problem:** At lines 280-290, `UpdatePurchaseDHFields` failure (persist the remote DH inventory ID) is only logged. Future runs re-call `inlineMatchAndPush`, creating duplicate DH inventory entries.

**Files:**
- Modify: `internal/domain/dhlisting/dh_listing_service.go:280-290`

- [ ] **Step 1: Read the current block**

```go
// ~lines 279-300
for _, r := range pushResp.Results {
    if r.Status == "failed" || r.DHInventoryID == 0 {
        continue
    }
    if s.fieldsUpdater != nil {
        if err := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
            ...
        }); err != nil {
            s.logger.Warn(ctx, "inline dh push: failed to persist DH fields", ...)
        }
    }
    ...
    return r.DHInventoryID
}
```

- [ ] **Step 2: Return 0 on persist failure**

```go
if s.fieldsUpdater != nil {
    if err := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
        CardID:            dhCardID,
        InventoryID:       r.DHInventoryID,
        CertStatus:        DHCertStatusMatched,
        ListingPriceCents: r.AssignedPriceCents,
        ChannelsJSON:      r.ChannelsJSON,
        DHStatus:          inventory.DHStatus(r.Status),
    }); err != nil {
        s.logger.Error(ctx, "inline dh push: failed to persist DH fields — returning 0 to prevent duplicate push",
            observability.String("cert", p.CertNumber), observability.Err(err))
        return 0
    }
}
```

This is a behavior change: if persist fails, we return 0 (which causes the caller to skip listing for this cert). On next run, `DHInventoryID` is still 0 so it retries. The push was already sent to DH — this may create an orphaned DH entry. Accept this trade-off vs. infinite duplicate creation.

Add a comment explaining the trade-off:

```go
// Note: returning 0 here means we'll retry next run and potentially create another DH entry.
// This is preferable to creating unlimited duplicates. The DH entry created above may need
// manual cleanup if the DB persist consistently fails.
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/dhlisting/...
go test -race ./internal/domain/dhlisting/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/dhlisting/dh_listing_service.go
git commit -m "fix: return 0 from inlineMatchAndPush when DH field persist fails to prevent duplicate DH entries"
```

---

## Task 3: Return error from `ListPurchases` on lookup failure — `dhlisting/dh_listing_service.go:111-119` (HIGH)

**Problem:** At lines 111-119, `ListPurchases` returns a zero-value `DHListingResult{}` on `GetPurchasesByCertNumbers` failure. Callers can't distinguish a real empty result from an error.

**Files:**
- Modify: `internal/domain/dhlisting/dh_listing_service.go:110-120`
- Possibly modify: `internal/domain/dhlisting/service.go` (interface definition)

- [ ] **Step 1: Check the interface signature**

```bash
grep -n "ListPurchases" internal/domain/dhlisting/service.go
```

If the interface returns `DHListingResult` (no error), changing the signature is a breaking change. In that case, the fix is to add an `Error` field to `DHListingResult`:

```go
type DHListingResult struct {
    Listed int
    Synced int
    Total  int
    Error  error // set when a fatal error prevented listing
}
```

And return:

```go
if err != nil {
    s.logger.Warn(ctx, "dh listing: batch cert lookup failed", observability.Err(err))
    return DHListingResult{Error: err}
}
```

If the interface already returns `(DHListingResult, error)`, simply return the error:

```go
return DHListingResult{}, err
```

- [ ] **Step 2: Update callers**

```bash
grep -rn "ListPurchases" internal/adapters/httpserver/ internal/adapters/scheduler/ | head -20
```

Update each caller to check the `Error` field (or returned error).

- [ ] **Step 3: Build and test**

```bash
go build ./...
go test -race ./internal/domain/dhlisting/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/dhlisting/
git commit -m "fix: surface lookup error from ListPurchases instead of returning zero-value result"
```

---

## Task 4: Fix bottom-performers slice logic — `portfolio/service.go:371-380` (HIGH)

**Problem:** Bottom performers slice is inconsistent:
- `>10 sales`: always picks last 5 (from sorted-by-profit-desc list, so 5 worst)
- `>5 sales`: picks from index 5 to end (variable count, could be up to 9)

This is inconsistent — the count varies based on total sale count.

**Files:**
- Modify: `internal/domain/portfolio/service.go:368-380`

- [ ] **Step 1: Read the current logic**

```go
sort.Slice(topSales, func(i, j int) bool {
    return topSales[i].ProfitCents > topSales[j].ProfitCents
})
if len(topSales) > 10 {
    summary.TopPerformers = topSales[:5]
    summary.BottomPerformers = topSales[len(topSales)-5:]
} else if len(topSales) > 5 {
    summary.TopPerformers = topSales[:5]
    summary.BottomPerformers = topSales[5:]
} else {
    summary.TopPerformers = topSales
    summary.BottomPerformers = nil
}
```

- [ ] **Step 2: Fix to be consistent**

```go
sort.Slice(topSales, func(i, j int) bool {
    return topSales[i].ProfitCents > topSales[j].ProfitCents
})

const maxPerformers = 5

if len(topSales) > 2*maxPerformers {
    summary.TopPerformers = topSales[:maxPerformers]
    summary.BottomPerformers = topSales[len(topSales)-maxPerformers:]
} else if len(topSales) > maxPerformers {
    summary.TopPerformers = topSales[:maxPerformers]
    summary.BottomPerformers = topSales[maxPerformers:]
} else {
    summary.TopPerformers = topSales
    summary.BottomPerformers = nil
}
```

- [ ] **Step 3: Write a test**

In the existing portfolio test file, add a table-driven case:

```go
{
    name:          "12 sales: top 5, bottom 5",
    salesCount:    12,
    wantTopLen:    5,
    wantBottomLen: 5,
},
{
    name:          "7 sales: top 5, bottom 2",
    salesCount:    7,
    wantTopLen:    5,
    wantBottomLen: 2,
},
{
    name:          "4 sales: all top, no bottom",
    salesCount:    4,
    wantTopLen:    4,
    wantBottomLen: 0,
},
```

- [ ] **Step 4: Build and test**

```bash
go build ./internal/domain/portfolio/...
go test -race ./internal/domain/portfolio/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/portfolio/service.go internal/domain/portfolio/service_test.go
git commit -m "fix: consistent bottom-performers slice count in GetWeeklyReviewSummary"
```

---

## Task 5: Eliminate sell-sheet duplication — `export/service_sell_sheet.go` (HIGH)

**Problem:** `export/service_sell_sheet.go` duplicates the sell-sheet logic from `inventory/service_sell_sheet.go`. The `export` package should delegate to the `inventory` package's implementation.

**Files:**
- Modify: `internal/domain/export/service_sell_sheet.go`
- Possibly modify: `internal/domain/export/service.go` (to add inventory service dependency)

- [ ] **Step 1: Check the export service struct**

```bash
grep -n "type service struct\|inventoryService\|InventoryService" internal/domain/export/service.go
```

If the export service already has an `InventoryService` field, use it. If not, add it.

- [ ] **Step 2: Check what export's sell sheet actually does**

```bash
wc -l internal/domain/export/service_sell_sheet.go
grep -n "func " internal/domain/export/service_sell_sheet.go
```

List all functions. For each one:
- If it's a method on `export.service` that calls `enrichSellSheetItem`, replace with a call to the inventory service's equivalent method.
- If it's a utility (pure function with no domain logic), it's OK to keep it.

- [ ] **Step 3: Delegate to inventory service**

For `GenerateSellSheet` in export:

```go
func (s *service) GenerateSellSheet(ctx context.Context, campaignID string) (*inventory.SellSheet, error) {
    return s.inventoryService.GenerateSellSheet(ctx, campaignID)
}
```

For `GenerateGlobalSellSheet`:

```go
func (s *service) GenerateGlobalSellSheet(ctx context.Context) (*inventory.SellSheet, error) {
    return s.inventoryService.GenerateGlobalSellSheet(ctx)
}
```

Do NOT modify `inventory/service_sell_sheet.go` (P1 scope).

- [ ] **Step 4: Remove duplicated `enrichSellSheetItem` from export**

After delegating all functions, remove the `enrichSellSheetItem` method from the export service (and any private helpers it used that are no longer needed).

- [ ] **Step 5: Update the constructor**

If you added `inventoryService` to the export service struct, update `NewService` to accept it:

```go
func NewService(inventoryService inventory.Service, ...) *service {
    return &service{inventoryService: inventoryService, ...}
}
```

Check callers of `export.NewService`:
```bash
grep -rn "export.NewService\|export\.NewService" internal/ cmd/ | head
```

- [ ] **Step 6: Build and test**

```bash
go build ./...
go test -race ./internal/domain/export/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/domain/export/
git commit -m "refactor: delegate export sell sheet to inventory service, remove duplication"
```

---

## Task 6: Fix PSA 8.5 half-grade filter — `arbitrage/service.go:82` (HIGH)

**Problem:** `if p.GradeValue > 8` at line 100 of `arbitrage/service.go` filters out cards with `GradeValue = 8.5`. PSA 8.5 cards trade closer to PSA 9 and should be included in crack analysis. The filter should be `>= 9` to match whole-grade PSA 9s and above.

Wait — actually re-read the intent. "Crack" analysis is for cards that could be cracked out of a PSA slab and raw cards would be submitted for regrading. A PSA 9 might crack to a 10 — that's the opportunity. Cards at grade 8 or below are the target. PSA 8.5 half-grades trade like PSA 9 not PSA 8, so they should be excluded from crack candidates (like 9s).

The fix: change `> 8` to `>= 9`:

**Files:**
- Modify: `internal/domain/arbitrage/service.go:100`

- [ ] **Step 1: Confirm the filter semantics**

```bash
grep -n "GradeValue > 8\|GradeValue >= 9\|crack" internal/domain/arbitrage/service.go | head
```

- [ ] **Step 2: Change the filter**

```go
// Before:
if p.GradeValue > 8 {
    continue
}

// After:
// Skip PSA 9+ and half-grades that trade at PSA 9 value (8.5).
// Crack analysis targets PSA 8 and below only.
if p.GradeValue >= 9 {
    continue
}
```

- [ ] **Step 3: Write a test**

```go
func TestCrackCandidates_ExcludesPSA9(t *testing.T) {
    // purchase with GradeValue = 9 should not appear in results
}
func TestCrackCandidates_ExcludesPSA8Point5(t *testing.T) {
    // purchase with GradeValue = 8.5 should not appear in results
}
func TestCrackCandidates_IncludesPSA8(t *testing.T) {
    // purchase with GradeValue = 8 should appear in results
}
```

- [ ] **Step 4: Build and test**

```bash
go build ./internal/domain/arbitrage/...
go test -race ./internal/domain/arbitrage/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/arbitrage/service.go internal/domain/arbitrage/service_test.go
git commit -m "fix: exclude PSA 8.5 half-grades from crack candidates (use >= 9 filter)"
```

---

## Task 7: Use campaign-specific fee in EV calculation — `arbitrage/expected_value.go:60-65` (HIGH)

**Problem:** `ComputeExpectedValue` accepts optional `feePctOpts ...float64` but callers in `GetAcquisitionTargets` pass no fee, defaulting to `DefaultMarketplaceFeePct = 0.1235`. The campaign's actual `EbayFeePct` is ignored.

**Files:**
- Modify: `internal/domain/arbitrage/service.go` (the `GetAcquisitionTargets` call site)

- [ ] **Step 1: Find the EV call site**

```bash
grep -n "ComputeExpectedValue" internal/domain/arbitrage/service.go
```

- [ ] **Step 2: Pass campaign fee to ComputeExpectedValue**

The campaign object is available in the function. Pass `campaign.EbayFeePct`:

```go
// Before:
ev := ComputeExpectedValue(...)

// After:
feePct := campaign.EbayFeePct
if feePct == 0 {
    feePct = DefaultMarketplaceFeePct
}
ev := ComputeExpectedValue(..., feePct)
```

Check the `ComputeExpectedValue` function signature in `expected_value.go`:
```bash
grep -n "func ComputeExpectedValue" internal/domain/arbitrage/expected_value.go
```

Confirm the variadic `feePctOpts` is the last parameter and pass the fee there.

- [ ] **Step 3: Write a test**

```go
func TestComputeExpectedValue_UsesCampaignFee(t *testing.T) {
    // With a campaign that has EbayFeePct = 0.15 (higher than default 0.1235),
    // the EV should be lower than with the default fee.
    evDefault := ComputeExpectedValue(/* ... */) // no fee arg
    evHighFee := ComputeExpectedValue(/* ... */, 0.15)
    if evHighFee >= evDefault {
        t.Errorf("expected high fee to reduce EV: default=%d high=%d", evDefault.EV, evHighFee.EV)
    }
}
```

- [ ] **Step 4: Build and test**

```bash
go build ./internal/domain/arbitrage/...
go test -race ./internal/domain/arbitrage/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/arbitrage/service.go internal/domain/arbitrage/expected_value_test.go
git commit -m "fix: pass campaign EbayFeePct to ComputeExpectedValue instead of using hardcoded default"
```

---

## Task 8: Per-card cost in Monte Carlo simulation — `arbitrage/montecarlo.go:105-131` (HIGH)

**Problem:** Monte Carlo simulation uses `avgCost` (average cost across all cards) for each card instead of per-card cost. Cards with wildly different costs get the same cost basis in simulation.

**Files:**
- Modify: `internal/domain/arbitrage/montecarlo.go:105-131`

- [ ] **Step 1: Read the simulation inner loop**

```bash
sed -n '95,140p' internal/domain/arbitrage/montecarlo.go
```

- [ ] **Step 2: Understand the current structure**

Look for where `avgCost` or `costBasis` is used in the per-card loop. The fix is to use the individual card's cost from the input data.

- [ ] **Step 3: Check the input type**

```bash
grep -n "func.*Monte\|MonteCarloInput\|type.*Carlo" internal/domain/arbitrage/montecarlo.go | head
```

Check if the input includes per-card cost fields.

- [ ] **Step 4: Use per-card cost**

If each card item has a `CostBasisCents` or `BuyCostCents` field:

```go
// Before: using avgCost for all cards
cardCost := avgCost

// After: use per-card cost, fall back to avg if zero
cardCost := item.CostBasisCents
if cardCost == 0 {
    cardCost = avgCost
}
```

- [ ] **Step 5: Write a regression test**

```go
func TestMonteCarloSimulation_UsesPerCardCost(t *testing.T) {
    // Two cards: cheap ($5) and expensive ($100)
    // With avgCost both get $52.50 — incorrect
    // With per-card cost each gets its actual cost
    // Verify that EV differs between the two
}
```

- [ ] **Step 6: Build and test**

```bash
go build ./internal/domain/arbitrage/...
go test -race ./internal/domain/arbitrage/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/domain/arbitrage/montecarlo.go internal/domain/arbitrage/montecarlo_test.go
git commit -m "fix: use per-card cost in Monte Carlo simulation instead of avg cost for all cards"
```

---

## Task 9: Log silent drops in `GetCrackOpportunities` — `arbitrage/service.go:143-151` (MEDIUM)

**Problem:** Per-campaign crack failure silently drops the campaign from `GetCrackOpportunities` with only a continue.

**Files:**
- Modify: `internal/domain/arbitrage/service.go:143-151`

- [ ] **Step 1: Find the drop site**

```bash
grep -n "crackCandidates\|GetCrackOpportunities\|continue" internal/domain/arbitrage/service.go | head -20
```

- [ ] **Step 2: Add log on continue**

```go
// Before:
if err != nil {
    continue
}

// After:
if err != nil {
    if s.logger != nil {
        s.logger.Warn(ctx, "GetCrackOpportunities: skipping campaign",
            observability.String("campaignID", c.ID),
            observability.Err(err))
    }
    continue
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/arbitrage/...
go test -race ./internal/domain/arbitrage/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/arbitrage/service.go
git commit -m "fix: log per-campaign crack failure in GetCrackOpportunities instead of silent drop"
```

---

## Task 10: Log silent drops in `GetAcquisitionTargets` — `arbitrage/service.go:279-287` (MEDIUM)

**Problem:** Per-campaign DB failure silently drops campaign from `GetAcquisitionTargets`.

**Files:**
- Modify: `internal/domain/arbitrage/service.go:279-287`

- [ ] **Step 1: Find the drop site**

```bash
sed -n '275,295p' internal/domain/arbitrage/service.go
```

- [ ] **Step 2: Add log**

```go
if err != nil {
    if s.logger != nil {
        s.logger.Warn(ctx, "GetAcquisitionTargets: skipping campaign",
            observability.String("campaignID", c.ID),
            observability.Err(err))
    }
    continue
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/arbitrage/...
go test -race ./internal/domain/arbitrage/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/arbitrage/service.go
git commit -m "fix: log per-campaign failure in GetAcquisitionTargets instead of silent drop"
```

---

## Task 11: Handle `enrichSellSheetItem` bool in export — `export/service_sell_sheet.go:158,250` (MEDIUM)

**Note:** If Task 5 (sell-sheet duplication removal) was implemented and the export package now delegates to inventory, this task may already be resolved. Check first.

```bash
grep -n "enrichSellSheetItem" internal/domain/export/service_sell_sheet.go
```

If still present:

**Files:**
- Modify: `internal/domain/export/service_sell_sheet.go:158` and `~250`

- [ ] **Step 1: Apply the same fix as P1 Task 9**

```go
item, ok := s.enrichSellSheetItem(ctx, purchase, "", campaign.EbayFeePct, crackSet)
if !ok {
    sheet.Totals.SkippedItems++
    continue
}
```

- [ ] **Step 2: Build and test**

```bash
go build ./internal/domain/export/...
go test -race ./internal/domain/export/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/export/service_sell_sheet.go
git commit -m "fix: handle enrichSellSheetItem bool return in export package"
```

---

## Task 12: Log `SaveExternalID` failure — `dhlisting/dh_listing_service.go:246-252` (MEDIUM)

**Problem:** `SaveExternalID` failure at lines 246-252 is logged but execution continues. The issue is that repeated failures cause repeated cert-resolver roundtrips on every listing attempt.

The fix is to not just log but track the failure more visibly, so operators know to investigate.

**Files:**
- Modify: `internal/domain/dhlisting/dh_listing_service.go:246-252`

- [ ] **Step 1: Upgrade log level from Warn to Error**

```go
if err := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, SourceDH, externalID); err != nil {
    s.logger.Error(ctx, "inline dh resolve: failed to save card mapping — cert resolver will be called again next run",
        observability.String("cert", p.CertNumber),
        observability.String("cardName", p.CardName),
        observability.Err(err))
}
```

- [ ] **Step 2: Build and test**

```bash
go build ./internal/domain/dhlisting/...
go test -race ./internal/domain/dhlisting/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/dhlisting/dh_listing_service.go
git commit -m "fix: upgrade SaveExternalID failure log to Error level in inlineMatchAndPush"
```

---

## Task 13: Fix archived campaign asymmetry — `portfolio/service.go:49,55` (MEDIUM)

**Problem:** `GetPortfolioHealth` loads all campaigns including archived (`ListCampaigns(ctx, false)`) but then loads purchases excluding archived (`WithExcludeArchived()`). Archived campaigns get a zero channel health score (not a bad score — just incorrect).

**Files:**
- Modify: `internal/domain/portfolio/service.go:49,55`

- [ ] **Step 1: Read the current load pattern**

```go
// Line 49
allCampaigns, err := s.campaigns.ListCampaigns(ctx, false)
// Line 55
allData, err := s.analytics.GetAllPurchasesWithSales(ctx, inventory.WithExcludeArchived())
```

- [ ] **Step 2: Align by excluding archived campaigns too**

```go
// Option A: exclude archived from campaign list too
allCampaigns, err := s.campaigns.ListCampaigns(ctx, true) // true = include archived? check signature

// Confirm the signature:
```

```bash
grep -n "func.*ListCampaigns" internal/domain/inventory/repository.go
```

If `ListCampaigns(ctx, includeArchived bool)`:
- `false` = exclude archived → use this
- The asymmetry would then be: why are we loading archived campaigns at all?

**Option B:** Keep loading archived campaigns (for full portfolio visibility) but also load their purchases:

```go
allData, err := s.analytics.GetAllPurchasesWithSales(ctx) // no WithExcludeArchived
```

Choose the option that matches the business intent. The spec says "asymmetry causes zero channel health for archived" — so the right fix is to either exclude archived campaigns entirely OR include their purchases too.

Recommended: exclude archived from campaign list (they shouldn't affect portfolio health):

```go
allCampaigns, err := s.campaigns.ListCampaigns(ctx, false) // false = exclude archived
```

If `false` already excludes archived, check what the second param does:

```bash
grep -n "ListCampaigns" internal/adapters/storage/sqlite/campaign_store.go | head
```

- [ ] **Step 3: Write a test covering archived campaign behavior**

Add a test case that verifies archived campaigns are not counted in portfolio health.

- [ ] **Step 4: Build and test**

```bash
go build ./internal/domain/portfolio/...
go test -race ./internal/domain/portfolio/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/portfolio/service.go internal/domain/portfolio/service_test.go
git commit -m "fix: align campaign and purchase loading for archived campaigns in GetPortfolioHealth"
```

---

## Task 14: Fix leftmost-match vs. ordered-registry contradiction — `csvimport/import_parsing_metadata.go:169-182` (MEDIUM)

**Problem:** The registry is built ordered "longest-first" to prefer more specific matches, but the lookup uses `strings.Contains` (leftmost match), which contradicts the ordering intent.

**Files:**
- Modify: `internal/domain/csvimport/import_parsing_metadata.go:169-182`

- [ ] **Step 1: Read the lookup code**

```bash
sed -n '160,190p' internal/domain/csvimport/import_parsing_metadata.go
```

- [ ] **Step 2: Understand the registry structure**

```bash
grep -n "registry\|longest\|ordered\|type.*Registry" internal/domain/csvimport/import_parsing_metadata.go | head -20
```

- [ ] **Step 3: Fix to use longest-first ordering correctly**

If the registry is a `[]struct{pattern, value}` sorted by `len(pattern)` descending, the lookup should iterate in order and return the first match:

```go
// Correct longest-first implementation:
for _, entry := range registry { // registry is sorted longest-first
    if strings.Contains(input, entry.Pattern) {
        return entry.Value
    }
}
return defaultValue
```

If the current code scans differently (e.g., uses a map), switch to a slice with sorted iteration.

- [ ] **Step 4: Write a test that validates longest-match wins**

```go
func TestMetadataParsing_LongestMatchWins(t *testing.T) {
    // "Scarlet & Violet—Twilight Masquerade" should match "Twilight Masquerade"
    // not just "Scarlet & Violet"
    result := parseSetName("Scarlet & Violet—Twilight Masquerade")
    if result != "Twilight Masquerade" {
        t.Errorf("expected longest match, got %q", result)
    }
}
```

- [ ] **Step 5: Build and test**

```bash
go build ./internal/domain/csvimport/...
go test -race ./internal/domain/csvimport/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/domain/csvimport/import_parsing_metadata.go internal/domain/csvimport/import_parsing_metadata_test.go
git commit -m "fix: use longest-first ordered match in CSV metadata parsing registry"
```

---

## Final Verification

- [ ] **Run full test suite**

```bash
go test -race -timeout 10m ./...
```
Expected: all packages pass.

- [ ] **Run quality checks**

```bash
make check
```
Expected: no lint errors, no architecture violations, no files over 500 lines.
