# P1 — domain/inventory Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 14 silent-failure, data-integrity, and maintainability issues in `internal/domain/inventory/`.

**Architecture:** All fixes are contained within `internal/domain/inventory/`. No new interfaces or external dependencies. Fixes range from nil-guard additions and error propagation to a named constant replacement and documentation improvement.

**Tech Stack:** Go 1.26, table-driven tests with `go test -race`.

**Worktree:** `.worktrees/plan-p1-domain-inventory`

---

## Setup

```bash
git worktree add .worktrees/plan-p1-domain-inventory -b feature/polish-p1-domain-inventory
cd .worktrees/plan-p1-domain-inventory
```

Verify:
```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```
Expected: builds and all tests pass.

---

## Task 1: Nil guard — `service_import_psa.go:108-111` (HIGH)

**Problem:** When `itemResult.Status == "allocated"`, code accesses `campaign.ID` and `campaign.Name` at lines 108-111. But `campaign` was set to `nil` in the branch where `match.Status != "matched"`. `handleNewPSAPurchase` can apparently return `"allocated"` only when a match was found — but this is implicit, not enforced. Add an explicit nil guard.

**Files:**
- Modify: `internal/domain/inventory/service_import_psa.go:106-111`

- [ ] **Step 1: Read the switch block**

```go
// current code at service_import_psa.go:105-116
switch itemResult.Status {
case "allocated":
    result.Allocated++
    summary := result.ByCampaign[campaign.ID]
    summary.CampaignName = campaign.Name
    summary.Allocated++
    result.ByCampaign[campaign.ID] = summary
    // Cache newly created purchase...
    if created, err := s.purchases.GetPurchaseByCertNumber(ctx, "PSA", row.CertNumber); err == nil && created != nil {
        existingMap[row.CertNumber] = created
    }
```

- [ ] **Step 2: Add nil guard**

Replace the `case "allocated":` block so it reads:

```go
case "allocated":
    result.Allocated++
    if campaign == nil {
        if s.logger != nil {
            s.logger.Error(ctx, "allocated status with nil campaign — skipping ByCampaign update",
                observability.String("certNumber", row.CertNumber))
        }
        break
    }
    summary := result.ByCampaign[campaign.ID]
    summary.CampaignName = campaign.Name
    summary.Allocated++
    result.ByCampaign[campaign.ID] = summary
    // Cache newly created purchase so duplicate cert rows in the same batch
    // are handled as updates rather than allocation attempts.
    if created, err := s.purchases.GetPurchaseByCertNumber(ctx, "PSA", row.CertNumber); err == nil && created != nil {
        existingMap[row.CertNumber] = created
    }
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/service_import_psa.go
git commit -m "fix: add nil campaign guard in PSA import allocated branch"
```

---

## Task 2: Propagate `json.Unmarshal` error — `service_analytics.go:62-65` (MEDIUM)

**Problem:** `buildEnrichedSnapshot` silently discards `json.Unmarshal` errors at line 62-64, falling through to the column-based fallback with no log. Callers receive degraded data.

**Files:**
- Modify: `internal/domain/inventory/service_analytics.go:60-65`

- [ ] **Step 1: Read the current block**

```go
// service_analytics.go:59-65
if p.SnapshotJSON != "" {
    var snap MarketSnapshot
    if err := json.Unmarshal([]byte(p.SnapshotJSON), &snap); err == nil {
        return &snap
    }
}
```

- [ ] **Step 2: Add error logging**

```go
if p.SnapshotJSON != "" {
    var snap MarketSnapshot
    if err := json.Unmarshal([]byte(p.SnapshotJSON), &snap); err != nil {
        if s.logger != nil {
            s.logger.Warn(context.Background(), "snapshot JSON unmarshal failed — using column fallback",
                observability.String("purchaseID", p.ID),
                observability.Err(err))
        }
    } else {
        return &snap
    }
}
```

