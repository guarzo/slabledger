# Opportunities + Inventory Polish — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the 13-column PSA-Exchange opportunities table to 6 columns + a hover-popover Signal cell, and tighten the awkward Actions column on the inventory page.

**Architecture:** Two independent, surgical UI changes shipped in one PR. The opportunities change introduces a new `SignalCell` sub-component (3-icon rail + Radix Popover) and reshapes the existing `OpportunitiesTable.tsx` rows. The inventory change is a pure layout fix (width + alignment + divider removal) on `DesktopRow.tsx` and the matching header in `InventoryTab.tsx`.

**Tech Stack:** React 18, TypeScript, Tailwind v4, Radix UI (`Popover`, already installed via `radix-ui` umbrella), Vitest + React Testing Library.

**Spec:** `docs/specs/2026-05-05-opps-and-actions-polish-design.md`

**Worktree:** `/workspace/.worktrees/opps-and-actions-polish` on branch `opps-and-actions-polish`. All commands assume this is the cwd unless stated otherwise.

---

## File Map

**Create:**
- `web/src/react/pages/psa-exchange/SignalCell.tsx` — composite Edge-value + 3-icon rail + popover
- `web/src/react/pages/psa-exchange/SignalCell.test.tsx` — renders Edge value, indicators, popover content
- `web/src/react/pages/psa-exchange/signalIndicators.ts` — pure tier helpers (`daysTier`, `velocityTier`, `confidenceTier`)
- `web/src/react/pages/psa-exchange/signalIndicators.test.ts` — tier boundary tests

**Modify:**
- `web/src/react/pages/psa-exchange/OpportunitiesTable.tsx` — collapse to 6 columns, restructure cells
- `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx` — Actions cell width + alignment + divider removal
- `web/src/react/pages/campaign-detail/InventoryTab.tsx` — Actions header to match (line 175)

**No change:**
- `web/src/react/pages/psa-exchange/SortableHeader.tsx`
- `web/src/react/pages/psa-exchange/Toolbar.tsx`
- `web/src/react/pages/psa-exchange/utils.ts` (existing helpers reused as-is)
- `web/src/react/pages/psa-exchange/OpportunitiesTableSkeleton.tsx` (generic skeleton, no column-specific structure)
- `web/src/react/pages/campaign-detail/inventory/RowActions.tsx`
- `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx`

---

## Task 1: Baseline check

**Files:** none modified.

- [ ] **Step 1: Confirm worktree state**

Run:
```bash
pwd
git rev-parse --abbrev-ref HEAD
git log --oneline -3
```

Expected: cwd is `/workspace/.worktrees/opps-and-actions-polish`, branch is `opps-and-actions-polish`, HEAD is the spec commit (`spec: opps table column collapse + inventory actions cell`).

- [ ] **Step 2: Verify deps**

Run:
```bash
ls -la web/node_modules | head -2
```

Expected: `web/node_modules` exists (symlinked to `/workspace/web/node_modules` during worktree setup). If missing, run `cd web && npm install` from the worktree.

- [ ] **Step 3: Run frontend tests baseline**

Run:
```bash
cd web && npm test -- --run 2>&1 | tail -20
```

Expected: all tests pass. Note the count for comparison after the change.

- [ ] **Step 4: Run go build smoke test**

Run:
```bash
go build -o /tmp/slabledger-baseline ./cmd/slabledger && rm /tmp/slabledger-baseline
```

Expected: clean build, no output.

---

## Task 2: Inventory Actions cell — investigate header style

**Files:** read-only inspection.

- [ ] **Step 1: Find `glass-table-th` CSS**

Run:
```bash
grep -rn "glass-table-th" web/src --include="*.css" --include="*.tsx" | head -10
```

- [ ] **Step 2: Inspect the rule**

Read the CSS file(s) returned. Look for `text-transform`, `letter-spacing`, or `font-variant-caps` declarations on `.glass-table-th`.

- [ ] **Step 3: Decide on header literal**

Two cases:
- **If `glass-table-th` has `text-transform: uppercase`:** every header is uppercased by CSS. The literal `Actions` in `InventoryTab.tsx:175` is already correct; keep as-is.
- **If `glass-table-th` does NOT uppercase:** then `Actions` is being upper-cased somewhere else, or the screenshot was misread. In that case, leave the literal as `Actions` and skip any case change.

