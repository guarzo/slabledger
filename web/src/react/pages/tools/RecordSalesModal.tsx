import { useState, useCallback } from 'react';
import type { SaleRowData, SaleSummary } from './sale-types';

interface RecordSalesModalProps {
  rows: SaleRowData[];
  summary: SaleSummary;
  onConfirm: (saleDate: string, channel: string) => void;
  onCancel: () => void;
  loading: boolean;
}

function fmt(cents: number): string {
  return `$${(cents / 100).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

export function RecordSalesModal({ rows, summary, onConfirm, onCancel, loading }: RecordSalesModalProps) {
  const [saleDate, setSaleDate] = useState(() => new Date().toISOString().slice(0, 10));
  const [channel, setChannel] = useState('local');

  const handleConfirm = useCallback(() => {
    onConfirm(saleDate, channel);
  }, [saleDate, channel, onConfirm]);

  const resolvedRows = rows.filter(r => r.status === 'resolved');

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onCancel}>
      <div className="max-h-[80vh] w-full max-w-lg overflow-auto rounded-lg border border-zinc-700 bg-zinc-900 p-6" onClick={e => e.stopPropagation()}>
        <h2 className="mb-4 text-lg font-semibold text-white">Record {summary.cardCount} Sale{summary.cardCount > 1 ? 's' : ''}</h2>

        <div className="mb-4 grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs text-zinc-400">Sale Date</label>
            <input type="date" value={saleDate} onChange={e => setSaleDate(e.target.value)}
              className="w-full rounded border border-zinc-700 bg-zinc-800 px-2 py-1.5 text-sm text-white" />
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">Channel</label>
            <select value={channel} onChange={e => setChannel(e.target.value)}
              className="w-full rounded border border-zinc-700 bg-zinc-800 px-2 py-1.5 text-sm text-white">
              <option value="local">Local</option>
              <option value="ebay">eBay</option>
              <option value="tcgplayer">TCGPlayer</option>
              <option value="other">Other</option>
            </select>
          </div>
        </div>

        <div className="mb-4 rounded border border-zinc-800 text-xs">
          <div className="grid grid-cols-[1fr_80px] gap-1 border-b border-zinc-800 bg-zinc-800/50 px-3 py-1.5 text-zinc-500">
            <span>Card</span>
            <span className="text-right">Sale Price</span>
          </div>
          {resolvedRows.map(row => (
            <div key={row.certNumber} className="grid grid-cols-[1fr_80px] gap-1 border-b border-zinc-800/50 px-3 py-1.5">
              <span className="truncate text-zinc-300">{row.cardName}</span>
              <span className="text-right text-white">{fmt(row.salePriceCents)}</span>
            </div>
          ))}
        </div>

        <div className="mb-4 flex gap-4 text-xs text-zinc-400">
          <span>Total: <span className="font-semibold text-white">{fmt(summary.saleTotalCents)}</span></span>
          <span>Avg: <span className="text-zinc-300">{summary.avgDiscountPct}%</span></span>
        </div>

        <div className="flex justify-end gap-2">
          <button onClick={onCancel} disabled={loading}
            className="rounded-md border border-zinc-700 px-4 py-2 text-sm text-zinc-300 hover:border-zinc-500 transition-colors disabled:opacity-50">
            Cancel
          </button>
          <button onClick={handleConfirm} disabled={loading}
            className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500 transition-colors disabled:opacity-50">
            {loading ? 'Recording...' : `Confirm ${summary.cardCount} Sale${summary.cardCount > 1 ? 's' : ''}`}
          </button>
        </div>
      </div>
    </div>
  );
}
