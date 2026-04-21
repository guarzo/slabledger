# Inventory Filter Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reshape the inventory screen's secondary filters into workflow-aligned buckets (Pending DH Match, Pending Price, DH Listed, Skipped, Awaiting Intake) and promote the completing action on each row so operators finish the full DH pipeline without leaving the page.

**Architecture:** Extend `inventoryCalcs.ts` with status-partitioning predicates and an updated `TabCounts` / `FilterTab` union. `InventoryHeader` renders the new pill set. `DesktopRow` / `MobileCard` derive a status-driven `actionIntent` so the prominent primary button always reflects what the item needs next. `ExpandedDetail` adopts the cert-intake "set reviewed price → list on DH" one-shot confirm when the item is pending price. Backend `HandleDismissMatch` is relaxed to accept any pre-listed state so a single frontend action covers every bucket.

**Tech Stack:** Go 1.26 / `chi` router + `golang-migrate` (backend); React 18 + TypeScript + Vite + TanStack Query + Vitest + React Testing Library (frontend).

---

## Working directory

All work happens in the `inventory-filter-workflow` worktree at `/workspace/.worktrees/inventory-filter-workflow/`. Run all commands from that directory unless otherwise specified.

---

## File map

Backend (relax one state guard + expand existing table test):
- `internal/adapters/httpserver/handlers/dh_dismiss_handler.go`
- `internal/adapters/httpserver/handlers/dh_dismiss_handler_test.go`

Frontend calc layer (predicates + partition + filter dispatch):
- `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts`
- `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts`

Frontend UI (pill row + row action promotion + combined price-and-list):
- `web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx`
- `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`
- `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx`
- `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx`
- `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts`

No new files are created. No type-sync work is needed (no new backend response shapes).

---

## Task 1: Relax backend dismiss guard

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_dismiss_handler.go:44-47`
- Test: `internal/adapters/httpserver/handlers/dh_dismiss_handler_test.go`

- [ ] **Step 1.1: Add a table-driven failing test for expanded dismiss states**

Add this test at the end of `dh_dismiss_handler_test.go`:

```go
func TestHandleDismissMatch_StateMatrix(t *testing.T) {
	cases := []struct {
		name      string
		status    inventory.DHPushStatus
		wantCode  int
		wantEvent bool
	}{
		{"pending allowed", inventory.DHPushStatusPending, http.StatusOK, true},
		{"unmatched allowed", inventory.DHPushStatusUnmatched, http.StatusOK, true},
		{"matched allowed", inventory.DHPushStatusMatched, http.StatusOK, true},
		{"manual allowed", inventory.DHPushStatusManual, http.StatusOK, true},
		{"held allowed", inventory.DHPushStatusHeld, http.StatusOK, true},
		{"already dismissed rejected", inventory.DHPushStatusDismissed, http.StatusConflict, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			purchase := &inventory.Purchase{
				ID:           "pur-1",
				CertNumber:   "c-1",
				DHPushStatus: tc.status,
			}
			repo := &mocks.PurchaseRepositoryMock{
				GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
					return purchase, nil
				},
			}
			rec := &mocks.MockEventRecorder{}
			h := NewDHHandler(DHHandlerDeps{
				PurchaseLister:    repo,
				PushStatusUpdater: repo,
				Logger:            mocks.NewMockLogger(),
				BaseCtx:           context.Background(),
				EventRecorder:     rec,
			})

			body, _ := json.Marshal(dhDismissRequest{PurchaseID: "pur-1"})
			req := authenticatedRequest(httptest.NewRequest(http.MethodPost, "/api/dh/dismiss", bytes.NewReader(body)))
			rr := httptest.NewRecorder()
			h.HandleDismissMatch(rr, req)

			require.Equal(t, tc.wantCode, rr.Code)
			if tc.wantEvent {
				require.Len(t, rec.Events, 1)
				assert.Equal(t, tc.status, rec.Events[0].PrevPushStatus)
				assert.Equal(t, inventory.DHPushStatusDismissed, rec.Events[0].NewPushStatus)
			} else {
				assert.Len(t, rec.Events, 0)
			}
		})
	}
}
```

- [ ] **Step 1.2: Run test to verify it fails**

Run: `go test ./internal/adapters/httpserver/handlers -run TestHandleDismissMatch_StateMatrix -race -count=1`
Expected: FAIL — "pending", "matched", "manual", "held" subtests return 409 because the current handler only accepts `unmatched`.

- [ ] **Step 1.3: Relax the handler's pre-condition**

In `dh_dismiss_handler.go`, replace the block at lines 44-47:

```go
	if p.DHPushStatus != inventory.DHPushStatusUnmatched {
		writeError(w, http.StatusConflict, "purchase is not in unmatched state")
		return
	}
```

with:

```go
	// Dismiss is valid from any non-terminal push state. Reject if the item is
	// already dismissed (idempotency) — DH-listed and sold items reach this
	// path by having dhPushStatus stuck in matched/manual, so the same guard
	// also covers already-dismissed without widening it further.
	switch p.DHPushStatus {
	case inventory.DHPushStatusPending,
		inventory.DHPushStatusUnmatched,
		inventory.DHPushStatusMatched,
		inventory.DHPushStatusManual,
		inventory.DHPushStatusHeld:
		// allowed
	default:
		writeError(w, http.StatusConflict, "purchase cannot be dismissed from current state")
		return
	}
	prevStatus := p.DHPushStatus