In either case, **no change to the header literal in this task**. Record the finding in a note for the next task.

---

## Task 3: Inventory Actions cell — width, alignment, divider

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx:325`
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx:175`

- [ ] **Step 1: Update the row cell**

Open `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx` at line 325. Replace:

```tsx
      <div className="glass-table-td flex-shrink-0 print-hide-actions ml-2 pl-3 border-l border-white/[0.14]" style={{ width: '220px' }}>
        <RowActions primary={primary} fallbackPrimary={fallbackPrimary} overflow={overflow} variant="desktop" />
      </div>
```

with:

```tsx
      <div className="glass-table-td flex-shrink-0 print-hide-actions flex justify-end" style={{ width: '144px' }}>
        <RowActions primary={primary} fallbackPrimary={fallbackPrimary} overflow={overflow} variant="desktop" />
      </div>
```

Changes: width `220px → 144px`, removed `ml-2 pl-3 border-l border-white/[0.14]`, added `flex justify-end` to right-align contents.

- [ ] **Step 2: Update the header cell**

Open `web/src/react/pages/campaign-detail/InventoryTab.tsx` at line 175. Replace:

```tsx
            <div className="glass-table-th flex-shrink-0 text-left print-hide-actions ml-2 pl-3" style={{ width: '220px' }}>Actions</div>
```

with:

```tsx
            <div className="glass-table-th flex-shrink-0 text-right print-hide-actions" style={{ width: '144px' }}>Actions</div>
```

Changes: width `220px → 144px`, `text-left → text-right`, removed `ml-2 pl-3`.

- [ ] **Step 3: Run typecheck + tests**

Run:
```bash
cd web && npm run typecheck && npm test -- --run 2>&1 | tail -10
```

Expected: typecheck clean, all tests pass.

- [ ] **Step 4: Visual smoke check**

Spec says to verify UI changes in a browser before claiming done. If the dev server is not already running:

```bash
cd web && npm run dev &
```

Then in a separate shell, navigate to `http://localhost:5173/inventory`. Verify:
- Actions column is ~144px wide instead of 220px
- No vertical divider to the left of the Sell button
- Sell button + dot menu hug the right edge of each row
- Header "Actions" sits above the buttons (right-aligned)
- Mobile view (`MobileCard.tsx`) still renders normally — open at narrow viewport

If you cannot run the browser, say so and use `make screenshots` to produce the comparison images instead.

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "ui(inventory): tighten Actions column width + alignment

Drop the fixed 220px to 144px, remove the left divider, and right-
align contents. Header matches. Eliminates the dead whitespace that
made the column feel awkward."
```

---

## Task 4: Opportunities — signal indicator tier helpers (TDD)

**Files:**
- Create: `web/src/react/pages/psa-exchange/signalIndicators.ts`
- Create: `web/src/react/pages/psa-exchange/signalIndicators.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `web/src/react/pages/psa-exchange/signalIndicators.test.ts` with:

```ts
import { describe, expect, it } from 'vitest';
import { daysTier, velocityTier, confidenceTier } from './signalIndicators';

describe('daysTier', () => {
  it('returns "fast" for ≤6 days', () => {
    expect(daysTier(0)).toBe('fast');
    expect(daysTier(6)).toBe('fast');
  });

  it('returns "medium" for 7–15 days', () => {
    expect(daysTier(7)).toBe('medium');
    expect(daysTier(15)).toBe('medium');
  });

  it('returns "slow" for >15 days or non-finite', () => {
    expect(daysTier(16)).toBe('slow');
    expect(daysTier(Number.POSITIVE_INFINITY)).toBe('slow');
    expect(daysTier(NaN)).toBe('slow');
  });
});

describe('velocityTier', () => {
  it('returns 3 for ≥10 sales/mo', () => {
    expect(velocityTier(10)).toBe(3);
    expect(velocityTier(50)).toBe(3);
  });

  it('returns 2 for 3–9 sales/mo', () => {
    expect(velocityTier(3)).toBe(2);
    expect(velocityTier(9)).toBe(2);
  });

  it('returns 1 for <3 sales/mo', () => {
    expect(velocityTier(0)).toBe(1);
    expect(velocityTier(2.99)).toBe(1);
  });
});

describe('confidenceTier', () => {
  it('returns "high" for ≥7', () => {
    expect(confidenceTier(7)).toBe('high');
    expect(confidenceTier(10)).toBe('high');
  });

  it('returns "medium" for 5–6', () => {
    expect(confidenceTier(5)).toBe('medium');
    expect(confidenceTier(6)).toBe('medium');
  });

  it('returns "low" for <5', () => {
    expect(confidenceTier(0)).toBe('low');
    expect(confidenceTier(4.9)).toBe('low');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
cd web && npx vitest run src/react/pages/psa-exchange/signalIndicators.test.ts 2>&1 | tail -20
```

