# P4 — domain/small+testutil Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use **superpowers:subagent-driven-development** to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. See Setup section below for worktree creation.

**Goal:** Fix 9 issues in `internal/domain/favorites/`, `picks/`, `cards/`, `auth/`, `pricing/`, `constants/`, `storage/`, and `internal/testutil/` — including a README rewrite, in-memory store correctness, and inline mock replacement.

**Architecture:** Most changes are in `internal/testutil/`. The `storage/doc.go` deletion and `pricing/repository.go` field removal are minor cleanup. Mock replacements in `picks/` and `favorites/` test files require adding canonical mocks to `testutil/mocks/` (P3 adds advisor/social mocks; this plan adds favorites/picks mocks).

**Tech Stack:** Go 1.26.

**Worktree:** `.worktrees/plan-p4-domain-small`

---

## Setup

```bash
# Create worktree from the main repo root (not from within another worktree)
git -C /workspace worktree add /workspace/.worktrees/plan-p4-domain-small -b feature/polish-p4-domain-small
cd /workspace/.worktrees/plan-p4-domain-small
```

---

## Task 1: Rewrite `testutil/mocks/README.md` (HIGH)

**Problem:** README examples reference the deleted `campaigns` package with stale code patterns. The entire README needs to reflect the current architecture.

**Files:**
- Modify: `internal/testutil/mocks/README.md`

- [ ] **Step 1: Read the current README**

```bash
cat internal/testutil/mocks/README.md
```

- [ ] **Step 2: List currently available mocks**

```bash
ls internal/testutil/mocks/*.go | grep -v _test.go
```

- [ ] **Step 3: Write a new README**

The README must cover:

1. **Overview**: What mocks are, the Fn-field pattern, when to use mocks vs. InMemoryStore.
2. **Fn-field pattern** with a complete working example using a real mock from the codebase:
   ```go
   // Example: override CampaignRepositoryMock.CreateCampaign for a specific test
   mock := &mocks.CampaignRepositoryMock{}
   mock.CreateCampaignFn = func(ctx context.Context, c *inventory.Campaign) error {
       return inventory.ErrCampaignNotFound // simulate error
   }
   ```
3. **InMemoryStore usage**: When to use `mocks.NewInMemoryCampaignStore()` vs. individual repo mocks.
4. **Service mocks**: List of service mocks (`MockArbitrageService`, `MockPortfolioService`, `MockTuningService`, etc.) and when to use them.
5. **Adding new mocks**: Step-by-step guide for adding a new mock type.
6. **Scope note**: Explain that the InMemoryStore uses direct state mutation (not Fn-fields) for simple cases — see Task 3.

Use real type names from the current codebase. Run `grep -rn "type Mock\|type.*Mock\|MockArbitrage\|CampaignRepositoryMock" internal/testutil/mocks/` to get the complete list.

- [ ] **Step 4: Commit**

```bash
git add internal/testutil/mocks/README.md
git commit -m "docs: rewrite testutil/mocks/README.md to reflect current architecture and real examples"
```

---

## Task 2: Fix `GetAllPurchasesWithSales` in `inmemory_campaign_store.go` (HIGH)

**Problem:** `GetAllPurchasesWithSales` ignores store state and always returns `[]inventory.PurchaseWithSales{}`.

**Files:**
- Modify: `internal/testutil/inmemory_campaign_store.go`

- [ ] **Step 1: Find the current implementation**

```bash
grep -n "GetAllPurchasesWithSales" internal/testutil/inmemory_campaign_store.go
```

- [ ] **Step 2: Read the current stub**

```bash
sed -n '<found_line-2>,<found_line+5>p' internal/testutil/inmemory_campaign_store.go
```

It likely looks like:
```go
func (s *InMemoryCampaignStore) GetAllPurchasesWithSales(ctx context.Context, opts ...inventory.QueryOption) ([]inventory.PurchaseWithSales, error) {
    return []inventory.PurchaseWithSales{}, nil
}
```

- [ ] **Step 3: Implement using store state**