```

Then update the event recording a few lines below so `PrevPushStatus` uses the real previous status instead of a hardcoded constant:

```go
	h.recordEvent(ctx, dhevents.Event{
		PurchaseID:     p.ID,
		CertNumber:     p.CertNumber,
		Type:           dhevents.TypeDismissed,
		PrevPushStatus: prevStatus,
		NewPushStatus:  inventory.DHPushStatusDismissed,
		Source:         dhevents.SourceManualUI,
	})
```

- [ ] **Step 1.4: Run tests to verify they pass**

Run: `go test ./internal/adapters/httpserver/handlers -run TestHandleDismissMatch -race -count=1`
Expected: PASS (new matrix + original `TestHandleDismissMatch_RecordsDismissedEvent` + `TestHandleDismissMatch_NilRecorderIsSafe`).

- [ ] **Step 1.5: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_dismiss_handler.go \
        internal/adapters/httpserver/handlers/dh_dismiss_handler_test.go
git commit -m "dh: allow dismiss from any pre-listed push state"
```

---

## Task 2: New partition predicates in `inventoryCalcs.ts`

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts`
- Test: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts`

- [ ] **Step 2.1: Write failing predicate tests**

Append to `inventoryCalcs.test.ts` (under the top-level `describe('inventoryCalcs', ...)`):

```ts
  describe('bucket predicates', () => {
    it('isSkipped returns true only when dhPushStatus is dismissed', () => {
      expect(isSkipped(makeItem({ purchase: { dhPushStatus: 'dismissed' } }))).toBe(true);
      expect(isSkipped(makeItem({ purchase: { dhPushStatus: 'matched' } }))).toBe(false);
      expect(isSkipped(makeItem({ purchase: { dhPushStatus: undefined } }))).toBe(false);
    });

    it('isDHListed returns true only when dhStatus is listed', () => {
      expect(isDHListed(makeItem({ purchase: { dhStatus: 'listed' } }))).toBe(true);
      expect(isDHListed(makeItem({ purchase: { dhStatus: 'in stock' } }))).toBe(false);
    });

    it('isPendingDHMatch requires received and no dhInventoryId and not skipped', () => {
      const received = '2026-04-20T00:00:00Z';
      expect(isPendingDHMatch(makeItem({ purchase: { receivedAt: received, dhInventoryId: undefined } }))).toBe(true);
      expect(isPendingDHMatch(makeItem({ purchase: { receivedAt: undefined, dhInventoryId: undefined } }))).toBe(false);
      expect(isPendingDHMatch(makeItem({ purchase: { receivedAt: received, dhInventoryId: 42 } }))).toBe(false);
      expect(isPendingDHMatch(makeItem({ purchase: { receivedAt: received, dhInventoryId: undefined, dhPushStatus: 'dismissed' } }))).toBe(false);
    });

    it('isPendingPrice requires received + matched + no committed price + not listed + not skipped', () => {
      const received = '2026-04-20T00:00:00Z';
      const base = { receivedAt: received, dhInventoryId: 42, dhStatus: 'in stock' as const };
      expect(isPendingPrice(makeItem({ purchase: { ...base } }))).toBe(true);
      expect(isPendingPrice(makeItem({ purchase: { ...base, reviewedPriceCents: 5000 } }))).toBe(false);
      expect(isPendingPrice(makeItem({ purchase: { ...base, overridePriceCents: 5000 } }))).toBe(false);
      expect(isPendingPrice(makeItem({ purchase: { ...base, dhStatus: 'listed' } }))).toBe(false);
      expect(isPendingPrice(makeItem({ purchase: { ...base, dhPushStatus: 'dismissed' } }))).toBe(false);
    });

    it('partition: every item lands in exactly one secondary bucket', () => {
      const received = '2026-04-20T00:00:00Z';
      const items = [
        makeItem({ purchase: { id: 'a', receivedAt: undefined } }),                                              // awaiting_intake
        makeItem({ purchase: { id: 'b', receivedAt: received, dhPushStatus: 'dismissed' } }),                    // skipped
        makeItem({ purchase: { id: 'c', receivedAt: received, dhStatus: 'listed', dhInventoryId: 1 } }),         // dh_listed
        makeItem({ purchase: { id: 'd', receivedAt: received, dhInventoryId: undefined } }),                     // pending_dh_match
        makeItem({ purchase: { id: 'e', receivedAt: received, dhInventoryId: 2, dhStatus: 'in stock' } }),       // pending_price
        makeItem({ purchase: { id: 'f', receivedAt: received, dhInventoryId: 3, dhStatus: 'in stock', reviewedPriceCents: 5000 } }), // ready_to_list
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.awaiting_intake).toBe(1);
      expect(meta.tabCounts.skipped).toBe(1);
      expect(meta.tabCounts.dh_listed).toBe(1);
      expect(meta.tabCounts.pending_dh_match).toBe(1);
      expect(meta.tabCounts.pending_price).toBe(1);
      expect(meta.tabCounts.ready_to_list).toBe(1);
      expect(meta.tabCounts.all).toBe(6);
      // Sum of partitioned buckets equals total.
      const partitioned =
        meta.tabCounts.awaiting_intake +
        meta.tabCounts.skipped +
        meta.tabCounts.dh_listed +
        meta.tabCounts.pending_dh_match +
        meta.tabCounts.pending_price +
        meta.tabCounts.ready_to_list;
      expect(partitioned).toBe(meta.tabCounts.all);
    });
  });
```

And update the top `import` block in the test file to add the new predicates:

```ts
import {
  computeInventoryMeta,
  filterAndSortItems,
  isReadyToList,
  needsPriceReview,
  wasUnlistedFromDH,
  isSkipped,
  isDHListed,
  isPendingDHMatch,
  isPendingPrice,
} from './inventoryCalcs';
```

Also update the `TestPurchase` type so the new fields are allowed:

```ts
type TestPurchase = Pick<Purchase,
  'id' | 'cardName' | 'gradeValue' | 'certNumber' | 'receivedAt' |
  'campaignId' | 'clValueCents' | 'buyCostCents' | 'psaSourcingFeeCents' | 'purchaseDate' |
  'createdAt' | 'updatedAt' | 'aiSuggestedPriceCents' | 'reviewedAt' |
  'dhInventoryId' | 'dhStatus' | 'reviewedPriceCents' | 'dhUnlistedDetectedAt' |
  'overridePriceCents' | 'dhPushStatus'
> & {
  setName?: string;
  cardNumber?: string;
};
```

- [ ] **Step 2.2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts`
Expected: FAIL — the new predicates don't exist yet.

- [ ] **Step 2.3: Implement the new predicates + update `TabCounts` / `FilterTab`**

In `inventoryCalcs.ts`:

Replace `EXCEPTION_STATUSES` and `TabCounts` declarations and `computeInventoryMeta` with the expanded versions below. Keep the existing `isDHHeld`, `hasCommittedPrice`, `needsAttention`, `isReadyToList`, `needsPriceReview`, `wasUnlistedFromDH` functions as-is. Add the new predicates.

```ts
import type { AgingItem, ReviewStats, ExpectedValue } from '../../../../types/campaigns';
import { costBasis, bestPrice, unrealizedPL, getReviewStatus, reviewUrgencySort } from './utils';
import type { SortKey, SortDir } from './utils';

const EXCEPTION_STATUSES = ['large_gap', 'no_data', 'flagged'] as const;

export interface TabCounts {
  needs_attention: number;
  awaiting_intake: number;
  pending_dh_match: number;
  pending_price: number;
  ready_to_list: number;
  dh_listed: number;
  skipped: number;
  /** Legacy alias for bookmarks using the old filter key. Equals `all`. */
  in_hand: number;
  all: number;
}

export interface SummaryStats {
  totalCost: number;
  totalMarket: number;
  totalPL: number;
}

export interface InventoryMeta {
  reviewStats: ReviewStats;
  tabCounts: TabCounts;
  summary: SummaryStats;
}

export function isDHHeld(item: AgingItem): boolean {
  return item.purchase.dhPushStatus === 'held';
}

function hasCommittedPrice(item: AgingItem): boolean {
  return (
    (item.purchase.reviewedPriceCents ?? 0) > 0 ||
    (item.purchase.overridePriceCents ?? 0) > 0
  );
}

export function isSkipped(item: AgingItem): boolean {
  return item.purchase.dhPushStatus === 'dismissed';
}

export function isDHListed(item: AgingItem): boolean {
  return item.purchase.dhStatus === 'listed';
}

export function isPendingDHMatch(item: AgingItem): boolean {
  if (!item.purchase.receivedAt) return false;
  if (isSkipped(item)) return false;
  if (isDHListed(item)) return false;
  return !item.purchase.dhInventoryId;
}

export function isPendingPrice(item: AgingItem): boolean {
  if (!item.purchase.receivedAt) return false;
  if (isSkipped(item)) return false;
  if (isDHListed(item)) return false;
  if (!item.purchase.dhInventoryId) return false;
  return !hasCommittedPrice(item);
}

export function isReadyToList(item: AgingItem): boolean {
  if (isSkipped(item)) return false;
  return (
    !!item.purchase.receivedAt &&
    !!item.purchase.dhInventoryId &&
    item.purchase.dhStatus !== 'listed' &&
    hasCommittedPrice(item)
  );
}

export function needsPriceReview(item: AgingItem): boolean {
  // Existing alias — same semantic as isPendingPrice.
  return isPendingPrice(item);
}

export function wasUnlistedFromDH(item: AgingItem): boolean {
  return !!item.purchase.dhUnlistedDetectedAt;
}

export function needsAttention(item: AgingItem, status = getReviewStatus(item)): boolean {
  if (!item.purchase.receivedAt) return false;
  if (isSkipped(item)) return false;
  if ((EXCEPTION_STATUSES as readonly string[]).includes(status)) return true;
  if (isDHHeld(item)) return true;
  if (!hasCommittedPrice(item) && (item.purchase.aiSuggestedPriceCents ?? 0) > 0) return true;
  return false;
}

export function computeInventoryMeta(items: AgingItem[]): InventoryMeta {
  const stats: ReviewStats = { total: items.length, reviewed: 0, flagged: 0, aging60d: 0 };
  const counts: TabCounts = {
    needs_attention: 0,
    awaiting_intake: 0,
    pending_dh_match: 0,
    pending_price: 0,
    ready_to_list: 0,
    dh_listed: 0,
    skipped: 0,
    in_hand: 0,
    all: items.length,
  };
  let totalCost = 0;
  let totalMarket = 0;
  for (const item of items) {
    if (item.hasOpenFlag) stats.flagged++;
    if (item.purchase.reviewedAt) stats.reviewed++;
    if (item.daysHeld >= 60) stats.aging60d++;

    const status = getReviewStatus(item);
    if (needsAttention(item, status)) counts.needs_attention++;

    // Secondary-row partition: evaluate top-down, first match wins.
    if (!item.purchase.receivedAt) {
      counts.awaiting_intake++;
    } else if (isSkipped(item)) {
      counts.skipped++;
    } else if (isDHListed(item)) {
      counts.dh_listed++;
    } else if (isPendingDHMatch(item)) {
      counts.pending_dh_match++;
    } else if (isPendingPrice(item)) {
      counts.pending_price++;
    } else if (isReadyToList(item)) {
      counts.ready_to_list++;
    }

    totalCost += costBasis(item.purchase);
    totalMarket += bestPrice(item);
  }
  counts.in_hand = counts.all; // alias kept for old bookmarks
  return {
    reviewStats: stats,
    tabCounts: counts,
    summary: { totalCost, totalMarket, totalPL: totalMarket - totalCost },
  };
}