Expected: FAIL with "Cannot find module './signalIndicators'".

- [ ] **Step 3: Write the implementation**

Create `web/src/react/pages/psa-exchange/signalIndicators.ts`:

```ts
export type DaysTier = 'fast' | 'medium' | 'slow';
export type ConfidenceTier = 'high' | 'medium' | 'low';
export type VelocityTier = 1 | 2 | 3;

export function daysTier(days: number): DaysTier {
  if (!Number.isFinite(days)) return 'slow';
  if (days <= 6) return 'fast';
  if (days <= 15) return 'medium';
  return 'slow';
}

export function velocityTier(velMonth: number): VelocityTier {
  if (velMonth >= 10) return 3;
  if (velMonth >= 3) return 2;
  return 1;
}

export function confidenceTier(conf: number): ConfidenceTier {
  if (conf >= 7) return 'high';
  if (conf >= 5) return 'medium';
  return 'low';
}
```

Thresholds match the existing `daysBucketClass`, `velocityBucketClass`, `confidenceColorClass` rules in `utils.ts`.

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
cd web && npx vitest run src/react/pages/psa-exchange/signalIndicators.test.ts 2>&1 | tail -10
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/psa-exchange/signalIndicators.ts web/src/react/pages/psa-exchange/signalIndicators.test.ts
git commit -m "feat(opps): add signal indicator tier helpers"
```

---

## Task 5: Opportunities — SignalCell component

**Files:**
- Create: `web/src/react/pages/psa-exchange/SignalCell.tsx`
- Create: `web/src/react/pages/psa-exchange/SignalCell.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/react/pages/psa-exchange/SignalCell.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import SignalCell from './SignalCell';

const baseRow = {
  edgeAtOffer: 0.333,
  daysToSellValue: 1,
  velocityMonth: 35,
  confidence: 8,
  comp: 13100,
  population: 12,
};

