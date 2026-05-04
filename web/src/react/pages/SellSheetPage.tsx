import { useEffect, useMemo, useState } from 'react';
import { useGlobalSellSheet } from '../queries/useCampaignQueries';
import {
  computeSlices,
  type SliceID,
  type SliceResult,
} from '../utils/sellSheetSlices';
import SellSheetPrintRow from './campaign-detail/inventory/SellSheetPrintRow';
import type { AgingItem, Purchase, SellSheetItem } from '../../types/campaigns';
import '../../styles/print-sell-sheet.css';

function dollars(cents: number): string {
  return `$${(cents / 100).toLocaleString('en-US', { maximumFractionDigits: 0 })}`;
}

function asAgingItem(item: SellSheetItem): AgingItem {
  const now = new Date().toISOString();
  const purchase: Purchase = {
    id: item.purchaseId ?? '',
    campaignId: '',
    cardName: item.cardName,
    certNumber: item.certNumber,
    setName: item.setName,
    cardNumber: item.cardNumber,
    grader: item.grader,
    gradeValue: item.grade,
    clValueCents: item.clValueCents,
    buyCostCents: item.buyCostCents,
    psaSourcingFeeCents: 0,
    purchaseDate: '',
    createdAt: now,
    updatedAt: now,
  };
  return {
    purchase,
    daysHeld: 0,
    recommendedPriceCents: item.targetSellPrice,
  };
}

interface PrintViewProps {
  slice: SliceResult;
  onBack: () => void;
}

function PrintView({ slice, onBack }: PrintViewProps) {
  useEffect(() => {
    // Wait for two animation frames so the browser has actually painted
    // every SellSheetPrintRow (including its barcode SVG, which renders
    // in the row's own useEffect on the first frame) before opening the
    // print dialog. A fixed timeout races with row rendering.
    let outerRaf = 0;
    let innerRaf = 0;
    let cancelled = false;
    outerRaf = requestAnimationFrame(() => {
      innerRaf = requestAnimationFrame(() => {
        if (!cancelled) window.print();
      });
    });
    return () => {
      cancelled = true;
      cancelAnimationFrame(outerRaf);
      cancelAnimationFrame(innerRaf);
    };
  }, []);

  return (
    <div data-testid="sell-sheet-print-view">
      <div className="no-print mb-3 flex items-center gap-3 p-4">
        <button onClick={onBack} className="px-3 py-1 border rounded">
          Back
        </button>
        <button onClick={() => window.print()} className="px-3 py-1 border rounded">
          Print
        </button>
      </div>
      <div className="sell-sheet-print">
        <div className="sell-sheet-print-header">
          <h1>
            Sell Sheet — {slice.label} · {slice.itemCount} cards ·{' '}
            {dollars(slice.totalAskCents)}
          </h1>
        </div>
        <div className="sell-sheet-print-thead">
          <div className="sell-sheet-print-cell" data-cell="num">#</div>
          <div className="sell-sheet-print-cell" data-cell="card">Card</div>
          <div className="sell-sheet-print-cell" data-cell="grade">Grade</div>
          <div className="sell-sheet-print-cell" data-cell="cert">Cert</div>
          <div className="sell-sheet-print-cell" data-cell="cl">CL</div>
          <div className="sell-sheet-print-cell" data-cell="agreed">Agreed Price</div>
        </div>
        {slice.items.map((it, idx) => (
          <SellSheetPrintRow
            key={it.purchaseId ?? `${it.certNumber}-${idx}`}
            item={asAgingItem(it)}
            rowNumber={idx + 1}
          />
        ))}
      </div>
    </div>
  );
}