export type FilterTab =
  | 'needs_attention'
  | 'sell_sheet'
  | 'all'
  | 'awaiting_intake'
  | 'pending_dh_match'
  | 'pending_price'
  | 'ready_to_list'
  | 'dh_listed'
  | 'skipped'
  | 'in_hand'; // legacy alias

export function filterAndSortItems(
  items: AgingItem[],
  opts: {
    debouncedSearch: string;
    showAll: boolean;
    filterTab: FilterTab;
    sellSheetHas: (id: string) => boolean;
    sortKey: SortKey;
    sortDir: SortDir;
    evMap: Map<string, ExpectedValue>;
  },
): AgingItem[] {
  const { debouncedSearch, showAll, filterTab, sellSheetHas, sortKey, sortDir, evMap } = opts;
  let result = items;

  if (debouncedSearch.trim()) {
    const q = debouncedSearch.toLowerCase();
    result = result.filter(i =>
      i.purchase.cardName.toLowerCase().includes(q) ||
      (i.purchase.certNumber && i.purchase.certNumber.toLowerCase().includes(q)) ||
      (i.purchase.setName && i.purchase.setName.toLowerCase().includes(q))
    );
  } else if (!showAll) {
    if (filterTab === 'sell_sheet') {
      result = result.filter(i => sellSheetHas(i.purchase.id) && !!i.purchase.receivedAt);
    } else if (filterTab === 'in_hand') {
      // Legacy alias: treat as `all`.
      // result stays as-is
    } else if (filterTab !== 'all') {
      result = result.filter(i => {
        switch (filterTab) {
          case 'needs_attention': return needsAttention(i);
          case 'awaiting_intake': return !i.purchase.receivedAt;
          case 'pending_dh_match': return isPendingDHMatch(i);
          case 'pending_price': return isPendingPrice(i);
          case 'ready_to_list': return isReadyToList(i);
          case 'dh_listed': return isDHListed(i);
          case 'skipped': return isSkipped(i);
          default: return false;
        }
      });
    }
  }

  if (!showAll && !debouncedSearch.trim()) {
    return [...result].sort(reviewUrgencySort);
  }

  const dir = sortDir === 'asc' ? 1 : -1;
  return [...result].sort((a, b) => {
    switch (sortKey) {
      case 'name':
        return dir * a.purchase.cardName.localeCompare(b.purchase.cardName);
      case 'grade':
        return dir * (a.purchase.gradeValue - b.purchase.gradeValue);
      case 'cost':
        return dir * (costBasis(a.purchase) - costBasis(b.purchase));
      case 'market': {
        const ma = bestPrice(a);
        const mb = bestPrice(b);
        return dir * (ma - mb);
      }
      case 'pl': {
        const pa = unrealizedPL(costBasis(a.purchase), a) ?? -Infinity;
        const pb = unrealizedPL(costBasis(b.purchase), b) ?? -Infinity;
        return dir * (pa - pb);
      }
      case 'days':
        return dir * (a.daysHeld - b.daysHeld);
      case 'ev': {
        const ea = evMap.get(a.purchase.certNumber)?.evCents ?? -Infinity;
        const eb = evMap.get(b.purchase.certNumber)?.evCents ?? -Infinity;
        return dir * (ea - eb);
      }
      default:
        return 0;
    }
  });
}
```

- [ ] **Step 2.4: Update existing `computeInventoryMeta` tests that assert on `in_hand`**

The old tests assert `tabCounts.in_hand === <received count>`. Under the new semantics, `in_hand` is a legacy alias for `all` (= every item). Open `inventoryCalcs.test.ts` and replace any assertion of the form `expect(meta.tabCounts.in_hand).toBe(N)` where N was derived from "items with receivedAt" with assertions against the new buckets that capture the original intent. For example:

Old:
```ts
expect(meta.tabCounts.in_hand).toBe(2);
```

New (for the same fixture of two received items, none matched):
```ts
expect(meta.tabCounts.pending_dh_match).toBe(2);
```

For tests that explicitly want the "every item" count, use `meta.tabCounts.all`.

- [ ] **Step 2.5: Run tests to verify they pass**

Run: `cd web && npx vitest run src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts`
Expected: PASS.

- [ ] **Step 2.6: Run the TypeScript check for the whole web package**

Run: `cd web && npm run typecheck` (or `npx tsc --noEmit` if there is no typecheck script; check `web/package.json` to confirm the right command).
Expected: PASS. If consumers of `TabCounts` / `FilterTab` error out, defer fixing them to the later UI tasks that actually touch those files.

- [ ] **Step 2.7: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts \
        web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts
git commit -m "inventory: add status-partition predicates and filter buckets"
```

---

