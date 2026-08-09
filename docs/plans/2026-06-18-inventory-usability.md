# Inventory Page Usability Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix two usability defects on the global inventory page (`/inventory`): make the `$` price-band pill badge counts reflect the active tab + search, and make clicking a column header actually sort the rows.

**Architecture:** Frontend-only. Extract the existing search/tab selection in `filterAndSortItems` into a shared `applySearchAndTab` helper, then count price bands over that scoped base set via a new `computePriceBandCounts`. Separately, make the row order honor the chosen `sortKey` regardless of search, with `sortKey === null` meaning the smart "urgency" default (so no phantom sort arrow shows until the user clicks a header).

**Tech Stack:** React 18 + TypeScript, Vitest + @testing-library/react, Vite. All work under `web/src/react/pages/campaign-detail/inventory/` (the stale-named folder whose sole consumer is `GlobalInventoryPage`).

**Spec:** `docs/superpowers/specs/2026-06-18-inventory-usability-design.md` (commit `09f26cf7`).

**Working dir for all `npm` commands:** `web/` inside the worktree. **`git` commands** run from the worktree root.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts` | filter / sort / count logic | extract `applySearchAndTab`; add `orderItems` + `computePriceBandCounts`; sort honors `SortKey \| null` |
| `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts` | inventory state hook | `sortKey` defaults to `null`; `priceBandCounts` from a tab+search-scoped memo |
| `web/src/react/pages/campaign-detail/inventory/SortableHeader.tsx` | clickable column header | widen `currentKey` prop to `SortKey \| null` |
| `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts` | unit tests | add `applySearchAndTab`, `computePriceBandCounts`, real-sort assertions |
| `web/src/react/pages/campaign-detail/inventory/useInventoryState.test.ts` | hook tests | default `sortKey` null; `handleSort` toggle; band-count scoping |

No backend, API, or `web/src/types/` changes.

---

## Task 1: Extract `applySearchAndTab` (pure refactor, no behavior change)

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts` (the `filterAndSortItems` function near the end)

This is a behavior-preserving refactor guarded by the existing `filterAndSortItems` tests. We pull the search-vs-tab selection out so both the filter path and (later) the count path share one definition.

- [ ] **Step 1: Add the `applySearchAndTab` export**

Insert this new function immediately **above** the existing `filterAndSortItems` function in `inventoryCalcs.ts`:

```ts
/** Select the base set for the current view: search wins over the tab filter;
    `all` and the legacy `in_hand` alias apply no narrowing. This is the single
    source of truth for "what rows does the active tab+search show", shared by
    both row filtering and price-band counting. */
export function applySearchAndTab(
  items: AgingItem[],
  debouncedSearch: string,
  filterTab: FilterTab,
): AgingItem[] {
  if (debouncedSearch.trim()) {
    const q = debouncedSearch.toLowerCase();
    return items.filter(i =>
      i.purchase.cardName.toLowerCase().includes(q) ||
      (i.purchase.certNumber && i.purchase.certNumber.toLowerCase().includes(q)) ||
      (i.purchase.setName && i.purchase.setName.toLowerCase().includes(q))
    );
  }
  if (filterTab === 'in_hand' || filterTab === 'all') {
    return items;
  }
  return items.filter(i => {
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
```

- [ ] **Step 2: Route `filterAndSortItems` through the new helper**

Replace the body of `filterAndSortItems` from `let result = items;` through the end of the tab-filter `if/else` chain (i.e. everything between the pinned-branch block and the `if (priceBand !== 'all')` block) with a single call. The function should read exactly:

```ts
export function filterAndSortItems(
  items: AgingItem[],
  opts: {
    debouncedSearch: string;
    filterTab: FilterTab;
    sortKey: SortKey;
    sortDir: SortDir;
    pinnedIds?: ReadonlySet<string>;
    priceBand?: PriceBand;
  },
): AgingItem[] {
  const { debouncedSearch, filterTab, sortKey, sortDir, priceBand = 'all' } = opts;

  if (opts.pinnedIds && opts.pinnedIds.size > 0) {
    const subset = items.filter(i => opts.pinnedIds!.has(i.purchase.id));
    return sortItems(subset, sortKey, sortDir);
  }

  let result = applySearchAndTab(items, debouncedSearch, filterTab);

  if (priceBand !== 'all') {
    result = result.filter(i => matchesPriceBand(i, priceBand));
  }

  if (!debouncedSearch.trim()) {
    return [...result].sort(reviewUrgencySort);
  }

  return sortItems(result, sortKey, sortDir);
}
```