```go
func (s *InMemoryCampaignStore) GetAllPurchasesWithSales(ctx context.Context, opts ...inventory.QueryOption) ([]inventory.PurchaseWithSales, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    qo := inventory.ApplyQueryOptions(opts...)
    var result []inventory.PurchaseWithSales
    for _, p := range s.purchases {
        if qo.ExcludeArchived {
            // check if the campaign is archived
            c, ok := s.campaigns[p.CampaignID]
            if ok && c.Archived {
                continue
            }
        }
        // Find matching sale (if any)
        var sale *inventory.Sale
        for _, sl := range s.sales {
            if sl.PurchaseID == p.ID {
                saleCopy := sl
                sale = &saleCopy
                break
            }
        }
        purchaseCopy := p
        result = append(result, inventory.PurchaseWithSales{
            Purchase: purchaseCopy,
            Sale:     sale,
        })
    }
    return result, nil
}
```

Adapt field names to match actual types. Check `inventory.QueryOption` and `ApplyQueryOptions`:

```bash
grep -n "ApplyQueryOptions\|type QueryOption\|ExcludeArchived" internal/domain/inventory/repository.go | head -20
```

- [ ] **Step 4: Write a test**

```go
func TestInMemoryStore_GetAllPurchasesWithSales_ReturnsData(t *testing.T) {
    store := mocks.NewInMemoryCampaignStore()
    // Add a campaign
    // Add a purchase
    // Add a sale for that purchase
    // Call GetAllPurchasesWithSales
    // Verify the result contains the purchase+sale pair
}
```

- [ ] **Step 5: Build and test**

```bash
go build ./internal/testutil/...
go test -race ./internal/testutil/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/testutil/inmemory_campaign_store.go internal/testutil/inmemory_campaign_store_test.go
git commit -m "fix: implement GetAllPurchasesWithSales in InMemoryCampaignStore to return actual store data"
```

---

## Task 3: Document InMemoryStore Fn-field omission — `inmemory_campaign_store.go` (HIGH)

**Problem:** 37 methods in InMemoryCampaignStore lack Fn-field override support. Either add them or document the deliberate omission.

**Files:**
- Modify: `internal/testutil/inmemory_campaign_store.go`
- Modify: `internal/testutil/mocks/README.md` (add note)

- [ ] **Step 1: Count the methods**

```bash
grep -c "^func (s \*InMemoryCampaignStore)" internal/testutil/inmemory_campaign_store.go
```

- [ ] **Step 2: Decide the approach**

Adding 37 Fn-fields is valid but mechanical. The acceptable alternative per the spec is a comment block documenting the design decision:

Add a comment at the top of the struct definition:

```go
// InMemoryCampaignStore is an in-memory implementation of the campaign store interfaces
// for use in unit tests. It uses direct state mutation (add to s.purchases, s.sales, etc.)
// rather than the Fn-field override pattern used in mocks.CampaignRepositoryMock.
//
// Use InMemoryCampaignStore when you need realistic state-based behavior across multiple
// calls (e.g., testing a service that creates then reads data).
//
// Use mocks.CampaignRepositoryMock when you need to control individual method responses
// per test case (Fn-field overrides).
//
// Methods do NOT have Fn-field overrides by design — if you need per-method control,
// use CampaignRepositoryMock instead.
type InMemoryCampaignStore struct {
    ...
}
```

- [ ] **Step 3: Update README**

Reference this decision in `testutil/mocks/README.md` (the README updated in Task 1).

- [ ] **Step 4: Commit**

```bash
git add internal/testutil/inmemory_campaign_store.go internal/testutil/mocks/README.md
git commit -m "docs: document InMemoryCampaignStore design decision to omit Fn-field overrides"
```

---

## Task 4: Verify `GetRevocationFlagByID` in `testutil/inventory_finance_repo.go` (MEDIUM)

**Problem:** `testutil/inventory_finance_repo.go` may have a `GetRevocationFlagByID` method that doesn't match the current `FinanceRepository` interface.

**Files:**
- Modify: `internal/testutil/inventory_finance_repo.go` (if needed)

- [ ] **Step 1: Check the interface**

```bash
grep -n "GetRevocationFlagByID" internal/domain/finance/repository.go internal/domain/inventory/repository.go 2>/dev/null
```