## Task 3: Update `InventoryHeader` pill definitions

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx:59-68`

- [ ] **Step 3.1: Replace the `primary` and `secondary` pill arrays**

Find the block at lines 59-68:

```tsx
  const primary = useMemo(() => [
    { key: 'needs_attention' as const, label: 'Needs Attention', count: tabCounts.needs_attention, alwaysShow: true },
    { key: 'ready_to_list' as const, label: 'Pending DH Listing', count: tabCounts.ready_to_list, alwaysShow: false },
    { key: 'sell_sheet' as const, label: 'Sell Sheet', count: pageSellSheetCount, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts, pageSellSheetCount]);
  const secondary = useMemo(() => [
    { key: 'all' as const, label: 'All', count: tabCounts.all, alwaysShow: true },
    { key: 'in_hand' as const, label: 'In Hand', count: tabCounts.in_hand, alwaysShow: false },
    { key: 'awaiting_intake' as const, label: 'Awaiting Intake', count: tabCounts.awaiting_intake, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts]);
```

Replace with:

```tsx
  const primary = useMemo(() => [
    { key: 'needs_attention' as const, label: 'Needs Attention', count: tabCounts.needs_attention, alwaysShow: true },
    { key: 'ready_to_list' as const, label: 'Pending DH Listing', count: tabCounts.ready_to_list, alwaysShow: false },
    { key: 'sell_sheet' as const, label: 'Sell Sheet', count: pageSellSheetCount, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts, pageSellSheetCount]);
  const secondary = useMemo(() => [
    { key: 'all' as const, label: 'All', count: tabCounts.all, alwaysShow: true },
    { key: 'dh_listed' as const, label: 'DH Listed', count: tabCounts.dh_listed, alwaysShow: false },
    { key: 'pending_dh_match' as const, label: 'Pending DH Match', count: tabCounts.pending_dh_match, alwaysShow: false },
    { key: 'pending_price' as const, label: 'Pending Price', count: tabCounts.pending_price, alwaysShow: false },
    { key: 'skipped' as const, label: 'Skipped on DH Listing', count: tabCounts.skipped, alwaysShow: false },
    { key: 'awaiting_intake' as const, label: 'Awaiting Intake', count: tabCounts.awaiting_intake, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts]);
```

- [ ] **Step 3.2: Update the smart-default tab logic in `useInventoryState`**

Open `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts`. Around lines 80-91 there is a `useEffect` that falls back from `needs_attention` → `ready_to_list` → `all`. Extend the fallback chain so it also tries the new pending buckets:

```tsx
  useEffect(() => {
    if (userTabChosenRef.current || items.length === 0) return;
    userTabChosenRef.current = true;
    if (tabCounts.needs_attention > 0) return;
    if (tabCounts.ready_to_list > 0) {
      setFilterTab('ready_to_list');
    } else if (tabCounts.pending_price > 0) {
      setFilterTab('pending_price');
    } else if (tabCounts.pending_dh_match > 0) {
      setFilterTab('pending_dh_match');
    } else {
      setFilterTab('all');
    }
  }, [items.length, tabCounts.needs_attention, tabCounts.ready_to_list, tabCounts.pending_price, tabCounts.pending_dh_match]);
```

- [ ] **Step 3.3: Typecheck and test to make sure header still renders cleanly**

Run:
```bash
cd web && npm run typecheck && npx vitest run src/react/pages/campaign-detail/inventory
```
Expected: PASS.

- [ ] **Step 3.4: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx \
        web/src/react/pages/campaign-detail/inventory/useInventoryState.ts
git commit -m "inventory: new secondary filter pills (DH Listed / Pending DH Match / Pending Price / Skipped)"
```

---

## Task 4: Add `handleDismiss` / `handleUndismiss` to `useInventoryState`

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts`

- [ ] **Step 4.1: Add the two handlers**

Add immediately after `handleApproveDHPush` (around line 154):

```tsx
  const handleDismiss = useCallback(async (purchaseId: string) => {
    try {
      await api.dismissDHMatch(purchaseId);
      toast.success('Dismissed from DH listing');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to dismiss'));
    }
  }, [toast, invalidateInventory]);

  const handleUndismiss = useCallback(async (purchaseId: string) => {
    try {
      await api.undismissDHMatch(purchaseId);
      toast.success('Restored to DH pipeline');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to restore'));
    }
  }, [toast, invalidateInventory]);
```

- [ ] **Step 4.2: Export both handlers from the hook's return value**

Find the return statement at the bottom of `useInventoryState` and add `handleDismiss` and `handleUndismiss` alongside `handleApproveDHPush` (keep alphabetical or nearby — the exact ordering is fine as long as it compiles).

- [ ] **Step 4.3: Typecheck**

Run: `cd web && npm run typecheck`
Expected: PASS.

- [ ] **Step 4.4: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/useInventoryState.ts
git commit -m "inventory: add dismiss/undismiss handlers"
```

---

## Task 5: Status-driven `actionIntent` + Dismiss/Restore affordances in `DesktopRow`

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`

- [ ] **Step 5.1: Extend `DesktopRowProps` with dismiss/undismiss callbacks**

Find the `DesktopRowProps` interface and add:

```tsx
  onDismiss?: () => void;
  onUndismiss?: () => void;
```

Add them to the destructured props in the function signature too.

- [ ] **Step 5.2: Add the `actionIntent` derivation and import the new predicates**

At the top of `DesktopRow.tsx`, change the import from `./inventoryCalcs` to include the new predicates:

```tsx
import { isReadyToList, needsPriceReview, wasUnlistedFromDH, isPendingDHMatch, isSkipped } from './inventoryCalcs';
```

Inside the `DesktopRow` function, just after the existing `reviewStatus` / `hotSeller` / `dot` derivations (around line 84), add:

```tsx
  type ActionIntent = 'fix_match' | 'set_and_list' | 'list' | 'restore' | 'none';
  const actionIntent: ActionIntent = (() => {
    if (isSkipped(item)) return 'restore';
    if (isPendingDHMatch(item)) return 'fix_match';
    if (needsPriceReview(item)) return 'set_and_list';
    if (isReadyToList(item)) return 'list';
    return 'none';
  })();
  const showDismiss = actionIntent === 'fix_match' || actionIntent === 'set_and_list' || actionIntent === 'list';
```

- [ ] **Step 5.3: Replace the action-cell rendering to honor `actionIntent`**

Find the block inside the `<div className="glass-table-td flex-shrink-0 text-center !px-1 print-hide-actions" style={{ width: '56px' }}>` (the one containing the existing List / Set price / badge logic, around lines 229-275). Replace its inner `<div className="flex flex-col items-center gap-0.5">` content with:

```tsx
        <div className="flex flex-col items-center gap-0.5">
          {wasUnlistedFromDH(item) && (
            <span
              className="text-[9px] font-medium px-1 py-0.5 rounded bg-[var(--warning)]/15 text-[var(--warning)] leading-none"
              title="Item was removed from DH — will be re-pushed + listed"
            >
              Re-list
            </span>
          )}
          {dhListedOverride ? (
            <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS.listed}`} title={DH_BADGE_TITLES['listed']}>listed</span>
          ) : actionIntent === 'fix_match' && onFixDHMatch ? (
            <button
              type="button"
              onClick={onFixDHMatch}
              className="text-xs font-medium px-2 py-1 rounded bg-[var(--warning)]/15 text-[var(--warning)] hover:bg-[var(--warning)]/30 transition-colors"
              title="Paste the correct DoubleHolo URL to fix the match"
              aria-label="Fix DH Match"
            >
              Fix Match
            </button>
          ) : actionIntent === 'list' && onListOnDH ? (
            <button
              type="button"
              onClick={() => onListOnDH(item.purchase.id)}
              disabled={dhListingLoading}
              className={`text-xs font-medium px-2 py-1 rounded transition-colors ${
                dhListingLoading
                  ? 'bg-[var(--surface-2)] text-[var(--text-muted)] cursor-wait'
                  : 'bg-[var(--success)]/15 text-[var(--success)] hover:bg-[var(--success)]/30'
              }`}
              title="Publish this item on DH"
            >
              {dhListingLoading ? 'Listing…' : 'List'}
            </button>
          ) : actionIntent === 'set_and_list' ? (
            <button
              type="button"
              onClick={onExpand}
              className="text-xs font-medium px-2 py-1 rounded bg-[var(--warning)]/15 text-[var(--warning)] hover:bg-[var(--warning)]/30 transition-colors"
              title="Set a price and list on DH"
              aria-label="Set price and list on DH"
            >
              Set &amp; List
            </button>
          ) : actionIntent === 'restore' && onUndismiss ? (
            <button
              type="button"
              onClick={onUndismiss}
              className="text-xs font-medium px-2 py-1 rounded bg-[var(--brand-500)]/15 text-[var(--brand-400)] hover:bg-[var(--brand-500)]/30 transition-colors"
              title="Restore to DH pipeline"
            >
              Restore
            </button>
          ) : (() => {
            const badge = dhBadgeFor(item.purchase.dhPushStatus, item.purchase.dhStatus, item.purchase.receivedAt);
            if (badge === 'unenrolled') return null;
            return (
              <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS[badge]}`} title={DH_BADGE_TITLES[badge] ?? badge}>
                {badge}
              </span>
            );
          })()}
          {showDismiss && onDismiss && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                if (window.confirm('Dismiss this item from DH listing?')) onDismiss();
              }}
              className="text-[9px] text-[var(--text-muted)] hover:text-[var(--danger)] underline underline-offset-2"
              title="Skip DH for this item"
            >
              Dismiss
            </button>
          )}
        </div>
