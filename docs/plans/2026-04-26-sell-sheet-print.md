# Sell Sheet Print Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current internal-facing printed sell sheet with a vendor-facing layout that shows card identity, CL price, last-sale comp, a Code 128 barcode, and a blank "Agreed $" column for the vendor to fill in.

**Architecture:** Print-only rewrite, all client-side under `web/`. A new `SellSheetPrintRow` component renders a printed row from an `AgingItem`. `InventoryTab` swaps to a print-mode view (header strip + `SellSheetPrintRow` list + footer totals) when `isPrinting` is true. Existing on-screen rendering is untouched. Code 128 barcodes are rendered inline as SVG via `jsbarcode`.

**Tech Stack:** React + TypeScript, existing `@media print` CSS, `jsbarcode` (~10KB, MIT) for Code 128 SVG. No backend changes.

**Source spec:** `docs/specs/2026-04-26-sell-sheet-print-design.md`

**Reference data sources (already on `AgingItem`):**
- `item.purchase.cardName`, `item.purchase.certNumber`, `item.purchase.setName`, `item.purchase.cardNumber`, `item.purchase.grader`, `item.purchase.gradeValue`, `item.purchase.clValueCents`
- `item.currentMarket?.lastSoldCents`, `item.currentMarket?.lastSoldDate`
- `item.recommendedPriceCents` (used for the `~` fallback)

---

### Task 1: Add `jsbarcode` dependency

**Files:**
- Modify: `web/package.json`

- [ ] **Step 1: Install dependency**

```bash
cd web && npm install jsbarcode@^3.11.6
```

- [ ] **Step 2: Verify it landed in package.json**

Run: `grep jsbarcode web/package.json`
Expected: a line like `"jsbarcode": "^3.11.6"` under `dependencies`.

- [ ] **Step 3: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "feat(sell-sheet-print): add jsbarcode for Code 128 barcodes"
```

---

### Task 2: Add `clPriceDisplayCents` helper

**Files:**
- Modify: `web/src/react/utils/sellSheetHelpers.tsx`
- Test: `web/src/react/utils/sellSheetHelpers.test.tsx` (create if missing)

- [ ] **Step 1: Write the failing test**

Append to (or create) `web/src/react/utils/sellSheetHelpers.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { clPriceDisplayCents } from './sellSheetHelpers';

