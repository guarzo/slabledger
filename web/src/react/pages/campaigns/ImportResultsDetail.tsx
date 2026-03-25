import { useState, useMemo, useEffect } from 'react';
import { api } from '../../../js/api';
import type { Campaign, ImportResultItem } from '../../../types/campaigns';
import { formatCents, getErrorMessage } from '../../utils/formatters';
import { isAPIError } from '../../../js/api';

interface ActionableItem extends ImportResultItem {
  rowId: string;
}

interface ImportResultsDetailProps {
  results: ImportResultItem[];
  campaigns: Campaign[];
  onItemResolved: () => void;
}

export default function ImportResultsDetail({ results, campaigns, onItemResolved }: ImportResultsDetailProps) {
  const actionable = useMemo<ActionableItem[]>(
    () => results
      .map((r, i) => ({ ...r, rowId: `row-${i}` }))
      .filter(r => r.status === 'unmatched' || r.status === 'ambiguous'),
    [results],
  );
  const [resolved, setResolved] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState<string | null>(null);

  // Reset per-batch state when results change
  useEffect(() => {
    setResolved({});
    setSelected({});
    setSubmitting(null);
  }, [results]);

  const campaignMap = useMemo(
    () => Object.fromEntries(campaigns.map(c => [c.id, c])),
    [campaigns],
  );

  if (actionable.length === 0) return null;

  function getCampaignOptions(item: ImportResultItem): Campaign[] {
    if (item.status === 'ambiguous' && item.candidates) {
      return item.candidates
        .map(id => campaignMap[id])
        .filter((c): c is Campaign => !!c);
    }
    return campaigns;
  }

  function canAssign(item: ActionableItem): boolean {
    return !!selected[item.rowId] && submitting !== item.rowId && item.buyCostCents != null && item.grade != null;
  }

  async function handleAssign(item: ActionableItem) {
    const targetId = selected[item.rowId];
    if (!targetId || item.buyCostCents == null) return;
    if (item.grade == null) {
      setResolved(prev => ({ ...prev, [item.rowId]: 'error: Grade is required' }));
      return;
    }
    const campaign = campaignMap[targetId];
    if (!campaign) return;

    setSubmitting(item.rowId);
    try {
      await api.createPurchase(targetId, {
        cardName: item.cardName ?? '',
        certNumber: item.certNumber,
        gradeValue: item.grade,
        buyCostCents: item.buyCostCents,
        ...(item.clValueCents != null && { clValueCents: item.clValueCents }),
        psaSourcingFeeCents: campaign.psaSourcingFeeCents,
        purchaseDate: item.purchaseDate ?? new Date().toLocaleDateString('en-CA'),
        setName: item.setName,
        cardNumber: item.cardNumber,
        population: item.population,
      });
      setResolved(prev => ({ ...prev, [item.rowId]: 'ok' }));
      onItemResolved();
    } catch (err) {
      const isDup = isAPIError(err) && err.status === 409;
      const msg = getErrorMessage(err, 'Failed');
      setResolved(prev => ({ ...prev, [item.rowId]: isDup ? 'duplicate' : `error: ${msg}` }));
    } finally {
      setSubmitting(null);
    }
  }

  const unresolvedCount = actionable.filter(r => !resolved[r.rowId]).length;

  return (
    <div className="mt-3 p-3 rounded-lg bg-[var(--surface-2)]/50 text-sm">
      <div className="flex items-center justify-between mb-2">
        <span className="font-medium text-[var(--text)]">
          Needs Action ({unresolvedCount} remaining)
        </span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-[var(--surface-2)]">
              <th className="text-left py-1 px-2 text-[var(--text-muted)]">Cert #</th>
              <th className="text-left py-1 px-2 text-[var(--text-muted)]">Card</th>
              <th className="text-center py-1 px-2 text-[var(--text-muted)]">Grade</th>
              <th className="text-right py-1 px-2 text-[var(--text-muted)]">Cost</th>
              <th className="text-center py-1 px-2 text-[var(--text-muted)]">Status</th>
              <th className="text-left py-1 px-2 text-[var(--text-muted)]">Campaign</th>
              <th className="py-1 px-2"></th>
            </tr>
          </thead>
          <tbody>
            {actionable.map(item => {
              const status = resolved[item.rowId];
              const options = getCampaignOptions(item);

              return (
                <tr key={item.rowId} className="border-b border-[var(--surface-2)]/30">
                  <td className="py-1.5 px-2 text-[var(--text-muted)]">{item.certNumber}</td>
                  <td className="py-1.5 px-2 text-[var(--text)]">{item.cardName}</td>
                  <td className="py-1.5 px-2 text-center text-[var(--text)]">{item.grade || '-'}</td>
                  <td className="py-1.5 px-2 text-right text-[var(--text)]">
                    {item.buyCostCents ? formatCents(item.buyCostCents) : '-'}
                  </td>
                  <td className="py-1.5 px-2 text-center">
                    {status === 'ok' ? (
                      <span className="text-[var(--success)]">Assigned</span>
                    ) : status === 'duplicate' ? (
                      <span className="text-[var(--warning)]">Already exists</span>
                    ) : status?.startsWith('error:') ? (
                      <span className="text-[var(--danger)]" title={status}>{status.slice(7)}</span>
                    ) : item.status === 'ambiguous' ? (
                      <span className="text-[var(--warning)]">Ambiguous</span>
                    ) : (
                      <span className="text-[var(--warning)]">Unmatched</span>
                    )}
                  </td>
                  <td className="py-1.5 px-2">
                    {!status || status.startsWith('error:') ? (
                      <select
                        className="text-xs bg-[var(--surface-1)] text-[var(--text)] rounded px-1.5 py-1 border border-[var(--surface-2)] w-full"
                        aria-label="Select campaign"
                        value={selected[item.rowId] ?? ''}
                        onChange={e => setSelected(prev => ({ ...prev, [item.rowId]: e.target.value }))}
                      >
                        <option value="">Select campaign...</option>
                        {options.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
                      </select>
                    ) : null}
                  </td>
                  <td className="py-1.5 px-2">
                    {(!status || status.startsWith('error:')) && (
                      <button
                        type="button"
                        onClick={() => handleAssign(item)}
                        disabled={!canAssign(item)}
                        className="text-xs text-[var(--brand-400)] hover:text-[var(--brand-300)] disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
                      >
                        {submitting === item.rowId ? '...' : 'Assign'}
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