export default function SellSheetPage() {
  const { data, isLoading, error } = useGlobalSellSheet();
  const [activeSliceId, setActiveSliceId] = useState<SliceID | null>(null);

  const slices = useMemo(
    () => (data ? computeSlices(data.items) : null),
    [data],
  );

  if (isLoading) return <div className="p-6">Loading inventory…</div>;
  if (error) return <div className="p-6 text-red-600">Failed to load sell sheet.</div>;
  if (!slices) return null;

  if (activeSliceId) {
    const slice = slices[activeSliceId];
    return <PrintView slice={slice} onBack={() => setActiveSliceId(null)} />;
  }

  const order: SliceID[] = [
    'psa10',
    'modern',
    'vintage',
    'highValue',
    'underOneK',
    'byGrade',
    'full',
  ];

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <header className="mb-6">
        <h1 className="page-title">Sell Sheet</h1>
        <div className="text-sm text-[var(--text-muted)] mt-1">
          All Inventory · {slices.totalItemCount} cards in hand ·{' '}
          {dollars(slices.totalAskCents)} total ask
        </div>
        <p className="text-xs text-[var(--text-muted)] mt-2 max-w-2xl leading-relaxed">
          Pick a slice and print. Each preset filters the same inventory by era, grade, or price band — useful for in-person buyers who only care about one segment.
        </p>
      </header>

      <ul className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {order.map((id) => {
          const s = slices[id];
          const empty = s.itemCount === 0;
          return (
            <li
              key={id}
              className={`flex flex-col gap-3 p-4 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] ${empty ? 'opacity-50' : ''}`}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--brand-400)] mb-0.5">
                    {s.label}
                  </div>
                  {/* Total ask as the tile hero — in mono so the seven tiles
                      vertically column-align across the grid. */}
                  <div className="text-2xl font-semibold tabular-nums text-[var(--text)] leading-none">
                    {dollars(s.totalAskCents)}
                  </div>
                  <div className="text-xs text-[var(--text-muted)] tabular-nums mt-1">
                    {s.itemCount} {s.itemCount === 1 ? 'card' : 'cards'} · {s.description}
                  </div>
                </div>
                <button
                  onClick={() => setActiveSliceId(id)}
                  className="px-4 py-1.5 rounded bg-[var(--brand-600)] text-white text-sm hover:bg-[var(--brand-700)] disabled:opacity-50 disabled:cursor-not-allowed flex-shrink-0 transition-colors"
                  disabled={empty}
                >
                  Print
                </button>
              </div>
              {/* Top 3 items by target ask — gives the operator a sniff test
                  for the slice contents without printing. Items are already
                  pre-sorted by the slice-builder (psa10 high-to-low, others
                  by set), so we sort here for consistency across slices. */}
              {!empty && (
                <ul className="border-t border-[var(--surface-2)]/60 pt-2 space-y-1">
                  {[...s.items]
                    .sort((a, b) => b.targetSellPrice - a.targetSellPrice)
                    .slice(0, 3)
                    .map((item, index) => (
                      // Fallback chain because certNumber is typed non-optional
                      // but real-world data could have an empty string or a dup
                      // across slices that happen to share a card; index-tail
                      // guarantees uniqueness within this preview list.
                      <li
                        key={item.certNumber || item.purchaseId || `preview-${index}`}
                        className="flex items-baseline justify-between gap-2 text-xs"
                      >
                        <span className="text-[var(--text)] truncate" title={item.cardName}>
                          <span className="text-[var(--text-muted)] mr-1.5 tabular-nums">
                            {item.grader ?? 'PSA'} {item.grade}
                          </span>
                          {item.cardName}
                        </span>
                        <span className="text-[var(--text-muted)] tabular-nums whitespace-nowrap">
                          {dollars(item.targetSellPrice)}
                        </span>
                      </li>
                    ))}
                  {s.itemCount > 3 && (
                    <li className="text-[10px] text-[var(--text-subtle)] tabular-nums pt-0.5">
                      + {s.itemCount - 3} more
                    </li>
                  )}
                </ul>
              )}
            </li>
          );
        })}
      </ul>

      {slices.unparseableYearCount > 0 && (
        <div className="text-xs text-[var(--text-muted)] mt-4">
          {slices.unparseableYearCount} cards have no parseable year and were
          excluded from the era slices.
        </div>
      )}
    </div>
  );
}
