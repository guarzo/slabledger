# Sell Sheet Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken, ephemeral sell sheet page with a persistent "Sell Sheet" tab in the inventory view, backed by localStorage.

**Architecture:** New `useSellSheet` hook stores purchase IDs in localStorage with cross-tab sync. The existing `InventoryTab` component gains a "Sell Sheet" filter tab that displays only items whose IDs are in the sell sheet. The separate `/sell-sheet` route and `SellSheetPrintPage` are deleted. Print uses `@media print` CSS on the sell sheet tab.

**Tech Stack:** React 18, TypeScript, Vitest, @testing-library/react, Vite, localStorage, CSS @media print

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `web/src/react/hooks/useSellSheet.ts` | Create | localStorage-backed sell sheet state hook |
| `web/src/react/hooks/useSellSheet.test.ts` | Create | Unit tests for the hook |
| `web/src/react/hooks/index.ts` | Modify | Export the new hook |
| `web/src/react/pages/campaign-detail/InventoryTab.tsx` | Modify | Add sell sheet tab, change bulk actions, add indicator |
| `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx` | Modify | Accept + display sell sheet indicator |
| `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx` | Modify | Accept + display sell sheet indicator |
| `web/src/react/pages/GlobalInventoryPage.tsx` | Modify | Remove PrintableSellSheet, simplify print logic |
| `web/src/react/App.tsx` | Modify | Remove `/sell-sheet` route |
| `web/src/react/pages/SellSheetPrintPage.tsx` | Delete | No longer needed |
| `web/src/styles/print-sell-sheet.css` | Create | Print-specific CSS for sell sheet tab |

---

### Task 1: Create `useSellSheet` Hook

**Files:**
- Create: `web/src/react/hooks/useSellSheet.ts`
- Create: `web/src/react/hooks/useSellSheet.test.ts`
- Modify: `web/src/react/hooks/index.ts`

- [ ] **Step 1: Write the failing tests**

Create `web/src/react/hooks/useSellSheet.test.ts`:

```typescript
import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useSellSheet } from './useSellSheet';

describe('useSellSheet', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('initializes with empty set when localStorage is empty', () => {
    const { result } = renderHook(() => useSellSheet());
    expect(result.current.count).toBe(0);
    expect(result.current.has('abc')).toBe(false);
  });

  it('initializes from existing localStorage data', () => {
    localStorage.setItem('sellSheetIds', JSON.stringify(['id1', 'id2']));
    const { result } = renderHook(() => useSellSheet());
    expect(result.current.count).toBe(2);
    expect(result.current.has('id1')).toBe(true);
    expect(result.current.has('id2')).toBe(true);
  });

  it('adds items', () => {
    const { result } = renderHook(() => useSellSheet());
    act(() => result.current.add(['a', 'b']));
    expect(result.current.count).toBe(2);
    expect(result.current.has('a')).toBe(true);
    expect(result.current.has('b')).toBe(true);
    expect(JSON.parse(localStorage.getItem('sellSheetIds')!)).toEqual(['a', 'b']);
  });

  it('does not duplicate existing items on add', () => {
    const { result } = renderHook(() => useSellSheet());
    act(() => result.current.add(['a', 'b']));
    act(() => result.current.add(['b', 'c']));
    expect(result.current.count).toBe(3);
  });

  it('removes items', () => {
    const { result } = renderHook(() => useSellSheet());
    act(() => result.current.add(['a', 'b', 'c']));
    act(() => result.current.remove(['b']));
    expect(result.current.count).toBe(2);
    expect(result.current.has('b')).toBe(false);
    expect(result.current.has('a')).toBe(true);
  });

  it('clears all items', () => {
    const { result } = renderHook(() => useSellSheet());
    act(() => result.current.add(['a', 'b']));
    act(() => result.current.clear());
    expect(result.current.count).toBe(0);
    expect(JSON.parse(localStorage.getItem('sellSheetIds')!)).toEqual([]);
  });

  it('handles corrupted localStorage gracefully', () => {
    localStorage.setItem('sellSheetIds', 'not-json');
    const { result } = renderHook(() => useSellSheet());
    expect(result.current.count).toBe(0);
  });

  it('syncs across hooks via storage event', () => {
    const { result: hook1 } = renderHook(() => useSellSheet());
    const { result: hook2 } = renderHook(() => useSellSheet());
    act(() => hook1.current.add(['x']));
    // The custom 'local-storage' event dispatched by the hook keeps same-tab hooks in sync
    expect(hook2.current.has('x')).toBe(true);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /workspace/web && npx vitest run src/react/hooks/useSellSheet.test.ts`