Note: `buildEnrichedSnapshot` has no `ctx` parameter. Use `context.Background()` for the log call. If you want to thread context through, that is an acceptable but larger refactor — keep it minimal here.

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/service_analytics.go
git commit -m "fix: log json.Unmarshal failure in buildEnrichedSnapshot instead of silently falling back"
```

---

## Task 3: Preserve underlying error in `applyOpenFlags` — `service_analytics.go:201-203,230-232` (MEDIUM)

**Problem:** `applyOpenFlags` failure is reported as a vague string `"Price flag data unavailable"`, discarding the underlying error. The caller's `result.Warnings` slice loses the real cause.

**Files:**
- Modify: `internal/domain/inventory/service_analytics.go:201-203` and `230-232`

- [ ] **Step 1: Locate both call sites**

```go
// Line 201-203
if err := s.applyOpenFlags(ctx, items); err != nil {
    result.Warnings = append(result.Warnings, "Price flag data unavailable")
}

// Line 230-232 (same pattern in GetGlobalInventoryAging)
if err := s.applyOpenFlags(ctx, items); err != nil {
    result.Warnings = append(result.Warnings, "Price flag data unavailable")
}
```

- [ ] **Step 2: Include error detail in warning message**

```go
// Line 201-203
if err := s.applyOpenFlags(ctx, items); err != nil {
    if s.logger != nil {
        s.logger.Warn(ctx, "applyOpenFlags failed", observability.Err(err))
    }
    result.Warnings = append(result.Warnings, "Price flag data unavailable")
}

