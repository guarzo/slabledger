# Bulk Sell at % of CL — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a bulk-sell flow to `/inventory` that lets the user record N sales at a uniform percentage of CardLadder value, with optional per-row tweaks and a pre-flight warning when selected cards lack CL data.

**Architecture:** Split `RecordSaleModal` into a single-only modal and a new `BulkRecordSaleModal`. The bulk modal owns a pricing-mode toggle (`% of CL` / `Flat $`), a fill-all input, a live gross total, and a collapsed per-row review. A pure helper `pricingModes.ts` computes per-row prices. `InventoryHeader` gains a pre-flight banner when the selection contains no-CL cards. No backend changes.

**Spec:** [`docs/specs/2026-04-25-bulk-sell-percent-of-cl-design.md`](../specs/2026-04-25-bulk-sell-percent-of-cl-design.md)

**Tech Stack:** React 18, TypeScript, vitest, `@testing-library/react`, `radix-ui` Dialog, `@tanstack/react-query`.

---

## File structure

```
web/src/react/pages/campaign-detail/
  RecordSaleModal.tsx                   # MODIFY: slim to single-only
  BulkRecordSaleModal.tsx               # CREATE
  saleModal/
    pricingModes.ts                     # CREATE
    pricingModes.test.ts                # CREATE
  BulkRecordSaleModal.test.tsx          # CREATE
  SellSheetView.tsx                     # MODIFY: route to bulk or single modal
  inventory/
    InventoryHeader.tsx                 # MODIFY: mount the new warning component
    BulkSelectionMissingCLWarning.tsx   # CREATE: small standalone warning component
    BulkSelectionMissingCLWarning.test.tsx  # CREATE
    inventoryCalcs.ts                   # MODIFY: support pinnedIds short-circuit
    useInventoryState.ts                # MODIFY: add pinnedIds + handlers
  InventoryTab.tsx                      # MODIFY: thread new callbacks/state through
```

No backend files change. No new query keys, API methods, or types on the wire.

---

## Conventions for every task

- **Run tests:** `cd web && npm test -- <pattern>` (vitest run, not watch).
- **TypeScript check:** `cd web && npm run typecheck` after each task before committing.
- **Lint:** `cd web && npm run lint` after each task.
- **Commit message style:** match recent project style (`feat(ui):`, `refactor(ui):`, `test(ui):` prefixes; subject under 70 chars; trailing newline; `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` line).
- **Worktree:** all work happens on branch `feature/bulk-sell` in `.worktrees/bulk-sell/`.

---

### Task 1: `pricingModes.ts` helper

**Files:**
- Create: `web/src/react/pages/campaign-detail/saleModal/pricingModes.ts`
- Create: `web/src/react/pages/campaign-detail/saleModal/pricingModes.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `web/src/react/pages/campaign-detail/saleModal/pricingModes.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { computeSalePrice } from './pricingModes';
import type { AgingItem } from '../../../../types/campaigns';

function itemWithCL(clValueCents: number): AgingItem {
  return {
    purchase: {
      id: 'p1',
      campaignId: 'c1',
      cardName: 'Test',
      setName: 'Set',
      certNumber: '1',
      grader: 'PSA',
      gradeValue: 10,
      cardNumber: '1',
      buyCostCents: 0,
      psaSourcingFeeCents: 0,
      clValueCents,
      frontImageUrl: '',
      purchaseDate: '2026-01-01',
      receivedAt: undefined,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    },
    daysHeld: 0,
    campaignName: 'C',
    currentMarket: undefined,
    signal: undefined,
    priceAnomaly: false,
  } as AgingItem;
}

describe('computeSalePrice', () => {
  describe('% of CL mode', () => {
    it('multiplies CL by percent and rounds to nearest cent', () => {
      // 4700 * 70 / 100 = 3290
      expect(computeSalePrice(itemWithCL(4700), 'pctOfCL', 70)).toBe(3290);
    });

    it('rounds half-cents away from zero', () => {
      // 4701 * 70 / 100 = 3290.7 -> 3291
      expect(computeSalePrice(itemWithCL(4701), 'pctOfCL', 70)).toBe(3291);
    });

    it('returns 0 when CL is missing', () => {
      expect(computeSalePrice(itemWithCL(0), 'pctOfCL', 70)).toBe(0);
    });

    it('returns 0 for zero percent', () => {
      expect(computeSalePrice(itemWithCL(4700), 'pctOfCL', 0)).toBe(0);
    });

    it('returns CL value for 100 percent', () => {
      expect(computeSalePrice(itemWithCL(4700), 'pctOfCL', 100)).toBe(4700);
    });

    it('clamps negative percent to 0', () => {
      expect(computeSalePrice(itemWithCL(4700), 'pctOfCL', -10)).toBe(0);
    });
  });

  describe('Flat $ mode', () => {
    it('returns the flat cents value regardless of CL', () => {
      expect(computeSalePrice(itemWithCL(4700), 'flat', 500)).toBe(500);
    });

    it('returns 0 when value is 0', () => {
      expect(computeSalePrice(itemWithCL(4700), 'flat', 0)).toBe(0);
    });

    it('clamps negative flat value to 0', () => {
      expect(computeSalePrice(itemWithCL(4700), 'flat', -100)).toBe(0);
    });
  });
});
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd web && npm test -- pricingModes`
Expected: FAIL — `Cannot find module './pricingModes'` or similar.

- [ ] **Step 3: Implement the helper**

Create `web/src/react/pages/campaign-detail/saleModal/pricingModes.ts`:

```ts
import type { AgingItem } from '../../../../types/campaigns';

export type PricingMode = 'pctOfCL' | 'flat';

/**
 * Compute the sale price (in cents) for a single item given a uniform pricing mode.
 *
 * - 'pctOfCL': item.purchase.clValueCents × value / 100, rounded.
 * - 'flat':    value (already in cents), unchanged.
 *
 * Negative inputs are clamped to 0.
 */