Expected: FAIL — `useSellSheet` module not found

- [ ] **Step 3: Implement the hook**

Create `web/src/react/hooks/useSellSheet.ts`:

```typescript
import { useCallback, useMemo } from 'react';
import { useLocalStorage } from './useLocalStorage';

const STORAGE_KEY = 'sellSheetIds';

export interface SellSheetHook {
  /** Set of purchase IDs currently on the sell sheet */
  items: Set<string>;
  /** Add purchase IDs to the sell sheet */
  add: (ids: string[]) => void;
  /** Remove purchase IDs from the sell sheet */
  remove: (ids: string[]) => void;
  /** Clear all items from the sell sheet */
  clear: () => void;
  /** Check if a purchase ID is on the sell sheet */
  has: (id: string) => boolean;
  /** Number of items on the sell sheet */
  count: number;
}

export function useSellSheet(): SellSheetHook {
  const [storedIds, setStoredIds] = useLocalStorage<string[]>(STORAGE_KEY, []);

  const itemsSet = useMemo(() => new Set(storedIds), [storedIds]);

  const add = useCallback((ids: string[]) => {
    setStoredIds(prev => {
      const set = new Set(prev);
      for (const id of ids) set.add(id);
      return Array.from(set);
    });
  }, [setStoredIds]);

  const remove = useCallback((ids: string[]) => {
    setStoredIds(prev => {
      const toRemove = new Set(ids);
      return prev.filter(id => !toRemove.has(id));
    });
  }, [setStoredIds]);

  const clear = useCallback(() => {
    setStoredIds([]);
  }, [setStoredIds]);

  const has = useCallback((id: string) => itemsSet.has(id), [itemsSet]);

  return { items: itemsSet, add, remove, clear, has, count: itemsSet.size };
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /workspace/web && npx vitest run src/react/hooks/useSellSheet.test.ts`
Expected: All 8 tests PASS

- [ ] **Step 5: Export the hook from index**

Modify `web/src/react/hooks/index.ts` — add this line after the existing exports:

```typescript
export { useSellSheet } from './useSellSheet';
```

- [ ] **Step 6: Commit**

```bash
git add web/src/react/hooks/useSellSheet.ts web/src/react/hooks/useSellSheet.test.ts web/src/react/hooks/index.ts
git commit -m "feat: add useSellSheet hook with localStorage persistence"
```

---

### Task 2: Add "Sell Sheet" Filter Tab to InventoryTab

