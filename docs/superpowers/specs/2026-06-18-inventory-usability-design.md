# Inventory Page Usability Fixes — Design

**Date:** 2026-06-18
**Status:** Approved (design)
**Scope:** Frontend only (`web/src/react/pages/campaign-detail/inventory/`)

## Context

"The inventory page" is the global inventory page at route `/inventory`:
`GlobalInventoryPage.tsx` → `<InventoryTab items={items} showCampaignColumn />`.
`GlobalInventoryPage` is the **sole** importer of `InventoryTab`. (The component code
lives under a stale-named `campaign-detail/` folder; there is no campaign-detail
inventory tab in the UI anymore.)

Two usability defects, both confirmed by reading the code:

### Issue 1 — `$` threshold pill counts ignore the selected tab

The header has two pill groups:
- **Secondary filter tabs** (`secondary` array in `InventoryHeader.tsx`): `All`,
  `DH Listed`, `Pending DH Match`, `Pending Price`, `Skipped`. Exactly one active.
- **`$` price-band pills** (`priceBands` array): `<$50`, `$50–100`, `$100–250`,
  `$250–500`, `$500+`. Composes on top of the active tab + search.

The **filtering already composes correctly** — `filterAndSortItems` applies the tab/
search filter first, then `if (priceBand !== 'all') result = result.filter(matchesPriceBand)`.

The bug is the **counts**: `priceBandCounts` is computed once over the *entire*
`items` array in `computeInventoryMeta(items)`, independent of `filterTab` and search.
So selecting `DH Listed` and reading the `$500+` badge shows the count across *all*
inventory, not just DH-listed items. A pill can show a non-zero badge that yields zero
rows, and the show/hide-at-0 logic (`if (band.count === 0 && !isActive) return null`)
keys off the wrong number.

### Issue 2 — clicking a column header does not sort

`InventoryTab.tsx` renders `SortableHeader` for Card / Gr / Cost / List-Rec / P/L /
Status. Clicking calls `handleSort(key)`, which updates `sortKey`/`sortDir` state and
flips the ▲/▼ indicator. But `filterAndSortItems` ends with:

```ts
if (!debouncedSearch.trim()) {
  return [...result].sort(reviewUrgencySort);   // ignores sortKey/sortDir
}
return sortItems(result, sortKey, sortDir);
```

With **no active search** (the normal case) the user's chosen column is ignored and the
list always falls back to `reviewUrgencySort` (flagged → large_gap → no_data →
needs_review → reviewed, then oldest-first). The arrow moves; the rows never reorder.

A secondary symptom: `sortKey` defaults to `'name'`, so the ▲ indicator sits
permanently on the **Card** header even though rows are not name-sorted — a lying
indicator.

## Goals

1. `$` pill badge counts reflect the active tab **and** active search, so each badge
   equals the number of rows clicking it would produce.
2. Clicking a column header actually sorts the visible rows by that column, toggling
   asc↔desc on repeat clicks; the ▲/▼ indicator honestly reflects the real order.
3. Preserve the valuable default "smart urgency" order when the user has not chosen a
   column.

## Non-Goals

- No "reset to smart sort" affordance after the user picks a column. Explicit-sort mode
  persists until reload (matches the existing asc/desc-only toggle pattern). Possible
  future follow-up.
- Tab badge counts (`All`, `DH Listed`, …) stay global — they are top-level facets and
  should show totals, not tab-scoped numbers.
- No change to price-band *filtering* (already correct), bucket thresholds, or the
  `bestPrice`-based bucketing metric.
- No backend changes.

## Design

### Fix 1 — tab+search-scoped price-band counts

**`inventoryCalcs.ts`:**

1. Extract the search/tab selection currently inlined in `filterAndSortItems` into a
   pure helper:

   ```ts
   export function applySearchAndTab(
     items: AgingItem[],
     debouncedSearch: string,
     filterTab: FilterTab,
   ): AgingItem[]
   ```

   This is the existing branch chain (search wins over tab; `in_hand`/`all` = no
   narrowing; otherwise the per-tab predicate switch). `filterAndSortItems` is refactored
   to call it, so there is exactly one definition of "the base set for the current view"
   — no logic duplication between filtering and counting.

2. Add a counting function over a given base set:

   ```ts
   export function computePriceBandCounts(base: AgingItem[]): PriceBandCounts
   ```

   Buckets via the existing `priceBandOf`. Returns the same `PriceBandCounts` shape
   (`all` + five bands).

**`useInventoryState.ts`:**

Stop sourcing `priceBandCounts` from `computeInventoryMeta`. Instead:

```ts
const priceBandCounts = useMemo(
  () => computePriceBandCounts(applySearchAndTab(items, debouncedSearch, filterTab)),
  [items, debouncedSearch, filterTab],
);
```

`computeInventoryMeta` keeps returning its global `priceBandCounts` field (still under
test); the hook simply stops reading it. Lowest-risk — no change to `computeInventoryMeta`
signature or its other consumers (e.g. `DashboardPage` reads `tabCounts` only).