- [ ] **Step 2: Check the testutil implementation**

```bash
grep -n "GetRevocationFlagByID" internal/testutil/inventory_finance_repo.go 2>/dev/null
```

- [ ] **Step 3: Fix signature mismatch if found**

If the testutil method signature doesn't match the interface:
```go
// Interface:
GetRevocationFlagByID(ctx context.Context, id string) (*inventory.RevocationFlag, error)

// Fix testutil to match:
func (s *InMemoryFinanceRepo) GetRevocationFlagByID(ctx context.Context, id string) (*inventory.RevocationFlag, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    for _, f := range s.flags {
        if f.ID == id {
            copy := f
            return &copy, nil
        }
    }
    return nil, nil
}
```

- [ ] **Step 4: Build**

```bash
go build ./internal/testutil/...
```

- [ ] **Step 5: Commit if changed**

```bash
git add internal/testutil/inventory_finance_repo.go
git commit -m "fix: align GetRevocationFlagByID in testutil with current FinanceRepository interface"
```

---

## Task 5: Fix non-deterministic iteration in InMemoryStore (MEDIUM)

**Problem:** `ListPurchasesByCampaign`/`ListSalesByCampaign` iterate map keys non-deterministically, causing flaky tests.

**Files:**
- Modify: `internal/testutil/inmemory_campaign_store.go`

- [ ] **Step 1: Find the functions**

```bash
grep -n "ListPurchasesByCampaign\|ListSalesByCampaign" internal/testutil/inmemory_campaign_store.go
```

- [ ] **Step 2: Add sorted key iteration**

```go
// Before (non-deterministic):
for id, p := range s.purchases {
    if p.CampaignID == campaignID {
        result = append(result, p)
    }
}

// After (deterministic):
// Collect matching IDs first, sort, then retrieve
var ids []string
for id, p := range s.purchases {
    if p.CampaignID == campaignID {
        ids = append(ids, id)
    }
}
sort.Strings(ids)
for _, id := range ids {
    result = append(result, s.purchases[id])
}
```

Apply to both `ListPurchasesByCampaign` and `ListSalesByCampaign`.

- [ ] **Step 3: Build and test**

```bash
go test -race ./internal/testutil/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/testutil/inmemory_campaign_store.go
git commit -m "fix: sort keys in InMemoryStore list methods to ensure deterministic test behavior"
```

---

## Task 6: Replace inline mocks in `picks/service_test.go` (MEDIUM)

**Problem:** `domain/picks/service_test.go:11-123` has 113 lines of inline mock infrastructure.

**Files:**
- Modify: `internal/domain/picks/service_test.go`
- Create: `internal/testutil/mocks/picks_mocks.go` (if not exists)

- [ ] **Step 1: Check for existing canonical mock**

```bash
ls internal/testutil/mocks/ | grep -i pick
```

- [ ] **Step 2: Find the picks repository interface**

```bash
grep -n "type.*Repository\|interface" internal/domain/picks/repository.go 2>/dev/null | head
```

- [ ] **Step 3: Create canonical mock if needed**

```go
package mocks

import (
    "context"
    "github.com/guarzo/slabledger/internal/domain/picks"
)

// PicksRepositoryMock is a test double for picks.Repository.
type PicksRepositoryMock struct {
    GetPickFn    func(ctx context.Context, id string) (*picks.Pick, error)
    SavePickFn   func(ctx context.Context, p *picks.Pick) error
    ListPicksFn  func(ctx context.Context) ([]picks.Pick, error)
    DeletePickFn func(ctx context.Context, id string) error
}

func (m *PicksRepositoryMock) GetPick(ctx context.Context, id string) (*picks.Pick, error) {
    if m.GetPickFn != nil {
        return m.GetPickFn(ctx, id)
    }
    return nil, nil
}
// ... implement all interface methods
```

Check actual interface methods before implementing.

- [ ] **Step 4: Replace inline mock in service_test.go**

- [ ] **Step 5: Build and test**

```bash
go build ./internal/domain/picks/... ./internal/testutil/...
go test -race ./internal/domain/picks/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/domain/picks/service_test.go internal/testutil/mocks/
git commit -m "refactor: replace inline mock in picks/service_test.go with canonical mock"
```

