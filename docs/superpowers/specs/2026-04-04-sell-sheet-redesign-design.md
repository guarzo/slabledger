# Sell Sheet Redesign — Design Spec

## Problem

The current sell sheet functionality has three critical UX issues:

1. **Items lost between navigation** — Selection state lives in React `useState`, passed through `sessionStorage` which is immediately cleared when the print page loads. Any navigation destroys the sell sheet.
2. **Terrible layout** — The sell sheet is a print-only page (`/sell-sheet`) opened in a new window with no interactivity beyond printing.
3. **Unnecessary separate page** — The sell sheet concept doesn't warrant its own route; it's fundamentally a filtered view of inventory.

## Solution

Eliminate the separate sell sheet page. Replace it with a persistent "Sell Sheet" tab within the existing inventory tab bar, backed by `localStorage` for state persistence.

## Architecture

### State Management — `useSellSheet` Hook

A new React hook (`web/src/react/hooks/useSellSheet.ts`) manages sell sheet state:

```typescript
useSellSheet() → {
  items: Set<string>          // purchase IDs on the sell sheet
  add(ids: string[])          // add items (from bulk select)
  remove(ids: string[])       // remove items (from sell sheet tab)
  clear()                     // empty the sell sheet
  has(id: string) → boolean   // check if item is on sell sheet
  count: number               // for tab badge
}
```

**Storage:** `localStorage` key `"sellSheetIds"` as a JSON array of purchase ID strings.

**Reactivity:** Uses `useSyncExternalStore` with a `storage` event listener so multiple browser tabs stay in sync.

**No backend calls.** This is purely client-side persistence. The existing inventory data (already loaded via React Query) provides all the card details, market data, and P/L information needed to display sell sheet items.

### UI Changes

#### 1. New "Sell Sheet" Tab

Added as the last tab in the `InventoryTab` filter bar, after "All":

```
Needs Review | Large Gap | No Data | Flagged | Card Show | All | Sell Sheet (3)
```

- Badge shows current item count from `useSellSheet().count`
- When active, filters the inventory list to only show purchases whose IDs are in the sell sheet set
- Uses the same table (desktop) / card (mobile) rendering as all other tabs — no new display components
- Empty state message: "No items on your sell sheet. Select items from any tab and click 'Add to Sell Sheet'."

#### 2. Bulk Action Buttons

**On non-sell-sheet tabs** (when items selected via checkbox):
- **"Add to Sell Sheet (N)"** — replaces the current "Sell Sheet (N)" button. Calls `sellSheet.add(selectedIds)`, shows a brief toast confirmation, clears selection. No `sessionStorage`, no `window.open`.
- **"Record Sale (N)"** — unchanged.

**On the sell sheet tab** (when items selected via checkbox):
- **"Record Sale (N)"** — unchanged.
- **"Remove from Sell Sheet (N)"** — calls `sellSheet.remove(selectedIds)`, items disappear from the tab.
- **"Print"** — triggers `window.print()` with print-optimized CSS.

#### 3. Sell Sheet Indicator on Other Tabs

When viewing any tab other than "Sell Sheet", items that are on the sell sheet display a small shopping-tag icon (or similar subtle glyph) to the left of the card name. Icon should be muted (e.g., `text-gray-400`) so it doesn't compete with the data columns. Uses `sellSheet.has(id)` — no extra data fetching, purely a visual indicator derived from localStorage state.

#### 4. Print Layout

CSS `@media print` rules applied when printing from the sell sheet tab:

- Hides navigation, sidebar, filter tabs, bulk action buttons, checkboxes
- Shows a print header with date and item count
- Reuses existing formatting logic from `sellSheetHelpers.tsx` (margin codes like `[45]`, card name formatting, grade display)
- Hot items (3+ sales in 30d at target price) marked with bold/highlight in-place (not separated into a distinct section)
- Data source: the already-loaded inventory data filtered to sell sheet IDs — no additional API call

### Cleanup — What Gets Removed

- **`SellSheetPrintPage.tsx`** — deleted (the separate `/sell-sheet` page)
- **`/sell-sheet` route** in `App.tsx` — removed
- **`sessionStorage` logic** in `InventoryTab.tsx` — replaced by `useSellSheet` hook
- **`useGlobalSellSheet()` hook usage** in `GlobalInventoryPage.tsx` — the print-only toggle logic removed
- **`window.open('/sell-sheet')` call** — replaced by `sellSheet.add()`

### What Stays Unchanged

- **Backend sell sheet endpoints** — `POST /api/sell-sheet`, `POST /api/portfolio/sell-sheet`, `POST /api/campaigns/{id}/sell-sheet` remain. They're useful for the future "Save to server" feature.
- **`sellSheetHelpers.tsx`** — formatting utilities reused for print layout
- **`RecordSaleModal`** — unchanged, works the same from the sell sheet tab
- **Backend handlers and service methods** — no backend changes needed
- **Database** — no migrations needed

## Scope

**Pure frontend refactor.** No backend changes, no database migrations.

Files to change (~5-7):
- `web/src/react/hooks/useSellSheet.ts` — new file
- `web/src/react/pages/campaign-detail/InventoryTab.tsx` — add tab, modify bulk actions, add indicator
- `web/src/react/pages/GlobalInventoryPage.tsx` — remove print toggle, provide sell sheet hook
- `web/src/react/App.tsx` — remove `/sell-sheet` route
- `web/src/react/pages/SellSheetPrintPage.tsx` — delete
- Print CSS (location TBD — either inline or a dedicated stylesheet)

## Future Enhancement

**Server-side persistence (not in this iteration):**
- New `sell_sheet_items` SQLite table
- API endpoints for CRUD
- `useSellSheet` hook gains an optional "Save" action that syncs localStorage → server
- Enables cross-device access and saved/named sell sheets