**Edge case — active band hidden by a tab switch:** if a `$` band is active and the
user switches to a tab where that band now has 0 items, the band's count becomes 0 but
it stays visible because it is active (`!isActive` guard), and the table correctly shows
"No cards in this view." This is acceptable and consistent with today's active-pill
behavior. No auto-reset of `priceBand` on tab change.

### Fix 2 — honor the chosen sort column

**`utils.ts`:** no type change needed to `SortKey`/`SortDir`. Introduce `null` at the
*state* level to mean "no explicit column → smart order."

**`useInventoryState.ts`:**

```ts
const [sortKey, setSortKey] = useState<SortKey | null>(null);  // was useState<SortKey>('name')
```

`handleSort` already toggles dir when the same key is clicked and resets to `asc` on a
new key; with the `null` start, the first click on any header sets that key + `asc`.

**`inventoryCalcs.ts` — `filterAndSortItems`:** replace the search-gated final sort with
an explicit-vs-smart decision that is independent of search. Centralize it in one helper
so both the normal path and the early-return **pinned** path share it:

```ts
function orderItems(items, sortKey, sortDir) {
  // explicit column chosen → sort by it; otherwise smart urgency order
  return sortKey == null
    ? [...items].sort(reviewUrgencySort)
    : sortItems(items, sortKey, sortDir);
}
```

- `opts.sortKey` widens to `SortKey | null`.
- The **pinned** early-return branch currently does `return sortItems(subset, sortKey,
  sortDir)` — it must become `return orderItems(subset, sortKey, sortDir)` so a `null`
  sortKey is valid there too (otherwise it's a type error passing `null` into
  `sortItems`, whose signature stays `SortKey`).
- The final return becomes `return orderItems(result, sortKey, sortDir)`.

Search no longer changes which sorter runs — a search with no chosen column still shows
urgency order (unchanged from today for the no-search path; for the *with-search* path
this means default order is now urgency instead of name-asc, which is the more useful
default and consistent across both paths).

**`SortableHeader.tsx`:** widen `currentKey: SortKey` → `currentKey: SortKey | null`.
`active = currentKey === sortKey` then yields `false` for every header when `currentKey`
is `null`, so **no arrow shows initially** (kills the phantom ▲ on Card) and the arrow
appears only on the column actually sorted.

## Data Flow

```
user clicks header
  → handleSort(key)               [useInventoryState]
  → setSortKey(key)/toggle dir
  → filteredAndSortedItems memo recomputes
  → filterAndSortItems(items, { …, sortKey, sortDir })
       applySearchAndTab → priceBand filter → (sortKey==null ? urgency : sortItems)
  → virtualizer re-renders rows in new order
  → SortableHeader shows ▲/▼ only on the active column

user clicks a $ pill / switches tab
  → priceBandCounts memo recomputes over applySearchAndTab(items, search, tab)
  → each $ badge = rows that pill would yield within the active tab+search
```

## Testing

- **`inventoryCalcs.test.ts`**
  - `applySearchAndTab`: returns the right subset for `all`, `dh_listed`, a search query
    (search overrides tab), and `in_hand` alias = all.
  - `computePriceBandCounts`: over a hand-built set, band buckets match `priceBandOf`;
    scoping it to a `dh_listed` base yields different numbers than the global set.
  - `filterAndSortItems` with `sortKey: null` → urgency order (existing expectation).
  - `filterAndSortItems` with `sortKey: 'cost'`, `dir: 'asc'`/`'desc'`, **no search** →
    rows ordered by cost basis (the regression-guard for Issue 2).
  - `filterAndSortItems` with `sortKey: 'name'` + a search query → name order (sort still
    applies under search).
- **`useInventoryState.test.ts`**
  - default `sortKey` is `null`.
  - `handleSort('cost')` → `sortKey='cost'`, `dir='asc'`; again → `'desc'`.
  - `priceBandCounts` recomputes when `filterTab` changes (assert a band count differs
    between `all` and a narrower tab on a seeded item set).
- **Build/lint gate:** `npm run build`, `npm test`, `npm run lint`/typecheck all green
  before declaring done.

## Files Touched

| File | Change |
|---|---|
| `inventory/inventoryCalcs.ts` | extract `applySearchAndTab`; add `computePriceBandCounts`; sort honors `sortKey \| null` |
| `inventory/useInventoryState.ts` | `sortKey` defaults to `null`; `priceBandCounts` from scoped memo |
| `inventory/SortableHeader.tsx` | `currentKey` type widened to `SortKey \| null` |
| `inventory/inventoryCalcs.test.ts` | new/updated assertions (above) |
| `inventory/useInventoryState.test.ts` | new/updated assertions (above) |

No backend, no API, no type-sync (`web/src/types/`) changes.

## Risks

- **`reviewUrgencySort` default unchanged for no-search** — preserved exactly; only the
  with-search default order shifts from name-asc to urgency, which is acceptable/better.
- **Type widening to `SortKey | null`** touches `SortableHeader` props and
  `filterAndSortItems` opts; both are internal to the inventory folder, contained blast
  radius.
- **`computeInventoryMeta.priceBandCounts` becomes unused by the hook** but is still
  exported/tested; leaving it avoids churn in `computeInventoryMeta` and its `tabCounts`
  consumers. (If a reviewer prefers, it can be dropped in a follow-up — out of scope.)
