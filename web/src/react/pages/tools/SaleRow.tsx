import { useCallback, useState } from 'react';
import type { SaleRowData } from './sale-types';

interface SaleRowProps {
  row: SaleRowData;
  costVisible: boolean;
  onCompValueChange: (certNumber: string, cents: number) => void;
  onSalePriceChange: (certNumber: string, cents: number) => void;
  onDismiss: (certNumber: string) => void;
}

function formatDollars(cents: number | undefined): string {
  if (cents === undefined || cents === 0) return '—';
  return `$${(cents / 100).toFixed(0)}`;
}

function EditableCell({ cents, onChange, highlight }: {
  cents: number;
  onChange: (cents: number) => void;
  highlight: 'default' | 'orange' | 'red';
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');

  const borderColor = highlight === 'orange' ? 'border-amber-500' : highlight === 'red' ? 'border-red-500' : 'border-zinc-700';
  const textColor = highlight === 'orange' ? 'text-amber-500' : highlight === 'red' ? 'text-red-500' : 'text-white';

  const handleStart = useCallback(() => {
    setDraft((cents / 100).toFixed(2));
    setEditing(true);
  }, [cents]);

  const handleCommit = useCallback(() => {
    setEditing(false);
    const val = Math.round(parseFloat(draft) * 100);
    if (!isNaN(val) && val >= 0) onChange(val);
  }, [draft, onChange]);

  if (editing) {
    return (
      <input
        autoFocus
        type="number"
        step="0.01"
        value={draft}
        onChange={e => setDraft(e.target.value)}
        onBlur={handleCommit}
        onKeyDown={e => e.key === 'Enter' && (e.target as HTMLInputElement).blur()}
        className={`w-20 rounded border ${borderColor} bg-zinc-900 px-1.5 py-0.5 text-right text-xs ${textColor}`}
      />
    );
  }

  return (
    <button
      onClick={handleStart}
      className={`rounded border ${borderColor} px-1.5 py-0.5 text-right text-xs ${textColor} hover:border-zinc-500 transition-colors cursor-text w-20`}
    >
      {formatDollars(cents)}
    </button>
  );
}

export function SaleRow({ row, costVisible, onCompValueChange, onSalePriceChange, onDismiss }: SaleRowProps) {
  if (row.status === 'error') {
    return (
      <div className="flex items-center gap-3 border-t border-zinc-800 px-3 py-2">
        <span className="text-xs text-red-500">✗</span>
        <span className="text-xs text-zinc-400">{row.certNumber}</span>
        <span className="text-xs text-red-400">{row.error}</span>
        <button onClick={() => onDismiss(row.certNumber)} className="ml-auto text-xs text-zinc-600 hover:text-zinc-400">✕</button>
      </div>
    );
  }

  if (row.status === 'scanning') {
    return (
      <div className="flex items-center gap-3 border-t border-zinc-800 px-3 py-2">
        <span className="text-xs text-zinc-500 animate-pulse">⏳</span>
        <span className="text-xs text-zinc-400">{row.certNumber}</span>
        <span className="text-xs text-zinc-500">Scanning...</span>
      </div>
    );
  }

  const effectivePct = row.compValueCents > 0 ? Math.round(row.salePriceCents / row.compValueCents * 100) : 0;
  const pctColor = row.salePriceManuallySet ? 'text-red-500' : row.compManuallySet ? 'text-amber-500' : 'text-zinc-500';
  const compHighlight = row.compManuallySet ? 'orange' as const : 'default' as const;
  const saleHighlight = row.salePriceManuallySet ? 'red' as const : row.compManuallySet ? 'orange' as const : 'default' as const;

  return (
    <div className="grid items-center gap-1 border-t border-zinc-800 px-3 py-2"
      style={{ gridTemplateColumns: costVisible
        ? '24px 1fr 64px 64px 64px 64px 80px 80px 36px'
        : '24px 1fr 64px 64px 64px 80px 80px 36px'
      }}
    >
      <span className="text-xs text-emerald-500">✓</span>
      <div className="min-w-0">
        <div className="truncate text-xs text-zinc-200">{row.cardName}</div>
        <div className="truncate text-[10px] text-zinc-600">#{row.certNumber} · {row.setName}</div>
      </div>
      {costVisible && <span className="text-right text-xs text-zinc-600">{formatDollars(row.buyCostCents)}</span>}
      <span className="text-right text-xs text-emerald-500">{formatDollars(row.clValueCents)}</span>
      <span className="text-right text-xs text-zinc-500">{formatDollars(row.dhListingPriceCents)}</span>
      <span className="text-right text-xs text-zinc-500">{formatDollars(row.lastSoldCents)}</span>
      <div className="text-right">
        <EditableCell cents={row.compValueCents} onChange={c => onCompValueChange(row.certNumber, c)} highlight={compHighlight} />
      </div>
      <div className="text-right">
        <EditableCell cents={row.salePriceCents} onChange={c => onSalePriceChange(row.certNumber, c)} highlight={saleHighlight} />
      </div>
      <span className={`text-right text-[11px] ${pctColor}`}>{effectivePct}%</span>
    </div>
  );
}
