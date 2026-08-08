import { useState } from 'react';
import { usePSAPendingItems, useAssignPendingItem, useDismissPendingItem, useCampaigns, useCertLookup } from '../../queries/useCampaignQueries';
import type { PSAPendingItem } from '../../../types/admin';
import PokeballLoader from '../../PokeballLoader';

function formatCents(cents: number): string {
  return '$' + (cents / 100).toFixed(2);
}

/**
 * A pending item's cardName is a placeholder when PSA never attached a real
 * marketplace listing title — e.g. instant-offer acquisitions arrive as
 * "PSA Offer - <cert>". Such names can't disambiguate campaigns, so we resolve
 * the real card name on demand via a cert lookup.
 */
// Matches the exact PSA instant-offer placeholder "PSA Offer - <cert>" (cert is
// the non-empty listing suffix). Scoped tightly so a legitimately named card
// like "PSA Offer Card" is not mistaken for a placeholder.
const placeholderNameRegex = /^psa offer - .+$/;

function isPlaceholderName(item: PSAPendingItem): boolean {
  const name = item.cardName?.trim() ?? '';
  return name === '' || placeholderNameRegex.test(name.toLowerCase());
}

function PendingRow({ item }: { item: PSAPendingItem }) {
  const { data: campaignsData } = useCampaigns(false);
  const assign = useAssignPendingItem();
  const dismiss = useDismissPendingItem();
  // Only ambiguous items hold campaign IDs in `candidates` (see dropdown note
  // below); an unmatched item's first candidate is a PSA campaign name, which
  // would seed the select with a value matching no option.
  const [selectedCampaign, setSelectedCampaign] = useState(
    item.status === 'ambiguous' ? (item.candidates?.[0] ?? '') : '',
  );

  const needsLookup = isPlaceholderName(item);
  const certLookup = useCertLookup(item.certNumber, needsLookup);
  const resolvedName = certLookup.data?.cert.cardName?.trim();
  const displayName = resolvedName || item.cardName;

  const campaigns = campaignsData ?? [];
  // Ambiguous items carry real campaign IDs in `candidates`, so the dropdown
  // narrows to the tie the matcher could not break. Unmatched items do not:
  // reconcile-sourced ones put the unresolvable PSA campaign name there, and
  // import-sourced ones leave it empty — so every campaign is a valid target.
  //
  // Do not narrow the unmatched list by phase. It used to filter to
  // phase === 'active', which since the phase-allocation work (#509) leaves
  // only the External sentinel: the operator's sole option was to dump
  // correctly-attributed cards into the unattributed bucket.
  const dropdownCampaigns = item.status === 'ambiguous'
    ? campaigns.filter((c) => (item.candidates ?? []).includes(c.id))
    : campaigns;

  // The row is keyed by item.id, so a refetch that changes this item's status
  // or candidates updates props without remounting — and a useState initializer
  // only runs on mount, leaving a selection the dropdown no longer offers. The
  // <select> renders blank in that state while `selectedCampaign` still holds
  // the stale ID, so gate on membership rather than on non-emptiness: what the
  // operator can see is what they can submit.
  const selectionIsOffered = dropdownCampaigns.some((c) => c.id === selectedCampaign);

  const handleAssign = () => {
    if (!selectionIsOffered) return;
    assign.mutate({ id: item.id, campaignId: selectedCampaign });
  };

  return (
    <tr className="border-b border-[var(--surface-2)]">
      <td className="py-2 pr-3 font-mono text-xs">{item.certNumber}</td>
      <td className="py-2 pr-3 text-sm truncate max-w-[200px]" title={displayName}>
        {needsLookup && certLookup.isLoading ? (
          <span className="text-[var(--text-muted)] italic">Looking up cert…</span>
        ) : (
          displayName
        )}
      </td>
      <td className="py-2 pr-3 text-sm text-center">{item.grade}</td>
      <td className="py-2 pr-3 text-sm text-right">{formatCents(item.buyCostCents)}</td>
      <td className="py-2 pr-3">
        <span className={`text-xs px-1.5 py-0.5 rounded ${
          item.status === 'ambiguous' ? 'bg-[var(--warning-bg)] text-[var(--warning)]' : 'bg-[var(--danger-bg)] text-[var(--danger)]'
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
          disabled={!selectionIsOffered || assign.isPending}
          className="text-xs px-2 py-1 rounded bg-[var(--brand-500)] text-white hover:opacity-90 disabled:opacity-50"
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
        <h2 className="text-sm font-semibold text-[var(--text)]">Pending Items</h2>
        {items.length > 0 && (
          <span className="text-xs px-2 py-0.5 rounded-full bg-[var(--warning-bg)] text-[var(--warning)] font-medium">
            {items.length}
          </span>
        )}
      </div>

      {isLoading && <PokeballLoader size="sm" />}

      {!isLoading && isError && (
        <p className="text-sm text-[var(--danger)]">Failed to load pending items.</p>
      )}

      {!isLoading && !isError && items.length === 0 && (
        <p className="text-sm text-[var(--text-muted)]">No pending items.</p>
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