---

## Task 7: Replace inline mock in `favorites/service_test.go` (MEDIUM)

**Problem:** `domain/favorites/service_test.go:11-163` has 180 lines of inline stateful mock repository.

**Files:**
- Modify: `internal/domain/favorites/service_test.go`
- Create: `internal/testutil/mocks/favorites_mocks.go` (if not exists)

- [ ] **Step 1: Check for existing canonical mock**

```bash
ls internal/testutil/mocks/ | grep -i fav
```

- [ ] **Step 2: Find the favorites repository interface**

```bash
grep -n "type.*Repository\|interface" internal/domain/favorites/repository.go 2>/dev/null | head
```

- [ ] **Step 3: Create canonical mock following Fn-field pattern**

```go
package mocks

import (
    "context"
    "github.com/guarzo/slabledger/internal/domain/favorites"
)

// FavoritesRepositoryMock is a test double for favorites.Repository.
type FavoritesRepositoryMock struct {
    GetFavoriteFn    func(ctx context.Context, id string) (*favorites.Favorite, error)
    SaveFavoriteFn   func(ctx context.Context, f *favorites.Favorite) error
    ListFavoritesFn  func(ctx context.Context) ([]favorites.Favorite, error)
    DeleteFavoriteFn func(ctx context.Context, id string) error
    // Add all interface methods
}

func (m *FavoritesRepositoryMock) GetFavorite(ctx context.Context, id string) (*favorites.Favorite, error) {
    if m.GetFavoriteFn != nil {
        return m.GetFavoriteFn(ctx, id)
    }
    return nil, nil
}
// ... all methods
```

- [ ] **Step 4: Replace inline mock in service_test.go**

- [ ] **Step 5: Build and test**

```bash
go build ./internal/domain/favorites/... ./internal/testutil/...
go test -race ./internal/domain/favorites/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/domain/favorites/service_test.go internal/testutil/mocks/
git commit -m "refactor: replace inline mock in favorites/service_test.go with canonical mock"
```

---

## Task 8: Delete ghost `domain/storage/doc.go` (LOW)

**Problem:** `domain/storage/doc.go` is a ghost package — no exports, nothing imports it.

**Files:**
- Delete: `internal/domain/storage/doc.go`
- Possibly delete: `internal/domain/storage/` (if dir is empty after)

- [ ] **Step 1: Verify nothing imports it**

```bash
grep -rn "domain/storage" internal/ cmd/ | grep -v "_test.go" | grep -v "doc.go"
```

Expected: no results.

- [ ] **Step 2: Delete the file**

```bash
rm internal/domain/storage/doc.go
rmdir internal/domain/storage 2>/dev/null || true  # only if dir is now empty
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: delete ghost domain/storage/doc.go (no exports, no imports)"
```

---

## Task 9: Remove stale pricing source fields — `pricing/repository.go:56-57` (LOW)

**Problem:** `CLPricedCards` and `MMPricedCards` fields in `pricing/repository.go` are from removed pricing sources (CardLadder, MarketMovers). These were dropped on 2026-04-06 per CLAUDE.md.

**Files:**
- Modify: `internal/domain/pricing/repository.go:56-57`

- [ ] **Step 1: Find the fields**

```bash
grep -n "CLPricedCards\|MMPricedCards" internal/domain/pricing/repository.go
```

- [ ] **Step 2: Check for usages**

```bash
grep -rn "CLPricedCards\|MMPricedCards" internal/ cmd/ | grep -v "repository.go"
```

If no usages: delete the fields.

- [ ] **Step 3: Remove the fields**

```go
// Remove:
CLPricedCards int // stale — CardLadder removed 2026-04-06
MMPricedCards int // stale — MarketMovers removed 2026-04-06
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/pricing/repository.go
git commit -m "chore: remove stale CLPricedCards/MMPricedCards fields from pricing repository"
```

---

## Final Verification

- [ ] **Run full test suite**

```bash
go test -race -timeout 10m ./...
```

- [ ] **Run quality checks**

```bash
make check
```