export function computeSalePrice(
  item: AgingItem,
  mode: PricingMode,
  value: number,
): number {
  if (value <= 0) return 0;

  if (mode === 'flat') {
    return Math.round(value);
  }

  // pctOfCL
  const cl = item.purchase.clValueCents ?? 0;
  if (cl <= 0) return 0;
  return Math.round((cl * value) / 100);
}
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `cd web && npm test -- pricingModes`
Expected: PASS — all 9 tests green.

- [ ] **Step 5: Run typecheck and lint**

Run: `cd web && npm run typecheck && npm run lint -- src/react/pages/campaign-detail/saleModal/`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/saleModal/
git commit -m "$(cat <<'EOF'
feat(ui): add pricingModes helper for bulk sell

Pure helper that computes per-row sale price from a uniform pricing
mode (% of CL or flat $). Used by the upcoming BulkRecordSaleModal.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Failing tests for `BulkRecordSaleModal` (baseline behavior)

**Files:**
- Create: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx`

These tests describe the bulk path's existing behavior (per-row prices, channel/date, group-by-campaign submit, partial-failure aggregation). Writing them BEFORE the file exists ensures the structural extraction in Task 3 lands under green tests.

- [ ] **Step 1: Write the failing tests**

Create `web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import BulkRecordSaleModal from './BulkRecordSaleModal';
import { ToastProvider } from '../../contexts/ToastContext';
import type { AgingItem } from '../../../types/campaigns';

vi.mock('../../../js/api', () => ({
  api: {
    createBulkSales: vi.fn(),
  },
}));

import { api } from '../../../js/api';

function makeItem(id: string, campaignId: string, clValueCents: number): AgingItem {
  return {
    purchase: {
      id,
      campaignId,
      cardName: `Card ${id}`,
      setName: 'Set',
      certNumber: id,
      grader: 'PSA',
      gradeValue: 10,
      cardNumber: id,
      buyCostCents: 1000,
      psaSourcingFeeCents: 0,
      clValueCents,
      frontImageUrl: '',
      purchaseDate: '2026-01-01',
      receivedAt: '2026-01-02',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    },
    daysHeld: 10,
    campaignName: `Campaign ${campaignId}`,
    currentMarket: undefined,
    signal: undefined,
    priceAnomaly: false,
  } as AgingItem;
}

function renderModal(items: AgingItem[], onSuccess = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <BulkRecordSaleModal open={true} onClose={vi.fn()} onSuccess={onSuccess} items={items} />
      </ToastProvider>
    </QueryClientProvider>
  );
}

describe('BulkRecordSaleModal', () => {
  beforeEach(() => {
    vi.mocked(api.createBulkSales).mockReset();
  });

  it('renders the count of selected cards in the title', () => {
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
    renderModal(items);
    expect(screen.getByText(/Record Sale \(2 cards\)/i)).toBeInTheDocument();
  });

  it('groups items by campaignId on submit and calls api.createBulkSales once per campaign', async () => {
    vi.mocked(api.createBulkSales).mockResolvedValue({ created: 1, failed: 0 });
    const items = [
      makeItem('1', 'c1', 5000),
      makeItem('2', 'c1', 6000),
      makeItem('3', 'c2', 7000),
    ];
    renderModal(items);

    // Default pricing mode is % of CL — type 70 in the fill-all input.
    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    fireEvent.click(screen.getByRole('button', { name: /Record 3 Sales/i }));

    await waitFor(() => {
      expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledTimes(2);
    });
    // c1 group: items 1 and 2 with 70% of their CL
    expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
      'c1',
      expect.any(String),
      expect.any(String),
      expect.arrayContaining([
        { purchaseId: '1', salePriceCents: 3500 },
        { purchaseId: '2', salePriceCents: 4200 },
      ]),
    );
    // c2 group: item 3
    expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
      'c2',
      expect.any(String),
      expect.any(String),
      [{ purchaseId: '3', salePriceCents: 4900 }],
    );
  });

  it('blocks submit when any row resolves to $0', async () => {
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 0)];
    renderModal(items);

    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    fireEvent.click(screen.getByRole('button', { name: /Record 2 Sales/i }));

    await waitFor(() => {
      expect(screen.getByText(/no sale price set/i)).toBeInTheDocument();
    });
    expect(vi.mocked(api.createBulkSales)).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd web && npm test -- BulkRecordSaleModal`
Expected: FAIL — `Cannot find module './BulkRecordSaleModal'`.

- [ ] **Step 3: Commit the failing tests**

Per project convention, fail-first commits aren't separated. Skip this commit; the next task lands the implementation under the same change set if you prefer. (Or commit the tests as a `test(ui):` if you want explicit two-step history. Either is fine.)

---

### Task 3: Create `BulkRecordSaleModal` (extract bulk path)

**Files:**
- Create: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`

