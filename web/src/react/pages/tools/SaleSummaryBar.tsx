import { formatCents } from './sale-types';
import type { SaleSummary } from './sale-types';

interface SaleSummaryBarProps {
  summary: SaleSummary;
  costVisible: boolean;
  onClearAll: () => void;
  onRecordSales: () => void;
}

export function SaleSummaryBar({ summary, costVisible, onClearAll, onRecordSales }: SaleSummaryBarProps) {
  if (summary.cardCount === 0) return null;

  return (
    <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-zinc-800 bg-zinc-900/50 px-3 py-2">
      <div className="flex gap-4 text-xs">
        <div><span className="text-zinc-500">Cards:</span> <span className="text-white">{summary.cardCount}</span></div>
        {costVisible && <div><span className="text-zinc-500">Cost:</span> <span className="text-zinc-300">{formatCents(summary.costTotalCents)}</span></div>}
        <div><span className="text-zinc-500">Comp:</span> <span className="text-emerald-500">{formatCents(summary.compTotalCents)}</span></div>
        <div><span className="text-zinc-500">Sale:</span> <span className="font-semibold text-white">{formatCents(summary.saleTotalCents)}</span></div>
        {costVisible && <div><span className="text-zinc-500">Profit:</span> <span className="font-semibold text-emerald-500">{formatCents(summary.profitCents)}</span></div>}
        <div><span className="text-zinc-500">Avg:</span> <span className="text-zinc-300">{summary.avgDiscountPct}%</span></div>
      </div>
      <div className="flex gap-2">
        <button onClick={onClearAll} className="rounded-md border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:border-zinc-500 transition-colors">
          Clear All
        </button>
        <button onClick={onRecordSales} className="rounded-md bg-indigo-600 px-4 py-1.5 text-xs font-semibold text-white hover:bg-indigo-500 transition-colors">
          Record Sales ({summary.cardCount})
        </button>
      </div>
    </div>
  );
}