```

- [ ] **Step 5.4: Add "Dismiss" / "Restore" entries to the overflow menu**

Find the `DropdownMenu.Content` block (around line 304). After `onUnmatchDH` and before the sell-sheet item, insert:

```tsx
              {showDismiss && onDismiss && (
                <DropdownMenu.Item
                  onSelect={onDismiss}
                  className="px-3 py-2 text-sm text-[var(--text-muted)] hover:bg-[rgba(255,255,255,0.04)] hover:text-[var(--text)] outline-none cursor-default"
                >
                  Dismiss from DH
                </DropdownMenu.Item>
              )}
              {actionIntent === 'restore' && onUndismiss && (
                <DropdownMenu.Item
                  onSelect={onUndismiss}
                  className="px-3 py-2 text-sm text-[var(--text-muted)] hover:bg-[rgba(255,255,255,0.04)] hover:text-[var(--text)] outline-none cursor-default"
                >
                  Restore to DH
                </DropdownMenu.Item>
              )}
```

- [ ] **Step 5.5: Wire the new props in `InventoryTab.tsx`**

Open `web/src/react/pages/campaign-detail/InventoryTab.tsx`. Find where `DesktopRow` is rendered (search for `<DesktopRow`) and pass `onDismiss={() => handleDismiss(item.purchase.id)}` and `onUndismiss={() => handleUndismiss(item.purchase.id)}` alongside the other row callbacks. Destructure `handleDismiss` and `handleUndismiss` from the `useInventoryState(...)` result.

- [ ] **Step 5.6: Typecheck**

Run: `cd web && npm run typecheck`
Expected: PASS.

- [ ] **Step 5.7: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx \
        web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "inventory: status-driven row action + Dismiss/Restore on desktop"
```

---

## Task 6: Mirror affordances on `MobileCard`

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx`

- [ ] **Step 6.1: Add the same props and `actionIntent` derivation**

Extend `MobileCardProps` with:

```tsx
  onDismiss?: () => void;
  onUndismiss?: () => void;