Strategy: copy the bulk path of the current `RecordSaleModal` into this new file. Use `% of CL` as the default pricing mode, with a fill-all input. The collapsed-review and per-row override behaviors land in later tasks; for now, the modal renders the per-row list visibly (just like today's bulk modal) so structure is verifiable.

- [ ] **Step 1: Read the current bulk path**

Read `web/src/react/pages/campaign-detail/RecordSaleModal.tsx` lines 98–115 (group-by-campaign submit) and 293–328 (per-row list rendering). These move into the new file.

- [ ] **Step 2: Implement `BulkRecordSaleModal.tsx`**

Create `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`:

```tsx
import { useMemo, useState } from 'react';
import { Dialog } from 'radix-ui';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem, SaleChannel } from '../../../types/campaigns';
import { api } from '../../../js/api';
import { formatCents, localToday, getErrorMessage } from '../../utils/formatters';
import { saleChannelLabels, DEFAULT_SALE_CHANNEL, activeSaleChannels } from '../../utils/campaignConstants';
import { useToast } from '../../contexts/ToastContext';
import { Button, Input, Select } from '../../ui';
import { queryKeys } from '../../queries/queryKeys';
import { costBasis } from './inventory/utils';
import { computeSalePrice, type PricingMode } from './saleModal/pricingModes';

interface Props {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  items: AgingItem[];
}

export default function BulkRecordSaleModal({ open, onClose, onSuccess, items }: Props) {
  const toast = useToast();
  const queryClient = useQueryClient();

  const [channel, setChannel] = useState<SaleChannel>(DEFAULT_SALE_CHANNEL);
  const [saleDate, setSaleDate] = useState(localToday());
  const [pricingMode, setPricingMode] = useState<PricingMode>('pctOfCL');
  const [fillValue, setFillValue] = useState<number>(0); // % when pctOfCL, cents when flat
  const [submitting, setSubmitting] = useState(false);

  const computedPrices = useMemo(() => {
    const m: Record<string, number> = {};
    for (const item of items) {
      m[item.purchase.id] = computeSalePrice(item, pricingMode, fillValue);
    }
    return m;
  }, [items, pricingMode, fillValue]);

  function reset() {
    setChannel(DEFAULT_SALE_CHANNEL);
    setSaleDate(localToday());
    setPricingMode('pctOfCL');
    setFillValue(0);
  }

  function handleClose() {
    if (submitting) return;
    reset();
    onClose();
  }

  async function handleSubmit() {
    if (!saleDate || isNaN(new Date(saleDate).getTime())) {
      toast.error('Please select a sale date');
      return;
    }

    const invalid = items.filter(i => (computedPrices[i.purchase.id] || 0) <= 0);
    if (invalid.length > 0) {
      toast.error(`${invalid.length} card(s) have no sale price set`);
      return;
    }

    setSubmitting(true);
    try {
      const groups = new Map<string, { purchaseId: string; salePriceCents: number }[]>();
      for (const item of items) {
        const cid = item.purchase.campaignId;
        if (!groups.has(cid)) groups.set(cid, []);
        groups.get(cid)!.push({
          purchaseId: item.purchase.id,
          salePriceCents: computedPrices[item.purchase.id] ?? 0,
        });
      }

      const groupEntries = Array.from(groups.entries());
      const results = await Promise.allSettled(
        groupEntries.map(([cid, groupItems]) =>
          api.createBulkSales(cid, channel, saleDate, groupItems)
        )
      );

      let totalCreated = 0;
      let totalFailed = 0;
      for (let i = 0; i < results.length; i++) {
        const r = results[i];
        if (r.status === 'fulfilled') {
          totalCreated += r.value.created;
          totalFailed += r.value.failed;
          if (r.value.errors) {
            for (const err of r.value.errors.slice(0, 3)) {
              toast.error(`Failed: ${err.error}`);
            }
          }
        } else {
          totalFailed += groupEntries[i][1].length;
          toast.error(getErrorMessage(r.reason, 'Bulk sale failed'));
        }
      }
      if (totalCreated > 0) {
        toast.success(`${totalCreated} sale(s) recorded${totalFailed > 0 ? `, ${totalFailed} failed` : ''}`);
      } else {
        toast.error(`All ${totalFailed} sale(s) failed`);
        return; // keep modal open
      }

      const affected = new Set(items.map(i => i.purchase.campaignId));
      for (const cid of affected) {
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.sales(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.purchases(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.pnl(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.channelPnl(cid) });
        queryClient.invalidateQueries({ queryKey: ['campaigns', cid, 'fillRate'] });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.daysToSell(cid) });
      }
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.channelVelocity });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.suggestions });

      onSuccess?.();
      reset();
      onClose();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to record sales'));
    } finally {
      setSubmitting(false);
    }
  }

  const fillInputLabel = pricingMode === 'pctOfCL' ? '% of CL' : 'Flat $';

  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/50 data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-6 max-w-lg w-[calc(100%-2rem)] shadow-xl data-[state=open]:animate-[scaleIn_150ms_ease-out] max-h-[85vh] overflow-y-auto"
        >
          <Dialog.Title className="text-lg font-semibold text-[var(--text)] mb-4">
            Record Sale ({items.length} cards)
          </Dialog.Title>
          <Dialog.Description className="sr-only">
            Enter sale details for multiple cards
          </Dialog.Description>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3 mb-4">
            <Select
              label="Channel"
              required
              selectSize="sm"
              value={channel}
              onChange={e => setChannel(e.target.value as SaleChannel)}
              options={activeSaleChannels.map(ch => ({ value: ch, label: saleChannelLabels[ch] }))}
            />
            <Input
              label="Sale Date"
              required
              type="date"
              inputSize="sm"
              value={saleDate}
              onChange={e => setSaleDate(e.target.value)}
            />
          </div>

          <div className="mb-4">
            <div className="flex items-center gap-4 mb-2">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="radio"
                  name="pricing-mode"
                  checked={pricingMode === 'pctOfCL'}
                  onChange={() => setPricingMode('pctOfCL')}
                />
                % of CL
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="radio"
                  name="pricing-mode"
                  checked={pricingMode === 'flat'}
                  onChange={() => setPricingMode('flat')}
                />
                Flat $
              </label>
            </div>
            <Input
              label={fillInputLabel}
              type="number"
              inputSize="sm"
              min="0"
              step={pricingMode === 'pctOfCL' ? '1' : '0.01'}
              value={pricingMode === 'pctOfCL' ? (fillValue || '') : (fillValue ? fillValue / 100 : '')}
              onChange={e => {
                const raw = e.target.value;
                if (raw === '') { setFillValue(0); return; }
                const n = parseFloat(raw);
                if (Number.isNaN(n)) { setFillValue(0); return; }
                setFillValue(pricingMode === 'pctOfCL' ? n : Math.round(n * 100));
              }}
            />
          </div>

          {/* Per-row review — visible in this task; collapsed disclosure lands in Task 7 */}
          <div className="space-y-2 max-h-60 overflow-y-auto mb-4">
            {items.map(item => (
              <div key={item.purchase.id} className="flex items-center gap-3 p-2 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-2)]/20">
                <div className="flex-1 min-w-0">
                  <div className="text-sm text-[var(--text)] truncate">{item.purchase.cardName}</div>
                  <div className="text-xs text-[var(--text-muted)]">
                    {item.purchase.grader ?? 'PSA'} {item.purchase.gradeValue} | Cost: <span className="tabular-nums">{formatCents(costBasis(item.purchase))}</span>
                    {item.purchase.clValueCents ? <> | CL: <span className="tabular-nums">{formatCents(item.purchase.clValueCents)}</span></> : ''}
                    {item.campaignName ? ` | ${item.campaignName}` : ''}
                  </div>
                </div>
                <div className="flex-shrink-0 w-28 text-right text-sm tabular-nums">
                  {formatCents(computedPrices[item.purchase.id] ?? 0)}
                </div>
              </div>
            ))}
          </div>

          <div className="flex justify-end gap-3 mt-6">
            <Dialog.Close asChild>
              <Button variant="ghost" size="sm" disabled={submitting}>Cancel</Button>
            </Dialog.Close>
            <Button size="sm" onClick={handleSubmit} loading={submitting}>
              {submitting ? 'Recording...' : `Record ${items.length} Sales`}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
```

- [ ] **Step 3: Run the Task 2 tests and verify they pass**

Run: `cd web && npm test -- BulkRecordSaleModal`
Expected: PASS — all 3 tests green.

- [ ] **Step 4: Run the helper tests still pass**

Run: `cd web && npm test -- pricingModes`
Expected: PASS.

- [ ] **Step 5: Run typecheck and lint**

Run: `cd web && npm run typecheck && npm run lint -- src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx
git commit -m "$(cat <<'EOF'
feat(ui): add BulkRecordSaleModal with % of CL pricing

Extracts the bulk-sale path out of RecordSaleModal into a dedicated
component. Adds a pricing-mode toggle (% of CL / flat $) with a
fill-all input that computes per-row sale prices via the pricingModes
helper. Per-row review remains visible — collapsing under a
disclosure lands in a follow-up task.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Wire `SellSheetView` to choose modal based on selection size

**Files:**
- Modify: `web/src/react/pages/campaign-detail/SellSheetView.tsx`

- [ ] **Step 1: Update imports and rendering in `SellSheetView.tsx`**

Open `web/src/react/pages/campaign-detail/SellSheetView.tsx`. At line 5, change the single import to import both modals:

```tsx
import RecordSaleModal from './RecordSaleModal';
import BulkRecordSaleModal from './BulkRecordSaleModal';
```

Replace the `<RecordSaleModal ... />` block (around lines 174–179) with:

```tsx
{saleModalItems.length === 1 ? (
  <RecordSaleModal
    open={saleModalOpen}
    onClose={onCloseSaleModal}
    onSuccess={() => onSaleSuccess(saleModalItems.map(i => i.purchase.id))}
    items={saleModalItems}
  />
) : (
  <BulkRecordSaleModal
    open={saleModalOpen}
    onClose={onCloseSaleModal}
    onSuccess={() => onSaleSuccess(saleModalItems.map(i => i.purchase.id))}
    items={saleModalItems}
  />
)}
```

(Inspect the actual existing handler prop names in the file — `onCloseSaleModal` is illustrative; match what's wired today.)

- [ ] **Step 2: Run the existing test suite**

Run: `cd web && npm test`
Expected: all green. If `useInventoryState.test.ts` or any sibling tests break, they're hitting a real wiring bug — fix before continuing.

- [ ] **Step 3: Run typecheck**

Run: `cd web && npm run typecheck`
Expected: no errors.

- [ ] **Step 4: Manually exercise both paths in dev**

Run: `cd web && npm run dev` in one terminal; `./slabledger` (or skip backend if mocked) in another.
- Open `/inventory`, click the kebab "Sell" on a single row → verify `RecordSaleModal` opens with single-mode UI (price input + listing-detail collapsible).
- Select 2+ checkboxes, click bulk "Record Sale" → verify `BulkRecordSaleModal` opens with the new pricing-mode UI.

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/campaign-detail/SellSheetView.tsx
git commit -m "$(cat <<'EOF'
refactor(ui): route bulk vs. single sale to dedicated modals

SellSheetView now mounts BulkRecordSaleModal when the selection has
more than one card and RecordSaleModal otherwise.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Slim `RecordSaleModal` to single-only

**Files:**
- Modify: `web/src/react/pages/campaign-detail/RecordSaleModal.tsx`

After Task 4, no caller mounts `RecordSaleModal` with `items.length > 1`. Drop the bulk branch and tighten types.

- [ ] **Step 1: Tighten the prop type and remove bulk branches**

Open `web/src/react/pages/campaign-detail/RecordSaleModal.tsx`. Make these changes:

1. Change the `items` prop type from `AgingItem[]` to `[AgingItem]` (a tuple with exactly one element). Update the interface:

```tsx
interface RecordSaleModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  items: [AgingItem]; // single-card flow only
}
```

2. Remove the `isSingle` boolean and use `items[0]` directly throughout.

3. Delete the bulk submit branch (the `else` block currently handling `groups.set / Promise.allSettled`) — only the single-sale `api.createSale` call remains.

4. Delete the bulk price-list rendering (the `: ( <div className="space-y-2 max-h-60 ..."> ... </div> )` block at the end of the price-input ternary). Only the single-sale `<Input label="Sale Price ($)" ... />` and the optional listing-detail collapsible remain.

5. Title text: change `Record Sale{items.length > 1 ? \` (\${items.length} cards)\` : ''}` to just `Record Sale`.

- [ ] **Step 2: Run all tests**

Run: `cd web && npm test`
Expected: all green. The new `BulkRecordSaleModal.test.tsx` and `pricingModes.test.ts` continue passing; nothing else regresses.

- [ ] **Step 3: Run typecheck and verify file size**

Run: `cd web && npm run typecheck && wc -l src/react/pages/campaign-detail/RecordSaleModal.tsx`
Expected: typecheck passes; file is well under 250 lines (was 344).

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/campaign-detail/RecordSaleModal.tsx
git commit -m "$(cat <<'EOF'
refactor(ui): slim RecordSaleModal to single-sale only

Drops the bulk-sale branch (now owned by BulkRecordSaleModal) and
tightens the items prop to a single-element tuple. The optional
listing-detail fields remain — they're single-sale concepts.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Live gross total on the bulk modal

**Files:**
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx`

- [ ] **Step 1: Add a failing test for the live total**

Add this test to `BulkRecordSaleModal.test.tsx` inside the existing `describe` block:

```tsx
it('shows a live gross total that updates as the fill-all input changes', () => {
  const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
  renderModal(items);

  // Empty / 0 percent: total is $0.00
  expect(screen.getByTestId('bulk-sale-total').textContent).toMatch(/\$0\.00/);

  const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
  fireEvent.change(pctInput, { target: { value: '70' } });

  // 5000*0.7 + 6000*0.7 = 3500 + 4200 = 7700 cents = $77.00
  expect(screen.getByTestId('bulk-sale-total').textContent).toMatch(/\$77\.00/);
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `cd web && npm test -- BulkRecordSaleModal`
Expected: FAIL — `Unable to find element with data-testid: bulk-sale-total`.

- [ ] **Step 3: Add the live total to the modal**

In `BulkRecordSaleModal.tsx`, add this just below the fill-all `<Input ... />` block (still inside the same `<div className="mb-4">`):

```tsx
<div className="mt-2 text-sm text-[var(--text-muted)]">
  →{' '}
  <span data-testid="bulk-sale-total" className="tabular-nums text-[var(--text)]">
    {formatCents(
      Object.values(computedPrices).reduce((sum, c) => sum + c, 0)
    )}
  </span>{' '}
  total at this {pricingMode === 'pctOfCL' ? '%' : 'price'}
</div>
```

- [ ] **Step 4: Run the test and verify it passes**

Run: `cd web && npm test -- BulkRecordSaleModal`
Expected: PASS.

- [ ] **Step 5: Run typecheck and lint**

Run: `cd web && npm run typecheck && npm run lint -- src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx
git commit -m "$(cat <<'EOF'
feat(ui): live gross total on bulk sale modal

Sum of computed per-row prices, updates as the fill-all input changes.
Gross only — fees vary by channel and would add noise.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Collapse per-row review behind a disclosure

**Files:**
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx`

- [ ] **Step 1: Add failing tests for the disclosure**

Add to `BulkRecordSaleModal.test.tsx`:

```tsx
it('hides the per-row review by default and reveals it on click', () => {
  const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
  renderModal(items);

  // Per-row review hidden by default
  expect(screen.queryByText('Card 1')).not.toBeInTheDocument();
  expect(screen.queryByText('Card 2')).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: /Review prices \(2\)/i }));

  expect(screen.getByText('Card 1')).toBeInTheDocument();
  expect(screen.getByText('Card 2')).toBeInTheDocument();
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `cd web && npm test -- BulkRecordSaleModal`
Expected: FAIL — Card 1 / Card 2 are visible by default.

- [ ] **Step 3: Wrap the per-row list in a disclosure**

In `BulkRecordSaleModal.tsx`, add a state hook near the others:

```tsx
const [reviewOpen, setReviewOpen] = useState(false);
```

Replace the current per-row list (`<div className="space-y-2 max-h-60 overflow-y-auto mb-4">...</div>`) with:

```tsx
<div className="mb-4">
  <button
    type="button"
    onClick={() => setReviewOpen(o => !o)}
    className="text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
  >
    {reviewOpen ? '▴' : '▾'} Review prices ({items.length})
  </button>
  {reviewOpen && (
    <div className="mt-2 space-y-2 max-h-60 overflow-y-auto">
      {items.map(item => (
        <div key={item.purchase.id} className="flex items-center gap-3 p-2 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-2)]/20">
          <div className="flex-1 min-w-0">
            <div className="text-sm text-[var(--text)] truncate">{item.purchase.cardName}</div>
            <div className="text-xs text-[var(--text-muted)]">
              {item.purchase.grader ?? 'PSA'} {item.purchase.gradeValue} | Cost: <span className="tabular-nums">{formatCents(costBasis(item.purchase))}</span>
              {item.purchase.clValueCents ? <> | CL: <span className="tabular-nums">{formatCents(item.purchase.clValueCents)}</span></> : ''}
              {item.campaignName ? ` | ${item.campaignName}` : ''}
            </div>
          </div>
          <div className="flex-shrink-0 w-28 text-right text-sm tabular-nums">
            {formatCents(computedPrices[item.purchase.id] ?? 0)}
          </div>
        </div>
      ))}
    </div>
  )}
</div>
```

- [ ] **Step 4: Run the test and verify it passes**

Run: `cd web && npm test -- BulkRecordSaleModal`
Expected: PASS.

- [ ] **Step 5: Run typecheck and lint**

Run: `cd web && npm run typecheck && npm run lint -- src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx
git commit -m "$(cat <<'EOF'
feat(ui): collapse bulk sale per-row review behind a disclosure

Per-row breakdown is now hidden by default behind a "Review prices (N)"
toggle. The user sees a clean view at a glance and can drill in only
when they need to.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Per-row override + reset link

**Files:**
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx`

The per-row list (in Tasks 3 + 7) currently shows a read-only computed price. This task makes that price editable and adds a "reset to computed" link when the user has overridden it.

- [ ] **Step 1: Add failing tests for override + reset**

Add to `BulkRecordSaleModal.test.tsx`:

```tsx
it('persists per-row overrides when the fill-all percent changes', async () => {
  vi.mocked(api.createBulkSales).mockResolvedValue({ created: 2, failed: 0 });
  const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
  renderModal(items);

  const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
  fireEvent.change(pctInput, { target: { value: '70' } });

  fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));

  // Manually override row 1 to $25.00 (2500 cents)
  const row1Input = screen.getByLabelText(/override price for Card 1/i) as HTMLInputElement;
  fireEvent.change(row1Input, { target: { value: '25' } });

  // Change the fill-all percent — row 1 should remain at 2500, row 2 should recompute
  fireEvent.change(pctInput, { target: { value: '80' } });

  fireEvent.click(screen.getByRole('button', { name: /Record 2 Sales/i }));

  await waitFor(() => {
    expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
      'c1',
      expect.any(String),
      expect.any(String),
      expect.arrayContaining([
        { purchaseId: '1', salePriceCents: 2500 }, // overridden
        { purchaseId: '2', salePriceCents: 4800 }, // 6000 * 0.80
      ]),
    );
  });
});

