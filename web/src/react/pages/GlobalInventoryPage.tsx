import { useGlobalInventory } from '../queries/useCampaignQueries';
import { Breadcrumb, SectionErrorBoundary } from '../ui';
import InventoryTab from './campaign-detail/InventoryTab';

export default function GlobalInventoryPage() {
  const { data: items = [], warnings, isLoading, isError, error } = useGlobalInventory();

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
      <div className="print:hidden">
        <Breadcrumb items={[{ label: 'Inventory' }]} />
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
    </div>
  );
}
