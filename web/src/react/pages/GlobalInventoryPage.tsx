import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { useSellSheet } from '../hooks/useSellSheet';
import { useGlobalInventory } from '../queries/useCampaignQueries';
import { SectionErrorBoundary } from '../ui';
import InventoryTab from './campaign-detail/InventoryTab';

export default function GlobalInventoryPage() {
  const { data: items = [], warnings, isLoading, isError, error } = useGlobalInventory();
  const sellSheet = useSellSheet();
  const pageSellSheetCount = useMemo(() => items.filter(i => sellSheet.has(i.purchase.id)).length, [items, sellSheet]);

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
          {new Date().toLocaleDateString()} &middot; {pageSellSheetCount} items
        </div>
      </div>

      <div className="print:hidden">
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Inventory</h1>
        </div>
      </div>

      {warnings && warnings.length > 0 && (
        <div className="mb-4 p-3 rounded-lg bg-[var(--warning)]/10 border border-[var(--warning)]/20 text-sm text-[var(--warning)]">
          <ul className="list-disc list-inside space-y-1">
            {warnings.map((w, i) => <li key={i}>{w}</li>)}
          </ul>
        </div>
      )}

      <SectionErrorBoundary sectionName="Inventory">
        <InventoryTab items={items} isLoading={isLoading} showCampaignColumn />
      </SectionErrorBoundary>

      {/* Print footer — visible only when printing */}
      <div className="sell-sheet-print-footer">
        {pageSellSheetCount} items &middot; {new Date().toLocaleDateString()} &middot; card-yeti.com
      </div>

      {/* Liquidation analysis link — full report lives on Insights */}
      <Link
        to="/insights"
        className="mt-6 print:hidden flex items-center justify-between gap-4 p-4 rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)] hover:border-[var(--brand-500)]/40 hover:bg-[var(--surface-2)]/30 transition-colors"
      >
        <div className="flex items-center gap-3">
          <span className="text-xl" aria-hidden="true">&#x2728;</span>
          <div>
            <div className="text-sm font-semibold text-[var(--text)]">Liquidation plan lives on Insights</div>
            <div className="text-xs text-[var(--text-muted)]">Markdowns, auction picks, hold list, and totals — as structured sections.</div>
          </div>
        </div>
        <span className="text-sm text-[var(--brand-400)] font-medium">Open Insights &rarr;</span>
      </Link>
    </div>
  );
}