it('reset link reverts a row override to the computed price', () => {
  const items = [makeItem('1', 'c1', 5000)];
  renderModal(items);

  const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
  fireEvent.change(pctInput, { target: { value: '70' } });

  fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));

  const row1Input = screen.getByLabelText(/override price for Card 1/i) as HTMLInputElement;
  fireEvent.change(row1Input, { target: { value: '25' } });
  expect(row1Input.value).toBe('25');

  fireEvent.click(screen.getByRole('button', { name: /reset to computed/i }));
  // After reset, the input shows the computed value: 5000 * 0.70 = 3500 cents = $35.00
  expect(row1Input.value).toBe('35');
});
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd web && npm test -- BulkRecordSaleModal`
Expected: FAIL — no override input, no reset link.

- [ ] **Step 3: Add override state and resolve effective prices**

In `BulkRecordSaleModal.tsx`, add a state hook for overrides:

```tsx
const [overrides, setOverrides] = useState<Record<string, number | undefined>>({});
```

Resolve effective prices by merging computed + overrides. Replace the existing `computedPrices` usage at submit time with `effectivePrices`:

```tsx
const effectivePrices = useMemo(() => {
  const m: Record<string, number> = {};
  for (const item of items) {
    const o = overrides[item.purchase.id];
    m[item.purchase.id] = o != null ? o : computedPrices[item.purchase.id];
  }
  return m;
}, [items, computedPrices, overrides]);
```

Update the live total and the submit's `groups.set` payload to use `effectivePrices` instead of `computedPrices`. Also update the invalid-row check at the top of `handleSubmit`:

```tsx
const invalid = items.filter(i => (effectivePrices[i.purchase.id] || 0) <= 0);
```

- [ ] **Step 4: Replace the read-only per-row price with an editable input**

Inside the disclosure's per-row map (added in Task 7), replace the read-only `<div className="flex-shrink-0 w-28 text-right text-sm tabular-nums">{formatCents(...)}</div>` with:

```tsx
<div className="flex-shrink-0 w-44 flex items-center gap-2">
  <input
    type="number"
    step="0.01"
    min="0"
    aria-label={`override price for ${item.purchase.cardName}`}
    placeholder="$"
    value={
      overrides[item.purchase.id] != null
        ? (overrides[item.purchase.id] as number) / 100
        : (computedPrices[item.purchase.id] ?? 0) / 100
    }
    onChange={e => {
      const raw = e.target.value;
      setOverrides(prev => ({
        ...prev,
        [item.purchase.id]: raw === '' ? undefined : Math.round(parseFloat(raw) * 100),
      }));
    }}
    className="w-24 px-2 py-1 text-sm rounded bg-[var(--surface-2)] border border-[var(--surface-2)] text-[var(--text)] focus:outline-none focus:border-[var(--brand-500)]"
  />
  {overrides[item.purchase.id] != null && (
    <button
      type="button"
      aria-label={`reset to computed price for ${item.purchase.cardName}`}
      onClick={() => setOverrides(prev => {
        const next = { ...prev };
        delete next[item.purchase.id];
        return next;
      })}
      className="text-xs text-[var(--text-muted)] hover:text-[var(--text)] underline"
    >
      ↩ reset to computed
    </button>
  )}