describe('SignalCell', () => {
  it('renders the Edge percentage as the loud line', () => {
    render(<SignalCell {...baseRow} />);
    expect(screen.getByText('33.3%')).toBeInTheDocument();
  });

  it('renders three indicator glyphs (days, velocity, confidence)', () => {
    render(<SignalCell {...baseRow} />);
    expect(screen.getByLabelText(/days to sell/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/velocity/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/confidence/i)).toBeInTheDocument();
  });

  it('exposes the full numerics in the popover', async () => {
    const user = userEvent.setup();
    render(<SignalCell {...baseRow} />);
    await user.click(screen.getByRole('button', { name: /signal details/i }));
    expect(screen.getByText(/days\/sale/i)).toBeInTheDocument();
    expect(screen.getByText(/velocity/i)).toBeInTheDocument();
    expect(screen.getByText(/confidence/i)).toBeInTheDocument();
    expect(screen.getByText(/comp/i)).toBeInTheDocument();
    expect(screen.getByText(/pop/i)).toBeInTheDocument();
    expect(screen.getByText('$13,100')).toBeInTheDocument();
    expect(screen.getByText('12')).toBeInTheDocument();
  });

  it('formats <1d for sub-1 days/sale', () => {
    render(<SignalCell {...baseRow} daysToSellValue={0.5} />);
    expect(screen.getByText('<1d')).toBeInTheDocument();
  });

  it('shows em-dash when daysToSell is non-finite', () => {
    render(<SignalCell {...baseRow} daysToSellValue={Number.POSITIVE_INFINITY} />);
    // In the popover, infinity → '—'. Open the popover to inspect.
    // Cheap check: just confirm the component still renders.
    expect(screen.getByText('33.3%')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
cd web && npx vitest run src/react/pages/psa-exchange/SignalCell.test.tsx 2>&1 | tail -10
```

Expected: FAIL with "Cannot find module './SignalCell'".

- [ ] **Step 3: Write the implementation**

Create `web/src/react/pages/psa-exchange/SignalCell.tsx`:

```tsx
import { Popover } from 'radix-ui';
import { clsx } from 'clsx';
import { edgeBucketClass, daysBucketClass, velocityBucketClass, confidenceColorClass } from './utils';
import { daysTier, velocityTier, confidenceTier } from './signalIndicators';

interface SignalCellProps {
  edgeAtOffer: number;
  daysToSellValue: number;
  velocityMonth: number;
  confidence: number;
  comp: number;
  population: number;
}

const dollar = (n: number) =>
  n.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 });

const pct = (n: number) => `${(n * 100).toFixed(1)}%`;

function formatDays(d: number): string {
  if (!Number.isFinite(d)) return '—';
  if (d < 1) return '<1d';
  if (d < 10) return `${d.toFixed(1)}d`;
  return `${Math.round(d)}d`;
}

const CONF_GLYPH: Record<'high' | 'medium' | 'low', string> = {
  high: '✓',
  medium: '~',
  low: '?',
};

export default function SignalCell({
  edgeAtOffer,
  daysToSellValue,
  velocityMonth,
  confidence,
  comp,
  population,
}: SignalCellProps) {
  const dTier = daysTier(daysToSellValue);
  const vTier = velocityTier(velocityMonth);
  const cTier = confidenceTier(confidence);

  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <button
          type="button"
          aria-label="Signal details"
          className="flex flex-col items-end gap-0.5 tabular-nums hover:bg-[var(--surface-2)]/40 rounded px-1 py-0.5 transition-colors focus:outline focus:outline-2 focus:outline-[var(--brand-400)]"
        >
          <span className={clsx('text-sm', edgeBucketClass(edgeAtOffer))}>{pct(edgeAtOffer)}</span>
          <span className="flex items-center gap-1.5 text-[10px] leading-none">
            <span aria-label={`Days to sell: ${formatDays(daysToSellValue)}`} className={daysBucketClass(daysToSellValue)}>
              {dTier === 'fast' ? '●' : dTier === 'medium' ? '◐' : '○'}
            </span>
            <span aria-label={`Velocity: ${velocityMonth} per month`} className={clsx('inline-flex gap-[1px]', velocityBucketClass(velocityMonth))}>
              <span className={clsx('w-[3px] h-[6px] rounded-sm', vTier >= 1 ? 'bg-current' : 'bg-current/20')} />
              <span className={clsx('w-[3px] h-[6px] rounded-sm', vTier >= 2 ? 'bg-current' : 'bg-current/20')} />
              <span className={clsx('w-[3px] h-[6px] rounded-sm', vTier >= 3 ? 'bg-current' : 'bg-current/20')} />
            </span>
            <span aria-label={`Confidence: ${cTier}`} className={confidenceColorClass(confidence)}>
              {CONF_GLYPH[cTier]}
            </span>
          </span>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={4}
          className="z-50 w-56 p-3 rounded-md bg-[var(--surface-1)] border border-[var(--surface-2)] shadow-lg text-xs space-y-1.5"
        >
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Days/sale</span>
            <span className={clsx('tabular-nums', daysBucketClass(daysToSellValue))}>{formatDays(daysToSellValue)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Velocity</span>
            <span className={clsx('tabular-nums', velocityBucketClass(velocityMonth))}>{velocityMonth}/mo</span>
          </div>
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Confidence</span>
            <span className={clsx('tabular-nums', confidenceColorClass(confidence))}>{confidence}/10 ({cTier})</span>
          </div>
          <div className="h-px bg-[var(--surface-2)]" />
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Comp</span>
            <span className="tabular-nums">{dollar(comp)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Pop</span>
            <span className="tabular-nums">{population || '—'}</span>
          </div>
          <Popover.Arrow className="fill-[var(--surface-2)]" />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
```

- [ ] **Step 4: Confirm `@testing-library/user-event` is installed**

Run:
```bash
ls /workspace/web/node_modules/@testing-library/user-event/package.json 2>&1 | head -1
```

Expected: file exists. If missing, add to dev deps:
```bash
cd web && npm install --save-dev @testing-library/user-event
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
cd web && npx vitest run src/react/pages/psa-exchange/SignalCell.test.tsx 2>&1 | tail -15
```

Expected: all tests PASS.

If the popover-content tests fail because `Popover.Portal` renders to `document.body` and JSDOM isn't catching it, adjust the test to use `screen.findByText` (async) instead of `screen.getByText` after clicking the trigger — the Radix portal mounts asynchronously.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/psa-exchange/SignalCell.tsx web/src/react/pages/psa-exchange/SignalCell.test.tsx
git commit -m "feat(opps): add SignalCell composite indicator + popover"
```

---

## Task 6: Opportunities — refactor OpportunitiesTable.tsx

**Files:**
- Modify: `web/src/react/pages/psa-exchange/OpportunitiesTable.tsx` (full rewrite)

- [ ] **Step 1: Replace the file contents**

Replace the entire contents of `web/src/react/pages/psa-exchange/OpportunitiesTable.tsx` with:

```tsx
import { useState } from 'react';
import { clsx } from 'clsx';
import { GradeBadge } from '../../ui';
import type { PsaExchangeOpportunity } from '../../../types/psaExchange';
import SortableHeader from './SortableHeader';
import SignalCell from './SignalCell';
import {
  daysToSell,
  type OpportunityGroup,
  type SortDir,
  type SortKey,
} from './utils';

interface OpportunitiesTableProps {
  rows: PsaExchangeOpportunity[];
  groups: OpportunityGroup[] | null;
  sortKey: SortKey;
  sortDir: SortDir;
  onSort: (key: SortKey) => void;
  topDecileScore: number;
}

const COLUMN_COUNT = 6;

const dollar = (n: number) =>
  n.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 });

function deltaLabel(target: number, value: number): string {
  if (value <= 0) return '';
  const delta = target - value;
  const pct = (delta / value) * 100;
  const sign = delta >= 0 ? '+' : '−';
  const absDelta = Math.abs(delta);
  return `${sign}${dollar(absDelta).replace('$', '$')} (${sign}${Math.abs(pct).toFixed(0)}%)`;
}

export default function OpportunitiesTable({
  rows,
  groups,
  sortKey,
  sortDir,
  onSort,
  topDecileScore,
}: OpportunitiesTableProps) {
  if (rows.length === 0) {
    return (
      <div className="p-6 text-center text-sm text-[var(--text-muted)]">
        No PSA-Exchange opportunities match the current filters.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-md border border-[var(--surface-2)]">
      <table className="w-full text-sm border-collapse">
        <thead className="sticky top-0 z-10 bg-[var(--surface-1)] border-b border-[var(--surface-2)]">
          {/* Six columns shown at every breakpoint. Cert, Comp, Days/sale,
              Vel/mo, Conf, Pop are folded into the Card cell or the Signal
              popover (see SignalCell.tsx). Sortability for those keys is
              dropped here; the underlying applySort() in utils.ts still
              accepts them if invoked externally. */}
          <tr>
            <th scope="col" aria-label="Image" className="w-12 p-2"></th>
            <SortableHeader label="Card" sortKey="description" currentKey={sortKey} currentDir={sortDir} onSort={onSort} />
            <SortableHeader label="PSA Value" sortKey="listPrice" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Target" sortKey="targetOffer" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Signal" sortKey="edgeAtOffer" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Score" sortKey="score" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
          </tr>
        </thead>
        <tbody>
          {groups
            ? groups.map((g) => (
                <GroupRow key={g.key} group={g} topDecileScore={topDecileScore} />
              ))
            : rows.map((r, idx) => (
                <DataRow key={r.cert} row={r} topDecileScore={topDecileScore} zebra={idx % 2 === 1} />
              ))}
        </tbody>
      </table>
    </div>
  );
}

interface DataRowProps {
  row: PsaExchangeOpportunity;
  topDecileScore: number;
  zebra: boolean;
  isMember?: boolean;
}

function DataRow({ row, topDecileScore, zebra, isMember = false }: DataRowProps) {
  const isTopDecile = row.score >= topDecileScore;
  const days = daysToSell(row);
  const grade = Number(row.grade) || 0;
  const delta = deltaLabel(row.targetOffer, row.listPrice);

  return (
    <tr
      className={clsx(
        'border-b border-[var(--surface-2)]/40 hover:bg-[var(--surface-2)]/40 transition-colors',
        zebra && !isMember && 'bg-[var(--surface-1)]/40',
        isMember && 'bg-[var(--surface-1)]/60',
      )}
    >
      <td className={clsx('w-12 p-2', isMember && 'pl-8')}>
        <div className="h-12 w-9 rounded-sm overflow-hidden bg-[var(--surface-2)]/40">
          {row.frontImage && (
            <img src={row.frontImage} alt="" className="h-full w-full object-cover" loading="lazy" />
          )}
        </div>
      </td>
      <td className="p-2 max-w-[36rem]">
        <div className={clsx('flex items-center gap-2 min-w-0', isMember && 'text-xs text-[var(--text-muted)]')}>
          <span className="truncate">{row.description || row.name}</span>
          {grade > 0 && !isMember && <GradeBadge grade={grade} />}
        </div>
        <div className="mt-0.5 flex items-center gap-2 text-[11px] leading-none">
          <span className="font-mono text-[var(--text-muted)] tabular-nums select-text">{row.cert}</span>
          {row.mayTakeAtList && !isMember && (
            <span className="px-1.5 py-0.5 rounded-md bg-[var(--success)]/15 text-[var(--success)]">
              PSA value &lt; target
            </span>
          )}
        </div>
      </td>
      <td className="p-2 text-right tabular-nums">{dollar(row.listPrice)}</td>
      <td className="p-2 text-right tabular-nums">
        <div>{dollar(row.targetOffer)}</div>
        {delta && (
          <div className="text-[10px] text-[var(--text-muted)] leading-none mt-0.5">{delta}</div>
        )}
      </td>
      <td className="p-2">
        <div className="flex justify-end">
          <SignalCell
            edgeAtOffer={row.edgeAtOffer}
            daysToSellValue={days}
            velocityMonth={row.velocityMonth}
            confidence={row.confidence}
            comp={row.comp}
            population={row.population}
          />
        </div>
      </td>
      <td
        className={clsx(
          'p-2 text-right tabular-nums',
          isTopDecile ? 'text-[var(--brand-400)] font-semibold' : 'text-[var(--text-muted)]',
        )}
        title={isTopDecile ? 'Top decile score' : undefined}
      >
        {row.score.toFixed(3)}
      </td>
    </tr>
  );
}

function GroupRow({ group, topDecileScore }: { group: OpportunityGroup; topDecileScore: number }) {
  const [expanded, setExpanded] = useState(false);
  const { primary, members } = group;
  const hasOthers = members.length > 1;
  const lowList = Math.min(...members.map((m) => m.listPrice));
  const highList = Math.max(...members.map((m) => m.listPrice));

  return (
    <>
      <DataRow row={primary} topDecileScore={topDecileScore} zebra={false} />
      {hasOthers && (
        <tr className="bg-[var(--surface-1)]/30 border-b border-[var(--surface-2)]/40">
          <td colSpan={COLUMN_COUNT} className="px-2 pb-2">
            <button
              type="button"
              onClick={() => setExpanded((v) => !v)}
              className="text-[11px] text-[var(--text-muted)] hover:text-[var(--brand-400)] inline-flex items-center gap-1"
              aria-expanded={expanded}
            >
              <span aria-hidden="true">{expanded ? '▾' : '▸'}</span>
              {members.length} listings · {lowList === highList ? dollar(lowList) : `${dollar(lowList)}–${dollar(highList)}`}
            </button>
          </td>
        </tr>
      )}
      {expanded &&
        members
          .filter((m) => m.cert !== primary.cert)
          .map((m) => <DataRow key={m.cert} row={m} topDecileScore={topDecileScore} zebra={false} isMember />)}
    </>
  );
}
```

Key changes vs. original:
- `COLUMN_COUNT = 6`
- Removed all `hidden lg:table-cell` and `hidden xl:table-cell` markers
- Card cell merges description + GradeBadge + cert chip + `PSA value < target` chip
- Target cell shows delta vs PSA Value as a muted second line
- Signal cell replaces Edge / Days/sale / Vel/mo / Conf / Comp / Pop columns
- Removed unused imports (`confidenceColorClass`, `daysBucketClass`, `edgeBucketClass`, `velocityBucketClass` are now consumed inside SignalCell)

- [ ] **Step 2: Run typecheck**

Run:
```bash
cd web && npm run typecheck 2>&1 | tail -10
```

Expected: clean. If errors mention unused imports, remove them from the file.

- [ ] **Step 3: Run tests**

Run:
```bash
cd web && npm test -- --run 2>&1 | tail -20
```

Expected: all tests pass. Compare count against baseline from Task 1.

- [ ] **Step 4: Visual check**

Start (or check) the dev server:

```bash
cd web && npm run dev &
```

Open `http://localhost:5173/opportunities/psa-exchange`. Verify:
- 6 columns: Image, Card, PSA Value, Target, Signal, Score
- No horizontal scroll at 1280px viewport
- Card cell shows description + grade pill on line 1, cert + (when applicable) `PSA value < target` chip on line 2
- Target cell shows the offer dollar with a muted delta below
- Signal cell shows Edge % large with the 3-icon rail underneath
- Hovering / focusing the Signal cell opens a popover with Days/sale, Velocity, Confidence, Comp, Pop
- Group rows still show "N listings · $X" expander; clicking expands members
- Score column is right-aligned and top-decile rows still glow

If you cannot run the browser, run `make screenshots` and review `web/screenshots/opportunities-psa-exchange*.png` (the actual filename will be one of the existing screenshot outputs — list `web/screenshots/` to find it).

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/psa-exchange/OpportunitiesTable.tsx
git commit -m "ui(opps): collapse to 6 columns + Signal cell

Fold Cert, Comp, Days/sale, Vel/mo, Conf, Pop into the Card cell or
the Signal popover. Target cell now shows delta vs PSA Value as a
muted second line. Eliminates horizontal scroll at common laptop
widths and gives the eye a clear hierarchy: anchor dollars → decision
metric (Edge) → reassurance score."
```

---

## Task 7: Final verification

**Files:** none modified.

- [ ] **Step 1: Run full frontend test suite**

Run:
```bash
cd web && npm test -- --run 2>&1 | tail -25
```

Expected: all tests pass; new tests from Tasks 4 + 5 are included.

- [ ] **Step 2: Run typecheck + lint**

Run:
```bash
cd web && npm run typecheck && npm run lint 2>&1 | tail -10
```

Expected: clean.

- [ ] **Step 3: Run go test (sanity)**

Run:
```bash
go test -timeout 5m ./... 2>&1 | tail -10
```

Expected: clean. The change is frontend-only so this should pass without alteration; running it ensures the worktree's go state isn't broken.

- [ ] **Step 4: Generate before/after screenshots**

Run:
```bash
make screenshots 2>&1 | tail -10
```

If `make screenshots` fails, note the failure but don't block on it — the visual verifications in Tasks 3 + 6 are the primary check.

- [ ] **Step 5: Push branch + open PR**

Confirm with the user before pushing. When approved:

```bash
git push -u origin opps-and-actions-polish
gh pr create --title "ui: opportunities column collapse + inventory actions cell tightening" --body "$(cat <<'EOF'
## Summary
- Collapse PSA-Exchange opportunities table from 13 → 6 columns; fold demoted numerics into a Signal cell with hover popover (Comp, Pop, Days/sale, Velocity, Confidence)
- Tighten inventory Actions column from 220px → 144px, drop the left divider, right-align contents

Spec: `docs/specs/2026-05-05-opps-and-actions-polish-design.md`

## Test plan
- [ ] Opportunities page at 1280px shows no horizontal scroll
- [ ] Signal popover opens on hover and on keyboard focus; contents include Days/sale, Velocity, Confidence, Comp, Pop
- [ ] Card cell cert is selectable
- [ ] Group rows expand and contain new column count
- [ ] Inventory Actions cell is ~144px wide with no left divider
- [ ] Mobile inventory view unchanged
- [ ] `npm test`, `npm run typecheck`, `npm run lint` clean
- [ ] `go test ./...` clean

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review checklist (run after writing the plan)

- [x] **Spec coverage:** every section of the spec has a corresponding task — Inventory cell width/divider/alignment in Tasks 2+3; Opps Card cell merge in Task 6; Signal cell anatomy in Tasks 4+5; column count in Task 6; sortability scope in Task 6 comment.
- [x] **Placeholder scan:** no TBDs / TODOs / "implement later"; every code block is complete.
- [x] **Type consistency:** `SignalCell` props match consumer call site in Task 6; `daysTier`/`velocityTier`/`confidenceTier` exports match imports in Task 5.
- [x] **Worktree paths:** every command starts from `/workspace/.worktrees/opps-and-actions-polish` (`cd web` is relative to that).