**Files:**
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx`

This task modifies three things in `InventoryTab.tsx`:
1. Widen the `filterTab` union type to include `'sell_sheet'`
2. Add the sell sheet tab to the tab bar with a count badge
3. Add sell sheet filtering logic in `filteredAndSortedItems`

- [ ] **Step 1: Import the hook and add sell sheet to filterTab type**

In `web/src/react/pages/campaign-detail/InventoryTab.tsx`:

Add import at the top (after existing hook imports around line 8):
```typescript
import { useSellSheet } from '../../hooks/useSellSheet';
```

Change the `filterTab` state declaration (line 59) from:
```typescript
const [filterTab, setFilterTab] = useState<'needs_review' | 'large_gap' | 'no_data' | 'flagged' | 'card_show' | 'all'>('needs_review');
```
to:
```typescript
const [filterTab, setFilterTab] = useState<'needs_review' | 'large_gap' | 'no_data' | 'flagged' | 'card_show' | 'all' | 'sell_sheet'>('needs_review');
```

- [ ] **Step 2: Initialize the sell sheet hook**

Add this line after the existing `useToast()` call (after line 37):
```typescript
const sellSheet = useSellSheet();
```

- [ ] **Step 3: Add sell sheet filtering to `filteredAndSortedItems`**

In the `filteredAndSortedItems` `useMemo` (around line 137), add a sell sheet filter case. Change the filter-by-tab block from:
```typescript
    } else if (!showAll) {
      // Filter by active tab using getReviewStatus
      if (filterTab !== 'all') {
        result = result.filter(i => {
```
to:
```typescript
    } else if (!showAll) {
      // Filter by active tab using getReviewStatus
      if (filterTab === 'sell_sheet') {
        result = result.filter(i => sellSheet.has(i.purchase.id));
      } else if (filterTab !== 'all') {
        result = result.filter(i => {
```

Also add `sellSheet` to the `useMemo` dependency array (line 201). Change:
```typescript
  }, [items, debouncedSearch, sortKey, sortDir, evMap, showAll, filterTab]);
```
to:
```typescript
  }, [items, debouncedSearch, sortKey, sortDir, evMap, showAll, filterTab, sellSheet]);
```

- [ ] **Step 4: Add the "Sell Sheet" tab button to the tab bar**

In the filter tabs section (around line 467-498), add the sell sheet tab after the "All" tab. Change the closing of the tab map from:
```typescript
          ] as const).map(tab => (
```

The simplest approach: add a sell sheet tab button after the `.map()` closing. Find the closing `</button>` + `))}` around line 497, and add after `))}`  but before the closing `</div>`:

```typescript
          <button
            key="sell_sheet"
            type="button"
            onClick={() => setFilterTab('sell_sheet')}
            className={`text-xs font-medium px-3 py-1.5 rounded-full border transition-colors ${
              filterTab === 'sell_sheet'
                ? 'border-[var(--brand-500)] bg-[var(--brand-500)]/10 text-[var(--brand-400)]'
                : 'border-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
            }`}
          >
            Sell Sheet
            <span
              className="ml-1.5 inline-block min-w-[18px] text-center text-[10px] font-semibold px-1 py-[1px] rounded-full"
              style={{
                background: filterTab === 'sell_sheet' ? 'color-mix(in srgb, var(--brand-400) 15%, transparent)' : 'rgba(255,255,255,0.06)',
                color: filterTab === 'sell_sheet' ? 'var(--brand-400)' : 'var(--text-muted)',
              }}
            >
              {sellSheet.count}
            </span>
          </button>