</div>
```

Also clear `overrides` in `reset()`:

```tsx
function reset() {
  setChannel(DEFAULT_SALE_CHANNEL);
  setSaleDate(localToday());
  setPricingMode('pctOfCL');
  setFillValue(0);
  setOverrides({});
  setReviewOpen(false);
}
```

- [ ] **Step 5: Run all tests**

Run: `cd web && npm test`
Expected: all green. Verify the existing live-total test still passes — the override case isn't covered there but the value of `effectivePrices` for unedited rows equals `computedPrices`.

- [ ] **Step 6: Run typecheck and lint**

Run: `cd web && npm run typecheck && npm run lint -- src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx
git commit -m "$(cat <<'EOF'
feat(ui): per-row override and reset on bulk sale modal

Inside the review disclosure, each row's price is now editable.
Overrides persist across fill-all changes; a reset link reverts a
row to the computed price. Submit uses effective prices (overrides
when present, computed otherwise).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: CL-missing pre-flight banner

The banner appears in the bulk action bar only when the current selection contains at least one card with no CL value. Two actions: `Highlight` (filters list to those rows) and `Deselect` (removes only no-CL cards from the selection).

To keep `InventoryHeader.tsx`'s prop interface (already 26 props) from sprawling further, extract the banner as a small standalone component `BulkSelectionMissingCLWarning`. Test it in isolation. `InventoryHeader` simply mounts it.