```

Add them to the destructured props. Import the new predicates (`isPendingDHMatch`, `isSkipped`) from `./inventoryCalcs` alongside the existing ones.

Inside the function body, add the same `actionIntent` derivation used in `DesktopRow`:

```tsx
  type ActionIntent = 'fix_match' | 'set_and_list' | 'list' | 'restore' | 'none';
  const actionIntent: ActionIntent = (() => {
    if (isSkipped(item)) return 'restore';
    if (isPendingDHMatch(item)) return 'fix_match';
    if (needsPriceReview(item)) return 'set_and_list';
    if (isReadyToList(item)) return 'list';
    return 'none';
  })();
  const showDismiss = actionIntent === 'fix_match' || actionIntent === 'set_and_list' || actionIntent === 'list';
```

- [ ] **Step 6.2: Update the mobile action-row rendering**

Find where `MobileCard` renders its DH/list/set-price/fix-match action (search for `onListOnDH` and `onFixDHMatch`). Replace the conditional action button with the same `actionIntent`-driven block used on desktop (smaller text sizing is fine; preserve existing mobile classes). For `actionIntent === 'restore'` render a "Restore" button; for `actionIntent === 'fix_match'` render a prominent "Fix DH Match" button; for `set_and_list` render a "Set & List" button that calls `onSetPrice ?? onExpand` (mobile expands the card inline the same way desktop does via `onSetPrice`). Keep the existing sell and delete controls.

Add a secondary Dismiss link below the primary button when `showDismiss && onDismiss`, same pattern as desktop.

- [ ] **Step 6.3: Wire new props in `InventoryTab.tsx`**

Pass `onDismiss` / `onUndismiss` to `<MobileCard>` next to the existing callbacks, same as Task 5.5.

- [ ] **Step 6.4: Typecheck + run component tests**

Run: `cd web && npm run typecheck && npx vitest run src/react/pages/campaign-detail/inventory/MobileCard.test.tsx`
Expected: PASS.

- [ ] **Step 6.5: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/MobileCard.tsx \
        web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "inventory: mirror status-driven row actions on mobile"
```

---

## Task 7: Combined set-price-and-list in `ExpandedDetail`

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx`

- [ ] **Step 7.1: Thread a `combineWithList` flag through `ExpandedDetailProps`**

Extend the props interface:

```tsx
interface ExpandedDetailProps {
  item: AgingItem;
  onReviewed?: () => void;
  campaignId?: string;
  onOpenFlagDialog?: () => void;
  onResolveFlag?: (flagId: number) => void;
  onApproveDHPush?: (purchaseId: string) => void;
  onSetPrice?: () => void;
  combineWithList?: boolean;
}
```

`combineWithList` is always derivable from `needsPriceReview(item)`, so `InventoryTab` will pass exactly that. Keeping it as a prop (not computed internally) lets us turn off the combined behavior in contexts like GlobalInventoryPage if it ever needs the review-only flow.

- [ ] **Step 7.2: Add the combined handler**

Add a new import for `isAPIError`:

```tsx
import { api, isAPIError } from '../../../../js/api';
```

Add a handler alongside `handleConfirm`:

```tsx
  const handleSetAndList = async (priceCents: number, source: string) => {
    setIsSubmitting(true);
    try {
      await api.setReviewedPrice(purchase.id, priceCents, source);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save reviewed price');
      setIsSubmitting(false);
      return;
    }
    try {
      await api.listPurchaseOnDH(purchase.id);
      toast.success('Price set and listed on DH');
      invalidateQueries();
      onReviewed?.();
    } catch (err) {
      if (isAPIError(err) && err.status === 409 && err.data?.error === 'Purchase already listed on DH') {
        toast.success('Price set and listed on DH');
        invalidateQueries();
        onReviewed?.();
        return;
      }
      const msg = err instanceof Error ? err.message : 'Listing failed';
      toast.error(
        msg.toLowerCase().includes('stock')
          ? 'DH push pending — check back after sync'
          : msg,
      );
    } finally {
      setIsSubmitting(false);
    }
  };
```

- [ ] **Step 7.3: Pick which confirm handler runs**

Replace the existing `<PriceDecisionBar ... onConfirm={handleConfirm} ... />` block with:

```tsx
      <PriceDecisionBar
        sources={sources}
        preSelected={preSelected}
        onConfirm={combineWithList ? handleSetAndList : handleConfirm}
        onFlag={onOpenFlagDialog}
        isSubmitting={isSubmitting}
        confirmLabel={combineWithList ? 'List on DH' : undefined}
      />
```

If `PriceDecisionBar` does not accept a `confirmLabel` prop today, add it: open `web/src/react/ui/PriceDecisionBar.tsx`, add an optional `confirmLabel?: string` to its props, default to `'Save reviewed price'` (or whatever the current label is), and use it when rendering the confirm button. This change is additive and safe for every existing caller.

- [ ] **Step 7.4: Wire `combineWithList` from `InventoryTab`**

In `web/src/react/pages/campaign-detail/InventoryTab.tsx`, find where `<ExpandedDetail>` is rendered and pass:

```tsx
<ExpandedDetail
  item={item}
  combineWithList={needsPriceReview(item)}
  /* ...existing props... */