describe('clPriceDisplayCents', () => {
  it('returns CL value when present', () => {
    expect(clPriceDisplayCents({ clValueCents: 27900, recommendedPriceCents: 25000 })).toEqual({ cents: 27900, estimated: false });
  });
  it('falls back to recommended price with estimated flag when CL missing', () => {
    expect(clPriceDisplayCents({ clValueCents: 0, recommendedPriceCents: 18500 })).toEqual({ cents: 18500, estimated: true });
  });
  it('returns null when both are missing', () => {
    expect(clPriceDisplayCents({ clValueCents: 0, recommendedPriceCents: 0 })).toBeNull();
    expect(clPriceDisplayCents({ clValueCents: 0 })).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/react/utils/sellSheetHelpers.test.tsx`
Expected: FAIL with "clPriceDisplayCents is not a function" or import error.

- [ ] **Step 3: Implement the helper**

Add to `web/src/react/utils/sellSheetHelpers.tsx` (at the end of the file):

```tsx
/**
 * Resolve the CL price to display on the printed sell sheet.
 * Returns null when neither CL nor recommended price is available.
 * `estimated: true` means the value came from the recommended price fallback
 * and should be rendered with a `~` prefix.
 */
export function clPriceDisplayCents(
  src: { clValueCents?: number; recommendedPriceCents?: number },
): { cents: number; estimated: boolean } | null {
  if (src.clValueCents && src.clValueCents > 0) {
    return { cents: src.clValueCents, estimated: false };
  }
  if (src.recommendedPriceCents && src.recommendedPriceCents > 0) {
    return { cents: src.recommendedPriceCents, estimated: true };
  }
  return null;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/react/utils/sellSheetHelpers.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/react/utils/sellSheetHelpers.tsx web/src/react/utils/sellSheetHelpers.test.tsx
git commit -m "feat(sell-sheet-print): add clPriceDisplayCents helper with ~estimate fallback"
```

---

### Task 3: Add `formatLastSaleDate` helper

**Files:**
- Modify: `web/src/react/utils/sellSheetHelpers.tsx`
- Test: `web/src/react/utils/sellSheetHelpers.test.tsx`

- [ ] **Step 1: Write the failing test**

Append to `web/src/react/utils/sellSheetHelpers.test.tsx`:

```tsx
import { formatLastSaleDate } from './sellSheetHelpers';

describe('formatLastSaleDate', () => {
  it('formats ISO date as MM/DD/YY', () => {
    expect(formatLastSaleDate('2026-03-12T00:00:00Z')).toBe('03/12/26');
  });
  it('formats date-only ISO', () => {
    expect(formatLastSaleDate('2026-03-12')).toBe('03/12/26');
  });
  it('returns empty string for missing input', () => {
    expect(formatLastSaleDate(undefined)).toBe('');
    expect(formatLastSaleDate('')).toBe('');
  });
  it('returns empty string for unparseable input', () => {
    expect(formatLastSaleDate('not a date')).toBe('');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/react/utils/sellSheetHelpers.test.tsx`
Expected: FAIL with "formatLastSaleDate is not a function".

- [ ] **Step 3: Implement the helper**

Add to `web/src/react/utils/sellSheetHelpers.tsx`:

```tsx
/**
 * Format an ISO date as MM/DD/YY for the printed last-sale column.
 * Returns '' for missing/unparseable input.
 */
export function formatLastSaleDate(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const mm = String(d.getUTCMonth() + 1).padStart(2, '0');
  const dd = String(d.getUTCDate()).padStart(2, '0');
  const yy = String(d.getUTCFullYear()).slice(-2);
  return `${mm}/${dd}/${yy}`;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/react/utils/sellSheetHelpers.test.tsx`
Expected: PASS (4 new tests + the 3 from Task 2).

- [ ] **Step 5: Commit**

```bash
git add web/src/react/utils/sellSheetHelpers.tsx web/src/react/utils/sellSheetHelpers.test.tsx
git commit -m "feat(sell-sheet-print): add formatLastSaleDate helper"
```

---

### Task 4: Create `SellSheetPrintRow` component

**Files:**
- Create: `web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.tsx`
- Test: `web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import SellSheetPrintRow from './SellSheetPrintRow';
import type { AgingItem } from '../../../../types/campaigns';

const baseItem: AgingItem = {
  purchase: {
    id: 'p1',
    campaignId: 'c1',
    cardName: 'GOLEM HOLO',
    certNumber: '133487731',
    cardNumber: '76',
    setName: 'Pokemon Japanese Vending',
    grader: 'PSA',
    gradeValue: 4,
    clValueCents: 27900,
    buyCostCents: 0,
    purchaseDate: '2026-01-01',
    phase: 'in_stock',
  } as AgingItem['purchase'],
  daysHeld: 30,
  currentMarket: { lastSoldCents: 26500, lastSoldDate: '2026-03-12', gradePriceCents: 0 },
  recommendedPriceCents: 25000,
};

describe('SellSheetPrintRow', () => {
  it('renders title-cased card name and subtitle', () => {
    render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(screen.getByText('Golem Holo')).toBeInTheDocument();
    expect(screen.getByText(/Pokemon Japanese Vending · #76/)).toBeInTheDocument();
  });

  it('renders cert number and a barcode svg', () => {
    const { container } = render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(screen.getByText('133487731')).toBeInTheDocument();
    expect(container.querySelector('svg.sell-sheet-print-barcode')).not.toBeNull();
  });

  it('renders CL price without ~ when CL is present', () => {
    render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(screen.getByText('$279')).toBeInTheDocument();
  });

  it('renders ~ prefix when CL missing and recommended price is used', () => {
    const item = { ...baseItem, purchase: { ...baseItem.purchase, clValueCents: 0 } };
    render(<SellSheetPrintRow item={item} rowNumber={1} />);
    expect(screen.getByText('~$250')).toBeInTheDocument();
  });

  it('renders em-dash when both CL and recommended are missing', () => {
    const item = {
      ...baseItem,
      purchase: { ...baseItem.purchase, clValueCents: 0 },
      recommendedPriceCents: 0,
    };
    render(<SellSheetPrintRow item={item} rowNumber={1} />);
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('renders last-sale price and date when present', () => {
    render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(screen.getByText('$265')).toBeInTheDocument();
    expect(screen.getByText('03/12/26')).toBeInTheDocument();
  });

  it('leaves last-sale cell blank when no last sale data', () => {
    const item = { ...baseItem, currentMarket: undefined };
    const { container } = render(<SellSheetPrintRow item={item} rowNumber={1} />);
    expect(container.querySelector('[data-cell="last-sale"]')?.textContent).toBe('');
  });

  it('renders the row number and an empty Agreed $ cell', () => {
    const { container } = render(<SellSheetPrintRow item={baseItem} rowNumber={7} />);
    expect(screen.getByText('7')).toBeInTheDocument();
    const agreed = container.querySelector('[data-cell="agreed"]');
    expect(agreed?.textContent ?? '').toBe('');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/react/pages/campaign-detail/inventory/SellSheetPrintRow.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the component**

Create `web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.tsx`:

```tsx
import { useEffect, useRef } from 'react';
import JsBarcode from 'jsbarcode';
import type { AgingItem } from '../../../../types/campaigns';
import {
  formatCardName,
  clPriceDisplayCents,
  formatLastSaleDate,
} from '../../../utils/sellSheetHelpers';

interface Props {
  item: AgingItem;
  rowNumber: number;
}

function dollars(cents: number): string {
  return `$${Math.round(cents / 100).toLocaleString('en-US')}`;
}

function subtitle(setName?: string, cardNumber?: string): string {
  const parts: string[] = [];
  if (setName) parts.push(setName);
  if (cardNumber) parts.push(`#${cardNumber}`);
  return parts.join(' · ');
}

function gradeLabel(grader: string | undefined, gradeValue: number): string {
  const prefix = grader && grader !== 'PSA' ? grader : 'PSA';
  return `${prefix} ${gradeValue}`;
}

export default function SellSheetPrintRow({ item, rowNumber }: Props) {
  const { purchase, currentMarket, recommendedPriceCents } = item;
  const barcodeRef = useRef<SVGSVGElement | null>(null);

  useEffect(() => {
    if (!barcodeRef.current || !purchase.certNumber) return;
    JsBarcode(barcodeRef.current, purchase.certNumber, {
      format: 'CODE128',
      width: 1.4,
      height: 24,
      displayValue: false,
      margin: 0,
    });
  }, [purchase.certNumber]);

  const cl = clPriceDisplayCents({
    clValueCents: purchase.clValueCents,
    recommendedPriceCents,
  });
  const clText = cl
    ? (cl.estimated ? `~${dollars(cl.cents)}` : dollars(cl.cents))
    : '—';

  const lastSoldCents = currentMarket?.lastSoldCents ?? 0;
  const lastSoldDate = formatLastSaleDate(currentMarket?.lastSoldDate);

  return (
    <div className="sell-sheet-print-row">
      <div className="sell-sheet-print-cell" data-cell="num">{rowNumber}</div>
      <div className="sell-sheet-print-cell" data-cell="card">
        <div className="sell-sheet-print-name">{formatCardName(purchase.cardName)}</div>
        <div className="sell-sheet-print-sub">{subtitle(purchase.setName, purchase.cardNumber)}</div>
      </div>
      <div className="sell-sheet-print-cell" data-cell="grade">
        {gradeLabel(purchase.grader, purchase.gradeValue)}
      </div>
      <div className="sell-sheet-print-cell" data-cell="cert">
        <div className="sell-sheet-print-cert">{purchase.certNumber}</div>
        {purchase.certNumber && (
          <svg ref={barcodeRef} className="sell-sheet-print-barcode" />
        )}
      </div>
      <div className="sell-sheet-print-cell" data-cell="cl">{clText}</div>
      <div className="sell-sheet-print-cell" data-cell="last-sale">
        {lastSoldCents > 0 && (
          <>
            <div>{dollars(lastSoldCents)}</div>
            {lastSoldDate && <div className="sell-sheet-print-date">{lastSoldDate}</div>}
          </>
        )}
      </div>
      <div className="sell-sheet-print-cell" data-cell="agreed" />
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/react/pages/campaign-detail/inventory/SellSheetPrintRow.test.tsx`
Expected: PASS (8 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.tsx web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.test.tsx
git commit -m "feat(sell-sheet-print): add SellSheetPrintRow with Code 128 barcode"
```

---

### Task 5: Rewrite `print-sell-sheet.css` for the new layout

**Files:**
- Modify: `web/src/styles/print-sell-sheet.css`

- [ ] **Step 1: Replace the file with the new print rules**

Overwrite `web/src/styles/print-sell-sheet.css` with:

```css
/* Print-only sell-sheet styles. No effect on screen view. */

@media print {
  /* Hide all chrome */
  header,
  nav,
  .print\:hidden,
  [class*="print:hidden"],
  .sell-sheet-no-print {
    display: none !important;
  }

  /* Hide the on-screen inventory table entirely while printing */
  .glass-table {
    display: none !important;
  }

  /* Reset page chrome */
  body,
  main,
  #main-content {
    background: white !important;
    color: black !important;
    padding: 0 !important;
    margin: 0 !important;
  }

  @page {
    size: letter landscape;
    margin: 0.4in;
  }

  /* Print container, header strip, footer strip */
  .sell-sheet-print {
    display: block !important;
    font-size: 8pt;
    color: black;
  }

  .sell-sheet-print-header {
    display: flex !important;
    align-items: flex-end;
    justify-content: space-between;
    border-bottom: 2px solid #333;
    padding-bottom: 4px;
    margin-bottom: 6px;
  }

  .sell-sheet-print-header h1 {
    font-size: 13pt;
    font-weight: bold;
    margin: 0;
  }

  .sell-sheet-print-header .meta {
    font-size: 8pt;
    color: #555;
    text-align: right;
  }

  /* Column header row, repeats on each printed page via display:table-header-group on .sell-sheet-print-thead */
  .sell-sheet-print-thead {
    display: table-header-group;
  }

  .sell-sheet-print-headrow,
  .sell-sheet-print-row {
    display: grid;
    grid-template-columns: 28px 1fr 56px 140px 64px 80px 80px;
    align-items: start;
    column-gap: 6px;
    padding: 3px 4px;
    border-bottom: 1px solid #ddd;
    break-inside: avoid;
  }

  .sell-sheet-print-headrow {
    font-weight: bold;
    border-bottom: 1.5px solid #333;
    background: white;
  }

  /* Alternating row bands for scanning ease */
  .sell-sheet-print-row:nth-child(even) {
    background: #f5f5f5;
  }

  .sell-sheet-print-cell[data-cell="num"]      { text-align: right; color: #666; }
  .sell-sheet-print-cell[data-cell="grade"]    { text-align: center; }
  .sell-sheet-print-cell[data-cell="cl"]       { text-align: right; font-variant-numeric: tabular-nums; }
  .sell-sheet-print-cell[data-cell="last-sale"]{ text-align: right; font-variant-numeric: tabular-nums; }
  .sell-sheet-print-cell[data-cell="agreed"]   {
    border: 1px solid #999;
    background: white;
    height: 28px;
  }

  .sell-sheet-print-name {
    font-size: 9pt;
    font-weight: 600;
  }
  .sell-sheet-print-sub {
    font-size: 7.5pt;
    color: #555;
  }
  .sell-sheet-print-cert {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 8pt;
  }
  .sell-sheet-print-barcode {
    width: 120px;
    height: 24px;
    display: block;
  }
  .sell-sheet-print-date {
    color: #555;
    font-size: 7pt;
  }

  /* Footer totals strip (last page only) */
  .sell-sheet-print-footer {
    display: block !important;
    margin-top: 12px;
    padding-top: 6px;
    border-top: 1px solid #999;
    font-size: 8pt;
  }
  .sell-sheet-print-footer .totals-row {
    display: flex;
    justify-content: space-between;
    gap: 16px;
    margin: 2px 0;
  }
  .sell-sheet-print-footer .totals-row .label {
    color: #555;
  }
  .sell-sheet-print-footer .blank-line {
    display: inline-block;
    border-bottom: 1px solid #999;
    min-width: 100px;
    height: 14px;
  }
  .sell-sheet-print-footer .note {
    margin-top: 8px;
    font-size: 7pt;
    color: #666;
    text-align: center;
  }
}

/* Hide print-only nodes on screen */
.sell-sheet-print,
.sell-sheet-print-header,
.sell-sheet-print-footer {
  display: none;
}
```

- [ ] **Step 2: Verify file syntax**

Run: `cd web && npx stylelint src/styles/print-sell-sheet.css || true`
Expected: no parse errors. (If stylelint isn't installed, skip — the build step in Task 7 will catch syntax errors.)

- [ ] **Step 3: Commit**

```bash
git add web/src/styles/print-sell-sheet.css
git commit -m "feat(sell-sheet-print): rewrite print CSS for vendor layout"
```

---

### Task 6: Wire print mode into `InventoryTab`

**Files:**
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx`

- [ ] **Step 1: Add the print imports and a sort helper**

At the top of `web/src/react/pages/campaign-detail/InventoryTab.tsx` (alongside existing imports), add:

```tsx
import SellSheetPrintRow from './inventory/SellSheetPrintRow';
import { clPriceDisplayCents } from '../../utils/sellSheetHelpers';
```

- [ ] **Step 2: Add the print-sort + totals helper above the component**

Insert above `export default function InventoryTab(`:

```tsx
function sortForPrint(items: AgingItem[]): AgingItem[] {
  const score = (i: AgingItem) =>
    clPriceDisplayCents({
      clValueCents: i.purchase.clValueCents,
      recommendedPriceCents: i.recommendedPriceCents,
    })?.cents ?? 0;
  return [...items].sort((a, b) => {
    if (b.purchase.gradeValue !== a.purchase.gradeValue) {
      return b.purchase.gradeValue - a.purchase.gradeValue;
    }
    return score(b) - score(a);
  });
}

function clTotalCents(items: AgingItem[]): number {
  return items.reduce((sum, i) => {
    const cl = clPriceDisplayCents({
      clValueCents: i.purchase.clValueCents,
      recommendedPriceCents: i.recommendedPriceCents,
    });
    return sum + (cl?.cents ?? 0);
  }, 0);
}
```

- [ ] **Step 3: Render the print view when `isPrinting` is true**

Find the `return (` for the component (currently `return ( <div> <InventoryHeader ... />`). Immediately after the `<InventoryHeader ... />` block and before the `{isMobile && sellSheetActive ? (...)` block, insert:

```tsx
{isPrinting && (
  <div className="sell-sheet-print">
    <div className="sell-sheet-print-header">
      <h1>SlabLedger Sell Sheet</h1>
      <div className="meta">
        <div>Generated: {new Date().toLocaleDateString('en-US')}</div>
        <div>{filteredAndSortedItems.length} cards</div>
      </div>
    </div>
    <div className="sell-sheet-print-thead">
      <div className="sell-sheet-print-headrow">
        <div className="sell-sheet-print-cell" data-cell="num">#</div>
        <div className="sell-sheet-print-cell" data-cell="card">Card</div>
        <div className="sell-sheet-print-cell" data-cell="grade">Grade</div>
        <div className="sell-sheet-print-cell" data-cell="cert">Cert</div>
        <div className="sell-sheet-print-cell" data-cell="cl">CL Price</div>
        <div className="sell-sheet-print-cell" data-cell="last-sale">Last Sale</div>
        <div className="sell-sheet-print-cell" data-cell="agreed">Agreed $</div>
      </div>
    </div>
    {sortForPrint(filteredAndSortedItems).map((item, idx) => (
      <SellSheetPrintRow key={item.purchase.id} item={item} rowNumber={idx + 1} />
    ))}
    <div className="sell-sheet-print-footer">
      <div className="totals-row">
        <span><span className="label">Items:</span> {filteredAndSortedItems.length}</span>
        <span>
          <span className="label">CL Price total:</span>{' '}
          ${(Math.round(clTotalCents(filteredAndSortedItems) / 100)).toLocaleString('en-US')}
        </span>
      </div>
      <div className="totals-row">
        <span className="label">Agreed total:</span>
        <span className="blank-line" />
      </div>
      <div className="totals-row">
        <span className="label">Offer %:</span>
        <span className="blank-line" style={{ minWidth: 60 }} />
      </div>
      <div className="totals-row">
        <span className="label">Offer $:</span>
        <span className="blank-line" />
      </div>
      <div className="note">
        CL price reflects most-recent CardLadder market value (~ = estimate from our recommended price).
        Last Sale shows our most recent realized sale where available.
      </div>
    </div>
  </div>
)}
```

- [ ] **Step 4: Hide the on-screen view branches when printing**

Wrap the existing mobile-and-desktop branches (the `{isMobile && sellSheetActive ? ( ... ) : isMobile ? ( ... ) : ( <div className="glass-table"> ... </div> )}` block) by prefixing with `{!isPrinting && (` and closing `)}` after the final `</div>` of the desktop branch.

(The CSS already hides `.glass-table` in `@media print`, but suppressing the React tree avoids running the virtualizer with a 0-height container.)

- [ ] **Step 5: Run frontend typecheck and tests**

```bash
cd web && npm run typecheck && npx vitest run src/react/pages/campaign-detail/inventory/SellSheetPrintRow.test.tsx src/react/utils/sellSheetHelpers.test.tsx
```

Expected: typecheck passes, all tests pass.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "feat(sell-sheet-print): swap to vendor print view in InventoryTab"
```

---

### Task 7: End-to-end verification

**Files:** none modified — verification only.

- [ ] **Step 1: Run the full frontend build**

```bash
cd web && npm run build
```

Expected: build succeeds with no TS or CSS errors.

- [ ] **Step 2: Run the full frontend test suite**

```bash
cd web && npm test -- --run
```

Expected: all tests pass.

- [ ] **Step 3: Manual print preview check**

Start the dev server and exercise a print:

```bash
cd web && npm run dev
```

Open a campaign with ~20 inventory items, select some onto the sell sheet, click **Print Sell Sheet**, and in the browser print dialog:

- Confirm the layout matches the spec (7 columns: #, Card, Grade, Cert, CL Price, Last Sale, Agreed $).
- Confirm card names are title-cased (e.g. `Golem Holo`, not `GOLEM HOLO`).
- Confirm a Code 128 barcode renders under the cert number on every row.
- Confirm rows with no CL price show `~$nnn` from the recommended price.
- Confirm rows with no recommended price either show `—`.
- Confirm last-sale price + date appears where available, blank otherwise.
- Confirm rows are sorted Grade DESC, then CL Price DESC.
- Confirm the column header repeats at the top of each printed page.
- Confirm the totals strip appears once at the very end.
- Confirm cost / list-rec / DH / Sell columns and the toast banner do **not** appear in print.
- Confirm the on-screen view is unchanged when not printing.

- [ ] **Step 4: Commit any verification-driven tweaks (only if needed)**

If the manual check finds a layout issue, fix in the relevant file and commit with a message like:

```bash
git commit -am "fix(sell-sheet-print): <specific tweak>"
```

If everything looks good, no additional commit is needed.

---

## Self-review notes

- All seven spec sections (columns, fallback rule, sort, header, footer, barcode, missing-data) are covered by Tasks 4–6.
- Helpers are unit-tested in Tasks 2–3; the row component is unit-tested in Task 4.
- Type names used across tasks (`AgingItem`, `clPriceDisplayCents`, `formatLastSaleDate`, `SellSheetPrintRow`) are consistent.
- No placeholders: every step shows the exact code, file path, and command.
- No backend / domain changes; all edits are under `web/`.