**Files:**
- Create: `web/src/react/pages/campaign-detail/inventory/BulkSelectionMissingCLWarning.tsx`
- Create: `web/src/react/pages/campaign-detail/inventory/BulkSelectionMissingCLWarning.test.tsx`
- Modify: `web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx` (add 2 props, render the new component)
- Modify: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts` (add `pinnedIds` short-circuit)
- Modify: `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts` (add `pinnedIds` state + handlers)
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx` (thread new props)

#### Step group A — the standalone warning component (test-first)

- [ ] **Step 1: Write the failing test for `BulkSelectionMissingCLWarning`**

Create `web/src/react/pages/campaign-detail/inventory/BulkSelectionMissingCLWarning.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import BulkSelectionMissingCLWarning from './BulkSelectionMissingCLWarning';

describe('BulkSelectionMissingCLWarning', () => {
  it('renders nothing when no missing-CL ids are present', () => {
    const { container } = render(
      <BulkSelectionMissingCLWarning
        missingCLIds={[]}
        selectedCount={5}
        onDeselect={vi.fn()}
        onHighlight={vi.fn()}
      />
    );
    expect(container.firstChild).toBeNull();
  });

  it('shows "{n} of {total} cards have no CL value" when ids are present', () => {
    render(
      <BulkSelectionMissingCLWarning
        missingCLIds={['a', 'b']}
        selectedCount={3}
        onDeselect={vi.fn()}
        onHighlight={vi.fn()}
      />
    );
    expect(screen.getByText(/2 of 3 cards have no CL value/i)).toBeInTheDocument();
  });

  it('Deselect button passes the missing-CL ids to onDeselect', () => {
    const onDeselect = vi.fn();
    render(
      <BulkSelectionMissingCLWarning
        missingCLIds={['a', 'b']}
        selectedCount={3}
        onDeselect={onDeselect}
        onHighlight={vi.fn()}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: /Deselect/i }));
    expect(onDeselect).toHaveBeenCalledWith(['a', 'b']);
  });

  it('Highlight button passes the missing-CL ids to onHighlight', () => {
    const onHighlight = vi.fn();
    render(
      <BulkSelectionMissingCLWarning
        missingCLIds={['a', 'b']}
        selectedCount={3}
        onDeselect={vi.fn()}
        onHighlight={onHighlight}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: /Highlight/i }));
    expect(onHighlight).toHaveBeenCalledWith(['a', 'b']);
  });
});
```

