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
    const t = setTimeout(() => window.print(), 100);
    return () => clearTimeout(t);
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
    <div className="p-6 max-w-3xl mx-auto">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold">Sell Sheet</h1>
        <div className="text-sm text-[var(--text-muted)] mt-1">
          All Inventory · {slices.totalItemCount} cards in hand ·{' '}
          {dollars(slices.totalAskCents)} total ask
        </div>
      </header>

      <ul className="divide-y border rounded">
        {order.map((id) => {
          const s = slices[id];
          return (
            <li key={id} className="flex items-center justify-between p-4">
              <div>
                <div className="font-medium">{s.label}</div>
                <div className="text-sm text-[var(--text-muted)]">
                  {s.itemCount} · {dollars(s.totalAskCents)}
                </div>
                <div className="text-xs text-[var(--text-muted)] mt-0.5">
                  {s.description}
                </div>
              </div>
              <button
                onClick={() => setActiveSliceId(id)}
                className="px-4 py-1.5 rounded bg-[var(--brand-500)] text-white disabled:opacity-50"
                disabled={s.itemCount === 0}
              >
                Print
              </button>
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