(Note: the search-gated sort tail and the pinned branch are intentionally **unchanged** in this task — behavior is identical. Task 2 changes the sort behavior.)

- [ ] **Step 3: Run the existing tests to confirm no behavior change**

Run: `cd web && npm test -- inventoryCalcs`
Expected: PASS — all existing `inventoryCalcs` tests (including the 5 `filterAndSortItems` cases) green.

- [ ] **Step 4: Typecheck**

Run: `cd web && npm run typecheck`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts
git commit -m "refactor(inventory): extract applySearchAndTab from filterAndSortItems"
```

---

## Task 2: Make sort honor the chosen column (Issue 2 core)

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts`
- Test: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts`

The bug: with no active search, `filterAndSortItems` always returns `reviewUrgencySort` order and ignores `sortKey`/`sortDir`. We make `sortKey === null` mean "smart urgency order" and any non-null key mean "really sort by that column", independent of search.

- [ ] **Step 1: Write the failing test**

Add this `describe` block inside the top-level `describe('inventoryCalcs', () => { ... })` in `inventoryCalcs.test.ts`, after the existing `filterAndSortItems` describe block (before `describe('isReadyToList'`):

```ts
  describe('filterAndSortItems — column sort (no search)', () => {
    // Three items with distinct cost bases so sort order is unambiguous.
    const items = [
      makeItem({ purchase: { id: 'mid', buyCostCents: 5000, psaSourcingFeeCents: 0 } }),
      makeItem({ purchase: { id: 'low', buyCostCents: 1000, psaSourcingFeeCents: 0 } }),
      makeItem({ purchase: { id: 'high', buyCostCents: 9000, psaSourcingFeeCents: 0 } }),
    ];

    it('sorts by cost ascending when sortKey=cost and no search is active', () => {
      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        filterTab: 'all',
        sortKey: 'cost',
        sortDir: 'asc',
      });
      expect(result.map(r => r.purchase.id)).toEqual(['low', 'mid', 'high']);
    });

    it('sorts by cost descending when sortDir=desc', () => {
      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        filterTab: 'all',
        sortKey: 'cost',
        sortDir: 'desc',
      });
      expect(result.map(r => r.purchase.id)).toEqual(['high', 'mid', 'low']);
    });

    it('falls back to smart urgency order when sortKey is null', () => {
      // All three are awaiting-intake (no receivedAt) → same review status,
      // so urgency sort is stable on the equal-priority tiebreak (daysHeld).
      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        filterTab: 'all',
        sortKey: null,
        sortDir: 'asc',
      });
      expect(result).toHaveLength(3);
    });
  });
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npm test -- inventoryCalcs`
Expected: FAIL — the "sorts by cost ascending" case returns urgency order (not cost order), and `sortKey: null` is a TypeScript error (`opts.sortKey` is still `SortKey`).

- [ ] **Step 3: Add the `orderItems` helper**

In `inventoryCalcs.ts`, add this private helper immediately **above** `filterAndSortItems` (it can sit next to `applySearchAndTab`):

```ts
/** Final row ordering: an explicit column → real column sort; `null` → the
    smart urgency order (flagged → large_gap → no_data → needs_review →
    reviewed, then oldest-first). Independent of search. */
function orderItems(items: AgingItem[], sortKey: SortKey | null, sortDir: SortDir): AgingItem[] {
  return sortKey == null
    ? [...items].sort(reviewUrgencySort)
    : sortItems(items, sortKey, sortDir);
}
```

- [ ] **Step 4: Widen `opts.sortKey` and use `orderItems` in both return paths**

In `filterAndSortItems`, change the `sortKey` field type in the `opts` object from `sortKey: SortKey;` to `sortKey: SortKey | null;`, then replace **both** sort returns. The full function should now read:

```ts
export function filterAndSortItems(
  items: AgingItem[],
  opts: {
    debouncedSearch: string;
    filterTab: FilterTab;
    sortKey: SortKey | null;
    sortDir: SortDir;
    pinnedIds?: ReadonlySet<string>;
    priceBand?: PriceBand;
  },
): AgingItem[] {
  const { debouncedSearch, filterTab, sortKey, sortDir, priceBand = 'all' } = opts;

  if (opts.pinnedIds && opts.pinnedIds.size > 0) {
    const subset = items.filter(i => opts.pinnedIds!.has(i.purchase.id));
    return orderItems(subset, sortKey, sortDir);
  }

  let result = applySearchAndTab(items, debouncedSearch, filterTab);

  if (priceBand !== 'all') {
    result = result.filter(i => matchesPriceBand(i, priceBand));
  }

  return orderItems(result, sortKey, sortDir);
}
```

(The search-gated `if (!debouncedSearch.trim())` tail is now gone — ordering is decided solely by `sortKey` being null or not.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && npm test -- inventoryCalcs`
Expected: PASS — including the three new column-sort cases and all pre-existing cases. (The existing `filterAndSortItems` tests pass `sortKey: 'days'` and assert only membership/length, so they remain green.)