- [ ] **Step 2: Run test and verify it fails**

Run: `cd web && npm test -- BulkSelectionMissingCLWarning`
Expected: FAIL — `Cannot find module './BulkSelectionMissingCLWarning'`.

- [ ] **Step 3: Implement the component**

Create `web/src/react/pages/campaign-detail/inventory/BulkSelectionMissingCLWarning.tsx`:

```tsx
interface Props {
  missingCLIds: string[];
  selectedCount: number;
  onDeselect: (ids: string[]) => void;
  onHighlight: (ids: string[]) => void;
}

export default function BulkSelectionMissingCLWarning({
  missingCLIds,
  selectedCount,
  onDeselect,
  onHighlight,
}: Props) {
  if (missingCLIds.length === 0) return null;

  return (
    <div className="flex items-center gap-3 text-xs text-[var(--warning)] bg-[var(--warning)]/10 border border-[var(--warning)]/20 rounded-md px-3 py-1.5 mb-2">
      <span>
        ⚠ {missingCLIds.length} of {selectedCount} cards have no CL value
      </span>
      <button
        type="button"
        onClick={() => onHighlight(missingCLIds)}
        className="underline hover:text-[var(--text)]"
      >
        Highlight
      </button>
      <button
        type="button"
        onClick={() => onDeselect(missingCLIds)}
        className="underline hover:text-[var(--text)]"
      >
        Deselect
      </button>
    </div>
  );
}
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `cd web && npm test -- BulkSelectionMissingCLWarning`
Expected: PASS — all 4 tests green.

#### Step group B — `pinnedIds` plumbing for the Highlight action

- [ ] **Step 5: Extend `filterAndSortItems` with `pinnedIds`**

Open `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts` and find `filterAndSortItems` (around line 174). Extend its options type to include `pinnedIds?: Set<string>`, and add a short-circuit at the top of the function:

```ts
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
    pinnedIds?: Set<string>; // NEW
  },
): AgingItem[] {
  // NEW: when pinnedIds is non-empty, ignore filterTab/showAll/search and
  // return only those items (sorted via the existing sort branch below).
  if (opts.pinnedIds && opts.pinnedIds.size > 0) {
    const subset = items.filter(i => opts.pinnedIds!.has(i.purchase.id));
    return sortItems(subset, opts.sortKey, opts.sortDir, opts.evMap);
  }

  // ... existing implementation unchanged
}
```

(If `sortItems` isn't a separate function in the current file, factor the sort logic out, or inline the sort into the new short-circuit. Match the file's existing pattern.)

If `inventoryCalcs.test.ts` covers `filterAndSortItems`, add a test case:

```ts
it('returns only pinned items, ignoring filterTab/search/showAll', () => {
  const items = [makeMock('1'), makeMock('2'), makeMock('3')];
  const result = filterAndSortItems(items, {
    debouncedSearch: 'no-match',
    showAll: false,
    filterTab: 'needs_attention',
    sellSheetHas: () => false,
    sortKey: 'name',
    sortDir: 'asc',
    evMap: new Map(),
    pinnedIds: new Set(['1', '3']),
  });
  expect(result.map(i => i.purchase.id)).toEqual(['1', '3']);
});
```

- [ ] **Step 6: Run inventoryCalcs tests**

Run: `cd web && npm test -- inventoryCalcs`
Expected: PASS — including the new `pinnedIds` case.

#### Step group C — wire state + handlers through `useInventoryState` and `InventoryTab`

- [ ] **Step 7: Add `pinnedIds` state and handlers in `useInventoryState.ts`**

Open `useInventoryState.ts`. Near the other `useState` calls (around line 58), add:

```tsx
const [pinnedIds, setPinnedIds] = useState<Set<string>>(new Set());
```

Add two handlers near the other `useCallback`s:

```tsx
const handleDeselectMissingCL = useCallback((purchaseIds: string[]) => {
  setSelected(prev => {
    const next = new Set(prev);
    for (const id of purchaseIds) next.delete(id);
    return next;
  });
}, []);

const handleHighlightMissingCL = useCallback((purchaseIds: string[]) => {
  setPinnedIds(new Set(purchaseIds));
}, []);