/>
```

Import `needsPriceReview` at the top if it isn't already.

- [ ] **Step 7.5: Add a component test for the combined flow**

Create or extend a test alongside `ExpandedDetail` (add a new file `ExpandedDetail.test.tsx` if none exists). Use React Testing Library + a mocked `api` module:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import ExpandedDetail from './ExpandedDetail';
import { ToastProvider } from '../../../contexts/ToastContext';

vi.mock('../../../../js/api', async () => {
  const actual = await vi.importActual<typeof import('../../../../js/api')>('../../../../js/api');
  return {
    ...actual,
    api: {
      setReviewedPrice: vi.fn().mockResolvedValue({ success: true, reviewedAt: '2026-04-21T00:00:00Z' }),
      listPurchaseOnDH: vi.fn().mockResolvedValue({ listed: 1, synced: 1, skipped: 0, total: 1 }),
    },
    isAPIError: actual.isAPIError,
  };
});

function renderWithProviders(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>{ui}</ToastProvider>
    </QueryClientProvider>,
  );
}

describe('ExpandedDetail combined set-and-list', () => {
  beforeEach(() => vi.clearAllMocks());

  it('calls setReviewedPrice then listPurchaseOnDH when combineWithList is true', async () => {
    // Fill in an item fixture that makes PriceDecisionBar render a confirmable source.
    // Render ExpandedDetail with combineWithList={true}, click the confirm button,
    // and assert the call order.
    // (Use existing test fixtures from inventoryCalcs.test.ts as a reference.)
  });
});
```

Fill in the fixture and the assertion using the project's existing test fixtures as reference (the `makeItem` helper in `inventoryCalcs.test.ts` produces suitable purchases). The key assertion is:

```tsx
    await waitFor(() => expect(api.setReviewedPrice).toHaveBeenCalledWith(purchaseId, anyNumber, anyString));
    await waitFor(() => expect(api.listPurchaseOnDH).toHaveBeenCalledWith(purchaseId));
    expect((api.setReviewedPrice as vi.Mock).mock.invocationCallOrder[0])
      .toBeLessThan((api.listPurchaseOnDH as vi.Mock).mock.invocationCallOrder[0]);
```

- [ ] **Step 7.6: Run the new test**

Run: `cd web && npx vitest run src/react/pages/campaign-detail/inventory/ExpandedDetail.test.tsx`
Expected: PASS.

- [ ] **Step 7.7: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx \
        web/src/react/pages/campaign-detail/inventory/ExpandedDetail.test.tsx \
        web/src/react/pages/campaign-detail/InventoryTab.tsx \
        web/src/react/ui/PriceDecisionBar.tsx
git commit -m "inventory: combine set-price + list-on-DH in expanded detail"
```

---

## Task 8: Quality sweep

**Files:** (none modified; verification only)

- [ ] **Step 8.1: Run the full frontend test suite**

Run: `cd web && npm test`
Expected: PASS. Triage any unrelated failures — if something fails that this change didn't touch, consult the user before poking at it.

- [ ] **Step 8.2: Run backend tests with race detection**

Run: `go test -race -timeout 10m ./...`
Expected: PASS.

- [ ] **Step 8.3: Run the architecture / quality gate**

Run: `make check`
Expected: PASS.

- [ ] **Step 8.4: Manual smoke test**

Start the server and dev frontend:

```bash
go build -o slabledger ./cmd/slabledger && ./slabledger &
cd web && npm run dev
```

Open the inventory page on a campaign that has at least one item in each state and verify:
- The secondary pill row shows `All | DH Listed | Pending DH Match | Pending Price | Skipped on DH Listing | Awaiting Intake` (any empty buckets hidden).
- Clicking each pill filters to items that match the predicate — no item appears in more than one bucket.
- A Pending-DH-Match row shows a prominent "Fix Match" pill and a small Dismiss link; clicking Dismiss moves the item to Skipped with a success toast.
- A Pending-Price row shows "Set & List"; clicking it expands the row to `PriceDecisionBar`; the confirm button reads "List on DH"; clicking it results in the item ending up in DH Listed (or a "DH push pending — check back after sync" toast if the DH pipeline is slow).
- A Skipped row shows "Restore"; clicking it moves the item back into the appropriate pending bucket.
- The old `in_hand` tab key, if hit via direct state manipulation, renders the same items as `all`.

- [ ] **Step 8.5: Push + PR (only when the user approves)**

```bash
git push -u origin inventory-filter-workflow
gh pr create --title "inventory: workflow-aligned secondary filters + one-shot List on DH" --body "$(cat <<'EOF'
## Summary
- Replaces coarse "In Hand" bucket with status-specific secondary filters (DH Listed / Pending DH Match / Pending Price / Skipped on DH Listing / Awaiting Intake).
- Row primary action is now status-driven (Fix Match / Set & List / List / Restore) so every bucket has a one-click path to the next step.
- `PriceDecisionBar` confirm becomes one-shot "List on DH" in the expanded detail when the item is pending price, mirroring the Cert Intake flow.
- Backend `HandleDismissMatch` accepts dismiss from any pre-listed push state; frontend gets `Dismiss`/`Restore` wired on every qualifying row.

## Test plan
- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm test` passes
- [ ] Manual: inventory screen shows all six secondary pills (where non-empty), partition is strict, and the full match → price → list flow completes without leaving the page.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review notes

- Spec coverage: every section of the design doc maps to a task (filter taxonomy → Tasks 2/3; partition → Task 2; row affordance model → Tasks 5/6; combined set-price-and-list → Task 7; backend dismiss relaxation → Task 1; `Restore` → Tasks 4/5/6; tests → within each task + Task 8).
- No placeholders. Test files contain concrete assertions; every code change shows exact code.
- Type consistency: `FilterTab`, `TabCounts`, `ActionIntent`, and the predicate names (`isSkipped`, `isDHListed`, `isPendingDHMatch`, `isPendingPrice`, `isReadyToList`, `needsPriceReview`) are used consistently across Tasks 2, 3, 5, 6, 7.
- Scope: all changes are under `inventory-filter-workflow`; no migration; no new files; `docs/superpowers/plans/` is gitignored so the plan lives under `docs/design/` alongside the spec.