- [ ] **Step 6: Typecheck**

Run: `cd web && npm run typecheck`
Expected: no errors. (`useInventoryState` still passes a non-null `SortKey`, which is assignable to `SortKey | null`.)

- [ ] **Step 7: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts
git commit -m "fix(inventory): honor chosen sort column regardless of search"
```

---

## Task 3: Add `computePriceBandCounts` (Issue 1 core logic)

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts`
- Test: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts`

`bestPrice` (and thus `priceBandOf`) reads `compSummary.lastSaleCents` first, so the tests drive a card's band by setting that field. Band thresholds (cents): `lt50` < 5000, `50to100` < 10000, `100to250` < 25000, `250to500` < 50000, `gte500` ≥ 50000.

- [ ] **Step 1: Write the failing test**

Add this `describe` block inside the top-level `describe('inventoryCalcs', ...)` in `inventoryCalcs.test.ts` (e.g. after the column-sort block from Task 2). The local `priced` helper attaches a `compSummary` via a double cast — `compSummary` is optional on `AgingItem` and we only need its `lastSaleCents` field for `bestPrice`:

```ts
  describe('computePriceBandCounts', () => {
    // Attach just enough compSummary to drive bestPrice() → priceBandOf().
    function priced(id: string, lastSaleCents: number, dhStatus?: string): AgingItem {
      const item = makeItem({ purchase: { id, dhStatus } });
      return {
        ...item,
        compSummary: { lastSaleCents } as unknown as AgingItem['compSummary'],
      };
    }

    it('buckets items by bestPrice into the preset bands', () => {
      const items = [
        priced('a', 4000),   // lt50
        priced('b', 9000),   // 50to100
        priced('c', 12000),  // 100to250
        priced('d', 30000),  // 250to500
        priced('e', 60000),  // gte500
        priced('f', 70000),  // gte500
      ];
      const counts = computePriceBandCounts(items);
      expect(counts.all).toBe(6);
      expect(counts.lt50).toBe(1);
      expect(counts['50to100']).toBe(1);
      expect(counts['100to250']).toBe(1);
      expect(counts['250to500']).toBe(1);
      expect(counts.gte500).toBe(2);
    });

    it('excludes items with no price from every band', () => {
      const items = [makeItem({ purchase: { id: 'np' } })]; // makeItem sets no compSummary/currentMarket → bestPrice 0
      const counts = computePriceBandCounts(items);
      expect(counts.all).toBe(1);
      expect(counts.lt50 + counts['50to100'] + counts['100to250'] + counts['250to500'] + counts.gte500).toBe(0);
    });

    it('scopes to the active tab when fed applySearchAndTab output', () => {
      const items = [
        priced('listed-hi', 60000, 'listed'),     // dh_listed, gte500
        priced('unlisted-hi', 60000, 'in stock'), // not listed, gte500
      ];
      const globalCounts = computePriceBandCounts(items);
      expect(globalCounts.gte500).toBe(2);

      const dhListedBase = applySearchAndTab(items, '', 'dh_listed');
      const dhListedCounts = computePriceBandCounts(dhListedBase);
      expect(dhListedCounts.gte500).toBe(1);
      expect(dhListedCounts.all).toBe(1);
    });
  });
```

- [ ] **Step 2: Add `computePriceBandCounts` and `applySearchAndTab` to the test imports**

At the top of `inventoryCalcs.test.ts`, add `applySearchAndTab` and `computePriceBandCounts` to the existing import block so it reads:

```ts
import {
  computeInventoryMeta,
  filterAndSortItems,
  applySearchAndTab,
  computePriceBandCounts,
  isReadyToList,
  needsPriceReview,
  wasUnlistedFromDH,
  isSkipped,
  isDHListed,
  isPendingDHMatch,
  isPendingPrice,
} from './inventoryCalcs';
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd web && npm test -- inventoryCalcs`
Expected: FAIL — `computePriceBandCounts is not a function` / import not found.

- [ ] **Step 4: Implement `computePriceBandCounts`**

In `inventoryCalcs.ts`, add this export immediately **below** the existing `computeInventoryMeta` function (it reuses the same `priceBandOf` bucketing):

```ts
/** Count items per price band over a given base set. Pass the output of
    `applySearchAndTab` to get counts scoped to the active tab + search, so each
    `$` pill badge equals the rows clicking it would produce in the current view.
    Items with no price (priceBandOf === null) count toward `all` only. */