const clearPinnedIds = useCallback(() => {
  setPinnedIds(new Set());
}, []);
```

Auto-clear `pinnedIds` when the user changes filter tab, search, or explicitly toggles "show all" (so `Highlight` is a transient state, not a sticky one). In the existing `useEffect` at line 112 (which already resets scroll on these changes), add `setPinnedIds(new Set())`:

```tsx
useEffect(() => {
  setExpandedId(null);
  setPinnedIds(new Set()); // NEW: clear highlight when user changes filter context
  scrollContainerRef.current?.scrollTo({ top: 0 });
  mobileScrollRef.current?.scrollTo({ top: 0 });
}, [sortKey, sortDir, debouncedSearch, filterTab, showAll]);
```

Pass `pinnedIds` into `filterAndSortItems`:

```tsx
const filteredAndSortedItems = useMemo(
  () => filterAndSortItems(items, {
    debouncedSearch,
    showAll,
    filterTab,
    sellSheetHas,
    sortKey,
    sortDir,
    evMap,
    pinnedIds, // NEW
  }),
  [items, debouncedSearch, sortKey, sortDir, evMap, showAll, filterTab, sellSheetHas, pinnedIds],
);
```

Add `pinnedIds`, `clearPinnedIds`, `handleDeselectMissingCL`, `handleHighlightMissingCL` to the hook's return object.

- [ ] **Step 8: Update `InventoryHeader.tsx` to mount the warning**

Open `InventoryHeader.tsx`. Add to `InventoryHeaderProps`:

```tsx
onDeselectMissingCL: (purchaseIds: string[]) => void;
onHighlightMissingCL: (purchaseIds: string[]) => void;
```

Destructure them in the function signature. Import the new component near the top:

```tsx
import BulkSelectionMissingCLWarning from './BulkSelectionMissingCLWarning';
```

Compute missing-CL ids near the top of the component body:

```tsx
const missingCLIds = useMemo(
  () => items.filter(i => selected.has(i.purchase.id) && !i.purchase.clValueCents).map(i => i.purchase.id),
  [items, selected],
);
```

Mount the warning just above the row that holds the bulk action buttons (the `{N} selected` line):

```tsx
<BulkSelectionMissingCLWarning
  missingCLIds={missingCLIds}
  selectedCount={selected.size}
  onDeselect={onDeselectMissingCL}
  onHighlight={onHighlightMissingCL}
/>
```

- [ ] **Step 9: Thread the callbacks through `InventoryTab.tsx`**

Open `InventoryTab.tsx`. Where the hook is destructured, add `handleDeselectMissingCL` and `handleHighlightMissingCL` to the destructured names. Pass them as props to `<InventoryHeader ... onDeselectMissingCL={handleDeselectMissingCL} onHighlightMissingCL={handleHighlightMissingCL} />`.

- [ ] **Step 10: Run all tests, typecheck, lint**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: all green.

- [ ] **Step 11: Manual smoke**

Start dev server (`cd web && npm run dev` + `./slabledger`). On `/inventory`:
- Select 5 cards including 2 with `clValueCents = 0` — banner appears with "2 of 5 cards have no CL value".
- Click `Deselect` — selection drops to 3, banner disappears.
- Click `Highlight` — list filters to show only the 2 no-CL cards. Change filter tab — `pinnedIds` clears, list returns to normal.

- [ ] **Step 12: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/BulkSelectionMissingCLWarning.tsx \
        web/src/react/pages/campaign-detail/inventory/BulkSelectionMissingCLWarning.test.tsx \
        web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx \
        web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts \
        web/src/react/pages/campaign-detail/inventory/inventoryCalcs.test.ts \
        web/src/react/pages/campaign-detail/inventory/useInventoryState.ts \
        web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "$(cat <<'EOF'
feat(ui): pre-flight CL-missing warning in bulk action bar

When the current selection contains any card with no CL value, the
bulk action bar surfaces an inline warning with one-click Deselect
and Highlight actions. Highlight uses a transient pinnedIds state in
filterAndSortItems to show only the no-CL cards; changing filter tab
or search clears it.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Manual verification + screenshot regression

**Files:** none (verification only).

- [ ] **Step 1: Run the full test suite**

Run: `cd web && npm test`
Expected: all green. Note the totals.

- [ ] **Step 2: Run typecheck and lint over the whole project**

Run: `cd web && npm run typecheck && npm run lint`
Expected: no errors.

- [ ] **Step 3: Generate fresh screenshots and verify no visual regression**

Run: `cd web && npx playwright test tests/screenshot-all-pages.spec.ts --project=chromium`
Inspect `web/screenshots/inventory.png`, `web/screenshots/campaigns-detail-inventory.png`, plus the mobile counterparts. Compare against the previous baseline (use `git diff -- web/screenshots/` if checked-in, otherwise eyeball against the committed images).

- [ ] **Step 4: Manually exercise the flow end-to-end**

In one terminal: `./slabledger` (after `go build -o slabledger ./cmd/slabledger`).
In another: `cd web && npm run dev`.

Walk through:
1. Open `/inventory`. Verify checkboxes work, selecting 1 card opens single modal, selecting 2+ opens bulk modal.
2. With 5+ cards selected (mix across 2 campaigns, mix with/without CL), verify the pre-flight banner shows the correct count.
3. Click `Highlight` — verify only the no-CL cards remain visible in the list. Reset by changing tab.
4. Click `Deselect` — verify only the no-CL cards leave the selection.
5. Open the bulk modal with all-CL selection. In `% of CL` mode, type 70 → verify live total updates. Open review → verify per-row prices match `CL × 0.7`.
6. Override one row to a custom price → change `%` to 80 → verify the overridden row stays put while others recompute. Click `↩ reset to computed` → verify it reverts to `CL × 0.8`.
7. Switch to `Flat $` → verify input label flips, all rows show the same flat amount.
8. Submit with channel `In person` and today's date. Confirm toast `N sale(s) recorded`. Verify the rows disappear from `/inventory` after refresh.
9. Verify the same flow on `/campaigns/:id` Inventory tab — same behavior, since `InventoryTab` is shared.

- [ ] **Step 5: Run `make check` for backend invariants**

Run: `make check` (from repo root)
Expected: pass — domain-import, file-size, and lint checks all clean. Backend should be untouched, so this is a sanity check.

- [ ] **Step 6: Final commit (only if any small fixes landed during verification)**

If verification surfaced no issues, no commit. If it did, fix and commit:

```bash
git add <files>
git commit -m "$(cat <<'EOF'
fix(ui): <specific fix found during verification>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Push and open PR**

When the user is ready (don't push proactively):

```bash
git push -u origin feature/bulk-sell
gh pr create --title "feat(ui): bulk sell at % of CL on /inventory" --body "<see spec for context>"
```

---

## Open follow-ups (out of scope; do not implement)

- Persist last-used `% of CL` to localStorage.
- `% of cost` pricing mode (liquidation runs).
- Server-side `pctOfCL` audit trail.

These are noted in the spec under "Open follow-ups" and should be deferred unless the user explicitly asks.
