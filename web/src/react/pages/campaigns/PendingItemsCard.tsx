import { useState } from 'react';
import { usePSAPendingItems, useAssignPendingItem, useDismissPendingItem, useCampaigns } from '../../queries/useCampaignQueries';
import type { PSAPendingItem } from '../../../types/admin';

function formatCents(cents: number): string {
  return '$' + (cents / 100).toFixed(2);
}

function PendingRow({ item }: { item: PSAPendingItem }) {
  const { data: campaignsData } = useCampaigns(false);
  const assign = useAssignPendingItem();
  const dismiss = useDismissPendingItem();
  const [selectedCampaign, setSelectedCampaign] = useState(item.candidates?.[0] ?? '');

  const campaigns = campaignsData ?? [];
  const dropdownCampaigns = item.status === 'ambiguous'
    ? campaigns.filter((c) => (item.candidates ?? []).includes(c.id))
    : campaigns.filter((c) => c.phase === 'active');

  const handleAssign = () => {
    if (!selectedCampaign) return;
    assign.mutate({ id: item.id, campaignId: selectedCampaign });
  };

  return (
    <tr className="border-b border-[var(--surface-2)]">
      <td className="py-2 pr-3 font-mono text-xs">{item.certNumber}</td>
      <td className="py-2 pr-3 text-sm truncate max-w-[200px]" title={item.cardName}>{item.cardName}</td>
      <td className="py-2 pr-3 text-sm text-center">{item.grade}</td>
      <td className="py-2 pr-3 text-sm text-right">{formatCents(item.buyCostCents)}</td>
      <td className="py-2 pr-3">
        <span className={`text-xs px-1.5 py-0.5 rounded ${
          item.status === 'ambiguous' ? 'bg-yellow-500/20 text-yellow-400' : 'bg-orange-500/20 text-orange-400'
        }`}>
          {item.status}
        </span>
      </td>
      <td className="py-2">
        <div className="flex items-center gap-2">
        <select
          value={selectedCampaign}
          onChange={(e) => setSelectedCampaign(e.target.value)}
          className="text-xs bg-[var(--surface-1)] border border-[var(--surface-2)] rounded px-1.5 py-1 text-[var(--text)] max-w-[160px]"
        >
          <option value="">Select campaign...</option>
          {dropdownCampaigns.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
        <button
          onClick={handleAssign}
          disabled={!selectedCampaign || assign.isPending}
          className="text-xs px-2 py-1 rounded bg-emerald-600 text-white hover:bg-emerald-500 disabled:opacity-50"
        >
          Assign
        </button>
        <button
          onClick={() => dismiss.mutate(item.id)}
          disabled={dismiss.isPending}
          className="text-xs px-2 py-1 rounded text-[var(--text-muted)] hover:text-red-400"
          title="Dismiss"
        >
          X
        </button>
        </div>
      </td>
    </tr>
  );
}

export function PendingItemsCard() {
  const { data, isLoading, isError } = usePSAPendingItems();
  const items = data?.items ?? [];

  return (
    <div className="rounded-xl border border-[var(--surface-2)] bg-[var(--surface-0)] p-4">
      <div className="flex items-center justify-between mb-3">
        <h4 className="text-sm font-semibold text-[var(--text)]">Pending Items</h4>
        {items.length > 0 && (
          <span className="text-xs px-2 py-0.5 rounded-full bg-orange-500/20 text-orange-400 font-medium">
            {items.length}
          </span>
        )}
      </div>

      {isLoading && <p className="text-sm text-[var(--text-muted)]">Loading...</p>}

      {!isLoading && isError && (
        <p className="text-sm text-red-400">Failed to load pending items.</p>
      )}

      {!isLoading && !isError && items.length === 0 && (
        <p className="text-sm text-[var(--text-muted)]">No pending items - all PSA imports matched or resolved.</p>
      )}

      {items.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-left">
            <thead>
              <tr className="text-xs text-[var(--text-muted)] border-b border-[var(--surface-2)]">
                <th className="pb-2 pr-3 font-medium">Cert #</th>
                <th className="pb-2 pr-3 font-medium">Card Name</th>
                <th className="pb-2 pr-3 font-medium text-center">Grade</th>
                <th className="pb-2 pr-3 font-medium text-right">Cost</th>
                <th className="pb-2 pr-3 font-medium">Status</th>
                <th className="pb-2 font-medium">Action</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <PendingRow key={item.id} item={item} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