export function computePriceBandCounts(items: AgingItem[]): PriceBandCounts {
  const counts: PriceBandCounts = {
    all: items.length,
    lt50: 0,
    '50to100': 0,
    '100to250': 0,
    '250to500': 0,
    gte500: 0,
  };
  for (const item of items) {
    const band = priceBandOf(item);
    if (band) counts[band]++;
  }
  return counts;
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && npm test -- inventoryCalcs`
Expected: PASS — all three new `computePriceBandCounts` cases plus everything prior.

- [ ] **Step 6: Typecheck**

Run: `cd web && npm run typecheck`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts
git commit -m "feat(inventory): add tab-scoped computePriceBandCounts"
```

---

## Task 4: Widen `SortableHeader.currentKey` to `SortKey | null`

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/SortableHeader.tsx`

Done before the hook switches `sortKey` to `null` (Task 5) so the type flows cleanly. With `currentKey === null`, `active` is `false` for every header → no phantom ▲/▼ until the user clicks.

- [ ] **Step 1: Widen the prop type**

In `SortableHeader.tsx`, change the `currentKey` field in `SortableHeaderProps` from:

```ts
  currentKey: SortKey;
```

to:

```ts
  currentKey: SortKey | null;
```

No other change is needed — `active = currentKey === sortKey` already yields `false` when `currentKey` is `null` (and `sortKey` is always a real key), so the arrow span renders only for the actively-sorted column.

- [ ] **Step 2: Typecheck**

Run: `cd web && npm run typecheck`
Expected: no errors. (`InventoryTab` passes `currentKey={sortKey}` where `sortKey` is still `SortKey` from the hook — assignable to `SortKey | null`.)

- [ ] **Step 3: Build to confirm nothing else references the old prop type**

Run: `cd web && npm run build`
Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/SortableHeader.tsx
git commit -m "refactor(inventory): allow null currentKey on SortableHeader"
```

---

## Task 5: Wire the hook — null default `sortKey` + scoped `priceBandCounts`

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts`
- Test: `web/src/react/pages/campaign-detail/inventory/useInventoryState.test.ts`

- [ ] **Step 1: Write the failing tests**

Add this `describe` block to `useInventoryState.test.ts`, after the existing `describe('useInventoryState — default filter tab', ...)` block:

```ts
describe('useInventoryState — sorting and price-band counts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMeta.current = {
      reviewStats: { total: 0, reviewed: 0, flagged: 0, aging60d: 0 },
      tabCounts: {
        needs_attention: 0,
        awaiting_intake: 0,
        pending_dh_match: 0,
        pending_price: 0,
        ready_to_list: 0,
        dh_listed: 0,
        skipped: 0,
        in_hand: 0,
        all: 0,
      },
      summary: { totalCost: 0, totalMarket: 0, totalPL: 0 },
    };
  });

  it('defaults sortKey to null (smart urgency order)', () => {
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useInventoryState(EMPTY_ITEMS, 'camp-1'), { wrapper });
    expect(result.current.sortKey).toBeNull();
  });

  it('handleSort sets the key ascending, then toggles to descending on repeat', () => {
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useInventoryState(EMPTY_ITEMS, 'camp-1'), { wrapper });

    act(() => { result.current.handleSort('cost'); });
    expect(result.current.sortKey).toBe('cost');
    expect(result.current.sortDir).toBe('asc');

    act(() => { result.current.handleSort('cost'); });
    expect(result.current.sortDir).toBe('desc');
  });

  it('scopes priceBandCounts.all to the active tab', async () => {
    // One DH-listed item, one not. priceBandCounts.all should track the
    // tab-scoped base set, not the full inventory.
    const items = [
      mockItem({ id: 'listed', dhStatus: 'listed' }),
      mockItem({ id: 'unlisted', dhStatus: 'in stock' }),
    ];
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useInventoryState(items, 'camp-1'), { wrapper });

    // Smart default resolves to "all" (needs_attention is 0) → both items in scope.
    await waitFor(() => {
      expect(result.current.filterTab).toBe('all');
    });
    expect(result.current.priceBandCounts.all).toBe(2);

    // Switching to DH Listed narrows the count to the single listed item.
    act(() => { result.current.setFilterTab('dh_listed'); });
    expect(result.current.priceBandCounts.all).toBe(1);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- useInventoryState`
Expected: FAIL — `sortKey` is currently `'name'` not `null`; and `priceBandCounts.all` reflects the mocked `computeInventoryMeta` (which has no `priceBandCounts`, so `.all` is `undefined`).

- [ ] **Step 3: Default `sortKey` to null**

In `useInventoryState.ts`, change the sort-key state initializer from:

```ts
  const [sortKey, setSortKey] = useState<SortKey>('name');
```

to:

```ts
  const [sortKey, setSortKey] = useState<SortKey | null>(null);
```

- [ ] **Step 4: Source `priceBandCounts` from a tab+search-scoped memo**

In `useInventoryState.ts`, find the meta memo that currently destructures `priceBandCounts`:

```ts
  const { reviewStats, tabCounts, priceBandCounts, summary } = useMemo(
    () => computeInventoryMeta(items),
    [items],
  );
```

Change it to stop taking `priceBandCounts` from there:

```ts
  const { reviewStats, tabCounts, summary } = useMemo(
    () => computeInventoryMeta(items),
    [items],
  );

  // Price-band badge counts are scoped to the active tab + search so each `$`
  // pill's number equals the rows clicking it would produce in the current view.
  const priceBandCounts = useMemo(
    () => computePriceBandCounts(applySearchAndTab(items, debouncedSearch, filterTab)),
    [items, debouncedSearch, filterTab],
  );
```

- [ ] **Step 5: Update the imports in the hook**

In `useInventoryState.ts`, the existing import pulls helpers from `./inventoryCalcs`:

```ts
import { computeInventoryMeta, computeTotals, filterAndSortItems } from './inventoryCalcs';
```

Add the two new helpers:

```ts
import { computeInventoryMeta, computeTotals, filterAndSortItems, applySearchAndTab, computePriceBandCounts } from './inventoryCalcs';
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd web && npm test -- useInventoryState`
Expected: PASS — all three new cases plus the pre-existing `handleListOnDH` / `handleBulkListOnDH` / default-filter-tab suites. (The `./inventoryCalcs` mock spreads `...original`, so `applySearchAndTab` and `computePriceBandCounts` run real.)

- [ ] **Step 7: Typecheck**

Run: `cd web && npm run typecheck`
Expected: no errors. (`sortKey` is now `SortKey | null`; it feeds `filterAndSortItems` opts (`SortKey | null` ✓), `SortableHeader.currentKey` (`SortKey | null` ✓), and `handleSort(key: SortKey)` which only ever receives a real key from a header click.)

- [ ] **Step 8: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/useInventoryState.ts web/src/react/pages/campaign-detail/inventory/useInventoryState.test.ts
git commit -m "fix(inventory): null default sortKey + tab-scoped price-band counts"
```

---

## Task 6: Full verification gate

**Files:** none (verification only)

- [ ] **Step 1: Run the full frontend test suite**

Run: `cd web && npm test`
Expected: PASS — entire Vitest suite green.

- [ ] **Step 2: Typecheck**

Run: `cd web && npm run typecheck`
Expected: no errors.

- [ ] **Step 3: Lint**

Run: `cd web && npm run lint`
Expected: no new errors in the four touched source files. (If the repo's baseline has pre-existing warnings elsewhere, do not fix them — out of scope.)

- [ ] **Step 4: Production build**

Run: `cd web && npm run build`
Expected: build succeeds.

- [ ] **Step 5: Manual smoke (optional but recommended)**

Run the dev server (`cd web && npm run dev`), open `/inventory`, and verify:
1. Click the **Cost** / **P/L** / **Status** headers → rows reorder; the ▲/▼ appears only on the clicked column and toggles on a second click.
2. No sort arrow is shown on any column on first load.
3. Select **DH Listed**, then look at the `$` pills → each badge count matches the number of DH-listed rows in that band (and clicking a `$` pill narrows within DH Listed).

- [ ] **Step 6: Final confirmation commit (only if Step 5 surfaced a tweak)**

If everything passed with no changes, there is nothing to commit here. Otherwise:

```bash
git add -A
git commit -m "chore(inventory): post-verification tweaks"
```

---

## Notes for the implementer

- **Do not** modify `computeInventoryMeta`'s returned `priceBandCounts` field or its signature — it stays exported and tested; the hook simply no longer reads it. (Removing it is explicitly out of scope per the spec.)
- **Do not** add a "reset to smart sort" button — out of scope. Explicit-sort mode persists until reload, matching the existing asc/desc toggle pattern.
- The `compSummary` double-cast in the Task 3 tests (`as unknown as AgingItem['compSummary']`) is deliberate: we only need `lastSaleCents` to drive `bestPrice`, and building a full `CompSummary` would be noise.
- Keep `inventoryCalcs.ts` under the 500-line soft / 600-line hard file-size budget (`make check`). It is well under today; these additions are small.