// Line 230-232
if err := s.applyOpenFlags(ctx, items); err != nil {
    if s.logger != nil {
        s.logger.Warn(ctx, "applyOpenFlags failed", observability.Err(err))
    }
    result.Warnings = append(result.Warnings, "Price flag data unavailable")
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/service_analytics.go
git commit -m "fix: log applyOpenFlags error instead of silently producing vague warning string"
```

---

## Task 4: Log `GetCampaignPNL` skip in `service_portfolio.go:26-34` (MEDIUM)

**Problem:** Already fixed — the current code at lines 26-34 already logs the error. Verify the log call exists, then skip to next task.

**Files:**
- Read: `internal/domain/inventory/service_portfolio.go:24-35`

- [ ] **Step 1: Verify fix is in place**

```bash
grep -n "skipping campaign in portfolio health" internal/domain/inventory/service_portfolio.go
```
Expected output: line number matching `s.logger.Error(...)`.

If the log call is missing, add it:

```go
if err != nil {
    if s.logger != nil {
        s.logger.Error(ctx, "skipping campaign in portfolio health",
            observability.String("campaignID", c.ID),
            observability.String("campaignName", c.Name),
            observability.Err(err))
    }
    continue
}
```

No commit needed if already present.

---

## Task 5: Log `computeChannelHealthSignals` DB error — `service_portfolio.go:139-148` (MEDIUM)

**Problem:** Already fixed — the current code at line 141-147 already logs the error and returns `(0,0,0)`. The zero-return is documented behavior. Verify and move on.

- [ ] **Step 1: Verify**

```bash
grep -n "channel health signals" internal/domain/inventory/service_portfolio.go
```
Expected: finds the log call at the error site.

No commit needed if already present.

---

## Task 6: Return error from `processSnapshotsByStatus` — `service_snapshots.go:198-207` (MEDIUM)

**Problem:** `processSnapshotsByStatus` returns `(0,0,0)` on DB error (line 206), making quiescence and failure indistinguishable to the caller (`CaptureNewSnapshots`, `RetryFailedSnapshots`).

**Files:**
- Modify: `internal/domain/inventory/service_snapshots.go:198-207`

- [ ] **Step 1: Read the current signature and callers**

```go
// Current signature:
func (s *service) processSnapshotsByStatus(ctx context.Context, status SnapshotStatus, limit int) (processed, skipped, failed int)

// Callers:
func (s *service) CaptureNewSnapshots(ctx context.Context, limit int) (processed, skipped, failed int) {
    return s.processSnapshotsByStatus(ctx, SnapshotStatusNone, limit)
}
func (s *service) RetryFailedSnapshots(ctx context.Context, limit int) (processed, skipped, failed int) {
    return s.processSnapshotsByStatus(ctx, SnapshotStatusFailed, limit)
}
```

- [ ] **Step 2: Change signature to return error**

Change `processSnapshotsByStatus` signature:

```go
func (s *service) processSnapshotsByStatus(ctx context.Context, status SnapshotStatus, limit int) (processed, skipped, failed int, err error) {
    purchases, dbErr := s.purchases.ListSnapshotPurchasesByStatus(ctx, status, limit)
    if dbErr != nil {
        if s.logger != nil {
            s.logger.Error(ctx, "ListSnapshotPurchasesByStatus failed",
                observability.String("status", string(status)),
                observability.Err(dbErr))
        }
        return 0, 0, 0, dbErr
    }
    // ... rest of body unchanged, final return:
    return processed, skipped, failed, nil
}
```

- [ ] **Step 3: Update callers to propagate or log**

The public interface `CaptureNewSnapshots` and `RetryFailedSnapshots` must match their interface signatures. Check `internal/domain/inventory/repository.go` or `service.go` for the interface definition.

```bash
grep -n "CaptureNewSnapshots\|RetryFailedSnapshots" internal/domain/inventory/service.go internal/domain/inventory/repository.go 2>/dev/null | head -20
```

If the interface returns `(int, int, int)`, keep the public signature and just log the error internally:

```go
func (s *service) CaptureNewSnapshots(ctx context.Context, limit int) (processed, skipped, failed int) {
    p, sk, f, err := s.processSnapshotsByStatus(ctx, SnapshotStatusNone, limit)
    if err != nil {
        // error already logged inside processSnapshotsByStatus
    }
    return p, sk, f
}
```

This preserves the interface while making the internal error non-silent. The key improvement is `processSnapshotsByStatus` now returns an error that callers _can_ use if needed.

- [ ] **Step 4: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/service_snapshots.go
git commit -m "fix: return error from processSnapshotsByStatus instead of silent zero-return"
```

---

## Task 7: Log cache-update error after PSA allocation — `service_import_psa.go:113-116` (MEDIUM)

**Problem:** Lines 113-116 call `GetPurchaseByCertNumber` after allocation but only use the result on success, silently ignoring the error. If the cache misses, the same cert can be re-allocated.

**Files:**
- Modify: `internal/domain/inventory/service_import_psa.go:113-116`

- [ ] **Step 1: Add error logging**

```go
// Replace:
if created, err := s.purchases.GetPurchaseByCertNumber(ctx, "PSA", row.CertNumber); err == nil && created != nil {
    existingMap[row.CertNumber] = created
}

// With:
created, lookupErr := s.purchases.GetPurchaseByCertNumber(ctx, "PSA", row.CertNumber)
if lookupErr != nil {
    if s.logger != nil {
        s.logger.Warn(ctx, "post-allocation cache update failed — duplicate cert risk",
            observability.String("certNumber", row.CertNumber),
            observability.Err(lookupErr))
    }
} else if created != nil {
    existingMap[row.CertNumber] = created
}
```

- [ ] **Step 2: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/inventory/service_import_psa.go
git commit -m "fix: log cache update failure after PSA allocation to surface duplicate cert risk"
```

---

## Task 8: Log cache-update error in CL import — `service_import_cl.go:333-336` (MEDIUM)

**Problem:** Same pattern as Task 7 but in the CardLadder import path.

**Files:**
- Modify: `internal/domain/inventory/service_import_cl.go` (find the similar pattern)

- [ ] **Step 1: Find the pattern**

```bash
grep -n "GetPurchaseByCertNumber" internal/domain/inventory/service_import_cl.go
```

- [ ] **Step 2: Apply identical fix**

```go
// Find and replace the silent-error pattern:
created, lookupErr := s.purchases.GetPurchaseByCertNumber(ctx, certGrader, row.CertNumber)
if lookupErr != nil {
    if s.logger != nil {
        s.logger.Warn(ctx, "post-allocation cache update failed — duplicate cert risk",
            observability.String("certNumber", row.CertNumber),
            observability.Err(lookupErr))
    }
} else if created != nil {
    existingMap[row.CertNumber] = created
}
```

Replace `certGrader` with the appropriate grader string used in this file (e.g. `"CL"` or `"CGC"` — check what's used in the existing code).

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/service_import_cl.go
git commit -m "fix: log cache update failure after CL allocation to surface duplicate cert risk"
```

---

## Task 9: Handle `enrichSellSheetItem` bool return — `service_sell_sheet.go:155,247` (MEDIUM)

**Problem:** `enrichSellSheetItem` returns a `bool` (success flag) that is discarded at lines 155 and 247 with `item, _`. Zero-priced items are included in revenue totals.

**Files:**
- Modify: `internal/domain/inventory/service_sell_sheet.go:155` and `~247`

- [ ] **Step 1: Read the callers**

```go
// Line 155:
item, _ := s.enrichSellSheetItem(ctx, purchase, "", campaign.EbayFeePct, crackSet)
sheet.Totals.TotalExpectedRevenue += item.TargetSellPrice
```

- [ ] **Step 2: Use the bool to skip zero-priced items**

```go
item, ok := s.enrichSellSheetItem(ctx, purchase, "", campaign.EbayFeePct, crackSet)
if !ok {
    sheet.Totals.SkippedItems++
    continue
}
sheet.Totals.TotalExpectedRevenue += item.TargetSellPrice
```

Apply the same fix to the second call site (line 247 approximately — find it with `grep -n "enrichSellSheetItem" internal/domain/inventory/service_sell_sheet.go`).

- [ ] **Step 3: Verify `enrichSellSheetItem` signature**

```bash
grep -n "func.*enrichSellSheetItem" internal/domain/inventory/service_sell_sheet.go
```

Confirm it returns `(SellSheetItem, bool)`. If it returns `(SellSheetItem, error)`, use the error variant instead.

- [ ] **Step 4: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/service_sell_sheet.go
git commit -m "fix: use enrichSellSheetItem bool return to skip unpriced items in sell sheet totals"
```

---

## Task 10: Log price-provider errors in `crackCandidatesForCampaign` — `service_arbitrage.go:69-74` (MEDIUM)

**Problem:** Price lookup errors in `crackCandidatesForCampaign` are silently swallowed.

**Files:**
- Modify: `internal/domain/inventory/service_arbitrage.go:69-74`

- [ ] **Step 1: Find the error site**

```bash
grep -n "priceLookup\|PriceLookup\|err.*price\|price.*err" internal/domain/inventory/service_arbitrage.go | head -20
```

- [ ] **Step 2: Add error logging**

Find the block where price lookup error is ignored and add:

```go
if err != nil {
    if s.logger != nil {
        s.logger.Debug(ctx, "price lookup failed for crack candidate",
            observability.String("purchaseID", p.ID),
            observability.String("cardName", p.CardName),
            observability.Err(err))
    }
    continue
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/service_arbitrage.go
git commit -m "fix: log price lookup errors in crackCandidatesForCampaign"
```

---

## Task 11: Log price-provider errors in `GetAcquisitionTargets` — `service_arbitrage.go:284-295` (MEDIUM)

**Problem:** Same pattern as Task 10 but in `GetAcquisitionTargets`.

**Files:**
- Modify: `internal/domain/inventory/service_arbitrage.go:284-295`

- [ ] **Step 1: Find the error site**

```bash
grep -n "priceLookup\|err" internal/domain/inventory/service_arbitrage.go | grep -A2 "284\|285\|290\|295"
```

Or just search near line 284:
```bash
sed -n '280,300p' internal/domain/inventory/service_arbitrage.go
```

- [ ] **Step 2: Add error logging** (identical pattern to Task 10)

```go
if err != nil {
    if s.logger != nil {
        s.logger.Debug(ctx, "price lookup failed for acquisition target",
            observability.String("campaignID", campaignID),
            observability.String("cardName", p.CardName),
            observability.Err(err))
    }
    continue
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/service_arbitrage.go
git commit -m "fix: log price lookup errors in GetAcquisitionTargets"
```

---

## Task 12: Validate date format in weekly review — `service_portfolio.go:381-413` (MEDIUM)

**Problem:** Date comparison at lines 381-413 uses string lexicographic comparison (`pd >= thisWeekStr`). This works only if dates are `YYYY-MM-DD`. A malformed date silently mis-categorizes purchases.

**Files:**
- Modify: `internal/domain/inventory/service_portfolio.go` (near line 381)

- [ ] **Step 1: Add a helper that validates date format before comparison**

Add near the top of the `GetWeeklyReviewSummary` function body (or as a package-level helper):

```go
// isValidDate returns true if the date string matches YYYY-MM-DD format.
func isValidDate(s string) bool {
    _, err := time.Parse("2006-01-02", s)
    return err == nil
}
```

- [ ] **Step 2: Wrap the comparison**

In the loop that compares `pd` and `sd`:

```go
pd := d.Purchase.PurchaseDate
if !isValidDate(pd) {
    if s.logger != nil {
        s.logger.Warn(ctx, "invalid purchase date format — skipping weekly bucketing",
            observability.String("purchaseID", d.Purchase.ID),
            observability.String("date", pd))
    }
    continue
}
if pd >= thisWeekStr && pd <= thisWeekEndStr {
    // ...
```

Apply the same guard to the sale date `sd` comparison block.

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/service_portfolio.go
git commit -m "fix: validate date format before lexicographic comparison in weekly review bucketing"
```

---

## Task 13: Named constant for `grossModeFee` — `channel_fees.go:12` (LOW)

**Problem:** `grossModeFee = -1.0` is a magic sentinel. Add a comment or export if other packages need it, but the name is already descriptive. The item asks for a named constant like `GrossModeFeeDisabled`. The const is already named `grossModeFee` — the improvement is to add a clearer comment.

**Files:**
- Modify: `internal/domain/inventory/channel_fees.go:12`

- [ ] **Step 1: Read the current constant**

```go
// grossModeFee signals enrichSellSheetItem to skip fee deduction, returning gross prices.
const grossModeFee = -1.0
```

- [ ] **Step 2: Improve comment clarity**

```go
// grossModeFee is a sentinel value passed to enrichSellSheetItem to suppress fee
// deduction and return gross (pre-fee) prices. Any negative value would work but
// -1.0 is chosen to be clearly invalid as a real fee percentage.
const grossModeFee = -1.0
```

- [ ] **Step 3: Build**

```bash
go build ./internal/domain/inventory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/channel_fees.go
git commit -m "docs: clarify grossModeFee sentinel value comment in channel_fees.go"
```

---

## Task 14: Document CL refresh duplication — `service_import_cl.go:82-119,243-262` (LOW)

**Problem:** Two blocks in `service_import_cl.go` appear to duplicate the CL refresh/import logic. Either document why they diverge or extract a shared helper.

**Files:**
- Modify: `internal/domain/inventory/service_import_cl.go:82-119,243-262`

- [ ] **Step 1: Read both blocks**

```bash
sed -n '75,130p' internal/domain/inventory/service_import_cl.go
sed -n '235,270p' internal/domain/inventory/service_import_cl.go
```

- [ ] **Step 2: Assess divergence**

If the blocks are truly identical: extract a shared helper function:

```go
// fetchAndUpdateCLData fetches current CL price data for the given cert and updates the purchase.
// Returns the updated purchase or an error.
func (s *service) fetchAndUpdateCLData(ctx context.Context, certNumber string) (*Purchase, error) {
    // ... shared implementation ...
}
```

If they diverge intentionally: add a comment above each block:

```go
// Note: This block intentionally re-fetches CL data even if already present because
// the import path needs the most current pricing to determine allocation. See the
// update path at line 243 which uses cached data instead.
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/inventory/...
go test -race ./internal/domain/inventory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/inventory/service_import_cl.go
git commit -m "refactor: document or deduplicate CL refresh/import blocks in service_import_cl.go"
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

- [ ] **Final commit if needed**

```bash
git add -A
git status
# Only commit if there are uncommitted changes
```