```

- [ ] **Step 5: Add empty state for sell sheet tab**

After the search results count display (around line 501-505), add an empty state that shows when the sell sheet tab is active but has no items. Add this before the `{isMobile ? (` block:

```typescript
      {filterTab === 'sell_sheet' && filteredAndSortedItems.length === 0 && !debouncedSearch && (
        <div className="text-center py-12">
          <div className="text-[var(--text-muted)] text-sm">No items on your sell sheet.</div>
          <div className="text-[var(--text-muted)] text-xs mt-1">Select items from any tab and click &ldquo;Add to Sell Sheet&rdquo;.</div>
        </div>
      )}
```

- [ ] **Step 6: Verify the app builds**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 7: Commit**

```bash
git add web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "feat: add Sell Sheet filter tab to inventory"
```

---

### Task 3: Modify Bulk Action Buttons

**Files:**
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx`

This task replaces the old "Sell Sheet" bulk button (sessionStorage + window.open) with "Add to Sell Sheet" on regular tabs, and shows "Remove from Sell Sheet" + "Print" on the sell sheet tab.

- [ ] **Step 1: Replace the bulk action buttons**

In `web/src/react/pages/campaign-detail/InventoryTab.tsx`, find the `{selected.size > 0 && (` block (lines 420-449). Replace the entire block from `{selected.size > 0 && (` through its closing `)}` with:

```typescript
      {selected.size > 0 && (
        <div className="flex items-center justify-between mb-3">
          <span className="text-sm text-[var(--text-muted)]">{selected.size} selected</span>
          <div className="flex items-center gap-2">
            {filterTab === 'sell_sheet' ? (
              <>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => {
                    sellSheet.remove(Array.from(selected));
                    setSelected(new Set());
                    toast.success(`Removed ${selected.size} item${selected.size > 1 ? 's' : ''} from sell sheet`);
                  }}
                >
                  Remove from Sell Sheet ({selected.size})
                </Button>
                <Button
                  size="sm"
                  onClick={() => openSaleModal(items.filter(i => selected.has(i.purchase.id)))}
                >
                  Record Sale ({selected.size})
                </Button>
              </>
            ) : (
              <>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => {
                    sellSheet.add(Array.from(selected));
                    setSelected(new Set());
                    toast.success(`Added ${selected.size} item${selected.size > 1 ? 's' : ''} to sell sheet`);
                  }}
                >
                  Add to Sell Sheet ({selected.size})
                </Button>
                <Button
                  size="sm"
                  onClick={() => openSaleModal(items.filter(i => selected.has(i.purchase.id)))}
                >
                  Record Sale ({selected.size})
                </Button>
              </>
            )}
          </div>
        </div>
      )}
```

- [ ] **Step 2: Add Print button for sell sheet tab**

Add a print button that appears when the sell sheet tab is active (regardless of selection). Add this right after the bulk action block's closing `)}` and before `{/* Crack Candidates Banner */}`:

```typescript
      {filterTab === 'sell_sheet' && sellSheet.count > 0 && (
        <div className="flex justify-end mb-3">
          <Button
            size="sm"
            variant="secondary"
            onClick={() => window.print()}
          >
            Print Sell Sheet
          </Button>
        </div>
      )}
```

- [ ] **Step 3: Verify the app builds**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "feat: replace sell sheet bulk actions with add/remove/print"
```

---

### Task 4: Add Sell Sheet Indicator to DesktopRow and MobileCard

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`
- Modify: `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx`

- [ ] **Step 1: Add `onSellSheet` prop to DesktopRow**

In `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`, add `onSellSheet?: boolean;` to the `DesktopRowProps` interface (after the `showCampaignColumn?` prop):

```typescript
interface DesktopRowProps {
  item: AgingItem;
  selected: boolean;
  onToggle: () => void;
  onExpand: () => void;
  onRecordSale: () => void;
  onFixPricing?: () => void;
  onSetPrice?: () => void;
  ev?: ExpectedValue;
  showEV?: boolean;
  showCampaignColumn?: boolean;
  onSellSheet?: boolean;
}
```

Destructure it in the function signature (line 42). Change:
```typescript
export default function DesktopRow({ item, selected, onToggle, onExpand, onRecordSale, onFixPricing, onSetPrice, ev, showEV, showCampaignColumn }: DesktopRowProps) {
```
to:
```typescript
export default function DesktopRow({ item, selected, onToggle, onExpand, onRecordSale, onFixPricing, onSetPrice, ev, showEV, showCampaignColumn, onSellSheet }: DesktopRowProps) {
```

- [ ] **Step 2: Render the sell sheet indicator icon in DesktopRow**

Find the card name `<span>` element (around line 93):
```typescript
          <span className="text-[var(--text)] truncate">
            {hotSeller && <span className="text-amber-400 mr-1" title="High demand">&#9733;</span>}
            {item.purchase.cardName}
          </span>
```

Add the sell sheet indicator after the hot seller star, before the card name:
```typescript
          <span className="text-[var(--text)] truncate">
            {hotSeller && <span className="text-amber-400 mr-1" title="High demand">&#9733;</span>}
            {onSellSheet && <span className="text-gray-400 mr-1 text-xs" title="On sell sheet">&#9864;</span>}
            {item.purchase.cardName}
          </span>
```

Note: `&#9864;` is Unicode U+2688 (a tag/label icon). If that doesn't render well, use a simple SVG tag icon or the text `[S]`. The implementer should verify it renders correctly and swap if needed.

- [ ] **Step 3: Add `onSellSheet` prop to MobileCard**

In `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx`, add `onSellSheet?: boolean;` to the `MobileCardProps` interface:

```typescript
interface MobileCardProps {
  item: AgingItem;
  selected: boolean;
  onToggle: () => void;
  onRecordSale: () => void;
  onFixPricing?: () => void;
  onSetPrice?: () => void;
  ev?: ExpectedValue;
  showCampaignColumn?: boolean;
  onSellSheet?: boolean;
}
```

Destructure it in the function signature (line 22). Change:
```typescript
export default function MobileCard({ item, selected, onToggle, onRecordSale, onFixPricing, onSetPrice, ev, showCampaignColumn }: MobileCardProps) {
```
to:
```typescript
export default function MobileCard({ item, selected, onToggle, onRecordSale, onFixPricing, onSetPrice, ev, showCampaignColumn, onSellSheet }: MobileCardProps) {
```

- [ ] **Step 4: Render the sell sheet indicator icon in MobileCard**

Find the card name section (around line 41):
```typescript
            <div className="text-sm font-medium text-[var(--text)]">
              {hotSeller && <span className="text-amber-400 mr-1" title="High demand">★</span>}
              {item.purchase.cardName}
```

Change to:
```typescript
            <div className="text-sm font-medium text-[var(--text)]">
              {hotSeller && <span className="text-amber-400 mr-1" title="High demand">★</span>}
              {onSellSheet && <span className="text-gray-400 mr-1 text-xs" title="On sell sheet">&#9864;</span>}
              {item.purchase.cardName}
```

- [ ] **Step 5: Pass `onSellSheet` from InventoryTab**

In `web/src/react/pages/campaign-detail/InventoryTab.tsx`, pass the prop to both `DesktopRow` and `MobileCard`.

Find the `DesktopRow` render (around line 591) and add the prop. Change:
```typescript
                      <DesktopRow
                        item={item}
                        selected={isSelected}
                        onToggle={() => toggleSelect(item.purchase.id)}
                        onExpand={() => toggleExpand(item.purchase.id)}
                        onRecordSale={() => openSaleModal([item])}
                        onFixPricing={() => handleFixPricing(item.purchase)}
                        onSetPrice={() => handleSetPrice(item)}
                        ev={evMap.get(item.purchase.certNumber)}
                        showEV={!!showEV}
                        showCampaignColumn={showCampaignColumn}
                      />
```
to:
```typescript
                      <DesktopRow
                        item={item}
                        selected={isSelected}
                        onToggle={() => toggleSelect(item.purchase.id)}
                        onExpand={() => toggleExpand(item.purchase.id)}
                        onRecordSale={() => openSaleModal([item])}
                        onFixPricing={() => handleFixPricing(item.purchase)}
                        onSetPrice={() => handleSetPrice(item)}
                        ev={evMap.get(item.purchase.certNumber)}
                        showEV={!!showEV}
                        showCampaignColumn={showCampaignColumn}
                        onSellSheet={filterTab !== 'sell_sheet' && sellSheet.has(item.purchase.id)}
                      />
```

Find the `MobileCard` render (around line 529) and add the same prop. Change:
```typescript
                    <MobileCard
                      item={item}
                      selected={selected.has(item.purchase.id)}
                      onToggle={() => toggleSelect(item.purchase.id)}
                      onRecordSale={() => openSaleModal([item])}
                      onFixPricing={() => handleFixPricing(item.purchase)}
                      onSetPrice={() => handleSetPrice(item)}
                      ev={evMap.get(item.purchase.certNumber)}
                      showCampaignColumn={showCampaignColumn}
                    />
```
to:
```typescript
                    <MobileCard
                      item={item}
                      selected={selected.has(item.purchase.id)}
                      onToggle={() => toggleSelect(item.purchase.id)}
                      onRecordSale={() => openSaleModal([item])}
                      onFixPricing={() => handleFixPricing(item.purchase)}
                      onSetPrice={() => handleSetPrice(item)}
                      ev={evMap.get(item.purchase.certNumber)}
                      showCampaignColumn={showCampaignColumn}
                      onSellSheet={filterTab !== 'sell_sheet' && sellSheet.has(item.purchase.id)}
                    />
```

Note: `filterTab !== 'sell_sheet'` ensures the indicator is hidden when viewing the sell sheet tab itself (redundant there since every item shown is on the sell sheet).

- [ ] **Step 6: Verify the app builds**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 7: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx web/src/react/pages/campaign-detail/inventory/MobileCard.tsx web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "feat: add sell sheet indicator icon to inventory rows"
```

---

### Task 5: Add Print CSS for Sell Sheet Tab

**Files:**
- Create: `web/src/styles/print-sell-sheet.css`
- Modify: `web/src/react/pages/GlobalInventoryPage.tsx`

- [ ] **Step 1: Create print CSS**

Create `web/src/styles/print-sell-sheet.css`:

```css
@media print {
  /* Hide all chrome: nav, header, sidebar, controls */
  header,
  nav,
  .print\\:hidden,
  [class*="print:hidden"] {
    display: none !important;
  }

  /* Hide inventory controls that shouldn't print */
  .sell-sheet-no-print {
    display: none !important;
  }

  /* Reset page background */
  body,
  main,
  #main-content {
    background: white !important;
    color: black !important;
    padding: 0 !important;
    margin: 0 !important;
  }

  /* Print header — injected above inventory table when printing */
  .sell-sheet-print-header {
    display: flex !important;
    align-items: center;
    justify-content: space-between;
    padding: 8px 0;
    margin-bottom: 8px;
    border-bottom: 2px solid #333;
  }

  .sell-sheet-print-header h1 {
    font-size: 14pt;
    font-weight: bold;
    color: black;
  }

  .sell-sheet-print-header .print-meta {
    font-size: 8pt;
    color: #666;
  }

  /* Make table fit the page */
  .glass-table {
    border: none !important;
    background: none !important;
    box-shadow: none !important;
  }

  .glass-table-header {
    border-bottom: 2px solid #333 !important;
    background: none !important;
  }

  .glass-table-th {
    color: #333 !important;
    font-size: 8pt !important;
  }

  .glass-table-td {
    font-size: 8pt !important;
    color: black !important;
    padding: 2px 4px !important;
  }

  .glass-vrow {
    border-bottom: 1px solid #ddd !important;
    background: none !important;
    break-inside: avoid;
  }

  /* Remove virtual scroll container height constraint */
  .glass-table > div[style*="max-height"] {
    max-height: none !important;
    overflow: visible !important;
  }

  /* Print footer */
  .sell-sheet-print-footer {
    display: block !important;
    margin-top: 12px;
    padding-top: 8px;
    border-top: 1px solid #999;
    font-size: 7pt;
    color: #666;
    text-align: center;
  }
}

/* Hide print-only elements on screen */
.sell-sheet-print-header,
.sell-sheet-print-footer {
  display: none;
}
```

- [ ] **Step 2: Import the print CSS in GlobalInventoryPage**

In `web/src/react/pages/GlobalInventoryPage.tsx`, add at the top of the file (after other imports):

```typescript
import '../../styles/print-sell-sheet.css';
```

- [ ] **Step 3: Simplify GlobalInventoryPage — remove PrintableSellSheet**

The current `GlobalInventoryPage.tsx` has a `PrintableSellSheet` component and a print-only `<div>`. Remove both. Also remove the `SellSheetMobileCard` and `SellSheetTable` local components, the print button in the header, and the related imports.

Replace the entire file content with:

```typescript
import { useSellSheet } from '../hooks/useSellSheet';
import { useGlobalInventory } from '../queries/useCampaignQueries';
import InventoryTab from './campaign-detail/InventoryTab';
import AIAnalysisWidget from '../components/advisor/AIAnalysisWidget';
import '../../styles/print-sell-sheet.css';

export default function GlobalInventoryPage() {
  const { data: items = [], isLoading, isError, error } = useGlobalInventory();
  const sellSheet = useSellSheet();

  if (isError) {
    return (
      <div className="max-w-6xl mx-auto px-4 text-center py-16">
        <p className="text-[var(--danger)] mb-4">{error instanceof Error ? error.message : 'Failed to load inventory'}</p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="px-4 py-2 bg-[var(--brand-500)] text-white rounded-lg text-sm font-medium hover:bg-[var(--brand-600)] transition-colors"
        >
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-4">
      {/* Print header — visible only when printing */}
      <div className="sell-sheet-print-header">
        <h1>Sell Sheet</h1>
        <div className="print-meta">
          {new Date().toLocaleDateString()} &middot; {sellSheet.count} items
        </div>
      </div>

      <div className="print:hidden">
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Inventory</h1>
        </div>
      </div>

      <InventoryTab items={items} isLoading={isLoading} showCampaignColumn />

      {/* Print footer — visible only when printing */}
      <div className="sell-sheet-print-footer">
        {sellSheet.count} items &middot; {new Date().toLocaleDateString()} &middot; card-yeti.com
      </div>

      {/* AI Liquidation Analysis — hidden when printing */}
      <div className="mt-6 print:hidden">
        <AIAnalysisWidget
          endpoint="liquidation-analysis"
          cacheType="liquidation"
          title="Liquidation Analysis"
          buttonLabel="Analyze Liquidation"
          description="Identify cards where selling now — even below market — frees capital more efficiently than holding. Factors in credit pressure, carrying costs, trends, and liquidity."
          collapsible
        />
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Verify the app builds**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 5: Commit**

```bash
git add web/src/styles/print-sell-sheet.css web/src/react/pages/GlobalInventoryPage.tsx
git commit -m "feat: add print CSS and simplify GlobalInventoryPage"
```

---

### Task 6: Remove `/sell-sheet` Route and Delete SellSheetPrintPage

**Files:**
- Modify: `web/src/react/App.tsx`
- Delete: `web/src/react/pages/SellSheetPrintPage.tsx`

- [ ] **Step 1: Remove the route and import from App.tsx**

In `web/src/react/App.tsx`:

Remove the lazy import (line 33):
```typescript
const SellSheetPrintPage = lazy(() => import('./pages/SellSheetPrintPage'));
```

Remove the route block (lines 143-147):
```typescript
              {/* Sell Sheet Print */}
              <Route path="/sell-sheet" element={
                <ProtectedRoute>
                  <SellSheetPrintPage />
                </ProtectedRoute>
              } />
```

- [ ] **Step 2: Delete the SellSheetPrintPage file**

```bash
rm web/src/react/pages/SellSheetPrintPage.tsx
```

- [ ] **Step 3: Verify the app builds**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Run all existing frontend tests to check for regressions**

Run: `cd /workspace/web && npx vitest run`
Expected: All tests pass (including the new `useSellSheet` tests)

- [ ] **Step 5: Commit**

```bash
git add web/src/react/App.tsx
git rm web/src/react/pages/SellSheetPrintPage.tsx
git commit -m "chore: remove /sell-sheet route and delete SellSheetPrintPage"
```

---

### Task 7: Verify End-to-End and Clean Up

**Files:**
- Verify: all changed files

- [ ] **Step 1: Run full test suite**

Run: `cd /workspace/web && npx vitest run`
Expected: All tests pass

- [ ] **Step 2: Run typecheck**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 3: Run lint**

Run: `cd /workspace/web && npx eslint . --ext .ts,.tsx`
Expected: No errors (warnings acceptable)

- [ ] **Step 4: Run the build**

Run: `cd /workspace/web && npx vite build`
Expected: Build succeeds

- [ ] **Step 5: Check for leftover references to old sell sheet**

Search for any remaining references to the old pattern:

```bash
grep -r "sessionStorage.*sellSheet\|window\.open.*sell-sheet\|/sell-sheet\|SellSheetPrintPage\|useGlobalSellSheet" web/src/ --include="*.ts" --include="*.tsx"
```

Expected: No matches (except possibly in `api/campaigns.ts` where the API methods are defined — those stay).

- [ ] **Step 6: Verify the Unicode indicator renders**

Start the dev server (`cd /workspace/web && npm run dev`) and visually verify that the `&#9864;` character renders correctly as a tag icon in the browser. If it doesn't render well, replace with a simple inline SVG tag icon or the text indicator `[S]` in both `DesktopRow.tsx` and `MobileCard.tsx`.

- [ ] **Step 7: Final commit if any fixups were needed**

```bash
git add -A
git commit -m "chore: sell sheet redesign cleanup and fixups"
```
