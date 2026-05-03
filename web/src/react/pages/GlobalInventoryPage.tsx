import { useGlobalInventory } from '../queries/useCampaignQueries';
import { SectionErrorBoundary } from '../ui';
import InventoryTab from './campaign-detail/InventoryTab';

export default function GlobalInventoryPage() {
  const { data: items = [], warnings, isLoading, isError, error, refetch, isFetching } = useGlobalInventory();

  const inHand = items.filter(i => !!i.purchase.receivedAt).length;

  if (isError) {
    return (
      <div className="max-w-6xl mx-auto px-4 text-center py-16">
        <p className="text-[var(--danger)] mb-4">{error instanceof Error ? error.message : 'Failed to load inventory'}</p>
        <button
          type="button"
          onClick={() => refetch()}
          disabled={isFetching}
          className="px-4 py-2 bg-[var(--brand-500)] text-white rounded-lg text-sm font-medium hover:bg-[var(--brand-600)] transition-colors disabled:opacity-60 disabled:cursor-not-allowed"
        >
          {isFetching ? 'Retrying…' : 'Retry'}
        </button>
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="print:hidden">
        <div className="flex items-baseline justify-between mb-6 gap-3 flex-wrap">
          <div className="flex items-baseline gap-3 flex-wrap">
            <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Inventory</h1>
            {!isLoading && items.length > 0 && (
              <span className="text-sm text-[var(--text-muted)] tabular-nums">
                {items.length} {items.length === 1 ? 'card' : 'cards'}
                {inHand > 0 && <> · {inHand} in hand</>}
              </span>
            )}
          </div>
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
    </div>
  );
}
